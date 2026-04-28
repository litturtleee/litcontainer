package daemon

import (
	"errors"
	"fmt"
	"litcontainer/internal/container"
	"litcontainer/internal/filesys"
	"litcontainer/internal/logger"
	"litcontainer/internal/network"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type CreateOptions struct {
	Name         string
	Image        string
	Command      []string
	Env          []string
	Mounts       []filesys.MountConfig
	CPULimit     string
	MemoryLimit  string
	Network      string
	PortMappings []string
	EnableTTY    bool
}

// ContainerCreate 创建容器
// 返回容器ID
func (d *Daemon) ContainerCreate(opts *CreateOptions) (string, error) {
	logger.Info("Creating container: %+v", opts)
	// 重名检查
	_, err := container.GetContainerConfigByName(opts.Name)
	if err == nil {
		logger.Warn("Container %s already exists", opts.Name)
		return "", fmt.Errorf("container %s already exists", opts.Name)
	}
	if !errors.Is(err, container.ErrContainerNotFound) {
		logger.Error("Get container failed, err: %v", err)
		return "", err
	}
	// 生成Config
	containerConfig := container.NewContainerConfig(opts.Name, opts.Image, opts.CPULimit, opts.MemoryLimit,
		opts.Network, opts.Command, opts.Env, opts.PortMappings, opts.Mounts,
		opts.EnableTTY)

	// 创建overlayFS
	if err := filesys.CreateOverlayFS(containerConfig.Image, containerConfig.ID); err != nil {
		logger.Error("Create overlayFS failed, err: %v", err)
		return "", err
	}

	// 持久化
	if err := container.WriteContainerConfig(containerConfig); err != nil {
		logger.Error("Write container config failed, err: %v", err)
		return "", err
	}

	// 写缓存
	d.mu.Lock()
	d.containers[containerConfig.ID] = &ContainerState{
		Config: containerConfig,
	}
	d.mu.Unlock()
	return containerConfig.ID, nil
}

// ContainerStart 启动容器
func (d *Daemon) ContainerStart(idOrName string) error {
	id, err := d.resolveID(idOrName)
	if err != nil {
		logger.Error("Resolve id failed, err: %v", err)
		return err
	}

	// 校验容器是否存在和状态
	d.mu.Lock()
	state, ok := d.containers[id]
	d.mu.Unlock()
	if !ok {
		logger.Error("Container %s not found", id)
		return container.ErrContainerNotFound
	}
	cfg := state.Config
	if cfg.TTY {
		return fmt.Errorf("tty not supported")
	}
	if cfg.State != container.CreatedState && cfg.State != container.StoppedState {
		logger.Error("Container %s is not in created or stopped state", id)
		return fmt.Errorf("container %s is not in created or stopped state", id)
	}
	// 启动容器
	initCmd, writePipe, err := NewInitProcess(cfg)
	if err != nil {
		logger.Error("New init process failed, err: %v", err)
		return err
	}

	// 保证子进程有错误清理
	success := false
	defer func() {
		if !success {
			initCmd.Process.Kill()
		}
	}()

	// 配置cgroup
	cg, err := SetupCGroup(initCmd.Process.Pid, cfg.ID, cfg.CPULimit, cfg.MemoryLimit)
	if err != nil {
		logger.Error("Setup cgroup failed, err: %v", err)
		return err
	}

	// 配置网络
	var ipAddr string
	if cfg.Network != "" {
		epCfg := &network.ContainerEndpointConfig{
			ID:           cfg.ID,
			Pid:          initCmd.Process.Pid,
			IPAddress:    cfg.IpAddress,
			PortMappings: cfg.PortMappings,
		}
		ip, err := d.netCtrl.Connect(cfg.Network, epCfg)
		if err != nil {
			logger.Error("Failed to connect network: %v", err)
			return err
		}
		ipAddr = ip.String()
	}

	// 将配置发送给init进程
	if err := SendInitConfig(writePipe, cfg); err != nil {
		logger.Error("Send init config failed, err: %v", err)
		return err
	}

	// 更新config
	d.mu.Lock()
	cfg.Pid = initCmd.Process.Pid
	if ipAddr != "" {
		cfg.IpAddress = ipAddr
	}
	cfg.State = container.RunningState
	cfg.UpdateAt = time.Now().Format(time.DateTime)
	state.Cmd = initCmd
	state.CGroup = cg
	state.done = make(chan struct{})
	d.mu.Unlock()

	err = container.WriteContainerConfig(cfg)
	if err != nil {
		logger.Error("Failed to update container serverconfig, err: %v", err)
		return err
	}

	// 起goroutine 监控容器
	go d.waitContainer(state)
	success = true
	return nil

}

// ContainerWait 等待容器结束
func (d *Daemon) ContainerWait(idOrName string) error {
	id, err := d.resolveID(idOrName)
	if err != nil {
		logger.Error("Resolve id failed, err: %v", err)
		return err
	}

	state, ok := d.lookup(id)
	if !ok {
		return container.ErrContainerNotFound
	}
	<-state.done
	return nil
}

// ContainerStop 停止容器
func (d *Daemon) ContainerStop(idOrName string, timeout time.Duration) error {
	id, err := d.resolveID(idOrName)
	if err != nil {
		logger.Error("Resolve id failed, err: %v", err)
		return err
	}

	state, ok := d.lookup(id)
	if !ok {
		return container.ErrContainerNotFound
	}
	if state.Config.State != container.RunningState {
		return container.ErrContainerNotRunning
	}

	// 1.发送SIGTERM
	if err := state.Cmd.Process.Signal(syscall.SIGTERM); err != nil {
		logger.Error("Failed to send SIGTERM to container: %v", err)
		return err
	}

	// 2.等done，超时则SIGKILL
	select {
	case <-state.done:
		return nil
	case <-time.After(timeout):
		logger.Warn("Stop container %s timeout", id)
		state.Cmd.Process.Kill()
		// 等cleanup完成
		<-state.done
		return nil
	}
}

// ContainerKill 杀死容器
func (d *Daemon) ContainerKill(idOrName string, signal syscall.Signal) error {
	id, err := d.resolveID(idOrName)
	if err != nil {
		logger.Error("Resolve id failed, err: %v", err)
		return err
	}

	state, ok := d.lookup(id)
	if !ok {
		return container.ErrContainerNotFound
	}
	if state.Config.State != container.RunningState {
		return container.ErrContainerNotRunning
	}
	return state.Cmd.Process.Signal(signal)
}

// ContainerRemove 删除容器
func (d *Daemon) ContainerRemove(idOrName string, force bool) error {
	id, err := d.resolveID(idOrName)
	if err != nil {
		logger.Error("Resolve id failed, err: %v", err)
		return err
	}

	state, ok := d.lookup(id)
	if !ok {
		return container.ErrContainerNotFound
	}
	if state.Config.State == container.RunningState {
		if !force {
			return container.ErrContainerIsRunning
		}
		if err := d.ContainerStop(id, 5*time.Second); err != nil {
			return err
		}
	}

	containerDir := filepath.Join(container.DefaultLitContainerDir, id)
	err = os.RemoveAll(containerDir)
	if err != nil {
		logger.Error("Failed to remove container dir: %v", err)
		return err
	}
	err = filesys.RemoveOverlayFS(id)
	if err != nil {
		logger.Error("Failed to remove overlayfs: %v", err)
		return err
	}

	d.mu.Lock()
	delete(d.containers, id)
	d.mu.Unlock()

	logger.Info("Removed container %s", id)
	return nil
}

// ContainerList 列出容器
func (d *Daemon) ContainerList() []*container.Config {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]*container.Config, 0, len(d.containers))
	for _, state := range d.containers {
		out = append(out, state.Config)
	}
	return out
}

// ContainerInspect 获取容器信息
func (d *Daemon) ContainerInspect(idOrName string) (*container.Config, error) {
	id, err := d.resolveID(idOrName)
	if err != nil {
		logger.Error("Resolve id failed, err: %v", err)
		return nil, err
	}

	state, ok := d.lookup(id)
	if !ok {
		return nil, container.ErrContainerNotFound
	}
	return state.Config, nil
}

// ContainerLogs 获取容器日志
func (d *Daemon) ContainerLogs(idOrName string) ([]byte, error) {
	id, err := d.resolveID(idOrName)
	if err != nil {
		logger.Error("Resolve id failed, err: %v", err)
		return nil, err
	}

	state, ok := d.lookup(id)
	if !ok {
		return nil, container.ErrContainerNotFound
	}
	path := filepath.Join(container.DefaultLitContainerDir, state.Config.ID, container.DefaultContainerLogFileName)
	return os.ReadFile(path)
}

// --- 内部方法 ---

func (d *Daemon) waitContainer(state *ContainerState) {
	waitErr := state.Cmd.Wait()
	if waitErr != nil {
		logger.Error("container %s exited: %v, ProcessState: %+v, ExitCode: %d",
			state.Config.ID, waitErr, state.Cmd.ProcessState, state.Cmd.ProcessState.ExitCode())
	}
	d.cleanupContainer(state)
	close(state.done)
}

func (d *Daemon) cleanupContainer(state *ContainerState) {
	cg := state.CGroup
	cfg := state.Config
	if err := cg.Cleanup(); err != nil {
		logger.Error("Failed to cleanup resource: %v", err)
	}

	if err := filesys.UmountOverlayFS(cfg.ID); err != nil {
		logger.Error("Failed to umount overlayfs: %v", err)
	}

	if cfg.Network != "" {
		epCfg := &network.ContainerEndpointConfig{
			ID:           cfg.ID,
			IPAddress:    cfg.IpAddress,
			Pid:          cfg.Pid,
			PortMappings: cfg.PortMappings,
		}
		if err := d.netCtrl.Disconnect(cfg.Network, epCfg); err != nil {
			logger.Error("Failed to disconnect network: %v", err)
		}
	}

	d.mu.Lock()
	cfg.State = container.StoppedState
	cfg.UpdateAt = time.Now().Format(time.DateTime)
	d.mu.Unlock()

	if err := container.WriteContainerConfig(cfg); err != nil {
		logger.Error("Failed to update container serverconfig, err: %v", err)
	}
}

func (d *Daemon) lookup(id string) (*ContainerState, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	s, ok := d.containers[id]
	return s, ok
}

func (d *Daemon) resolveID(idOrName string) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// 1. 精确匹配 ID
	if _, ok := d.containers[idOrName]; ok {
		return idOrName, nil
	}

	// 2. 精确匹配 name
	for id, s := range d.containers {
		if s.Config.Name == idOrName {
			return id, nil
		}
	}

	if len(idOrName) < 12 {
		return "", container.ErrInitInvalidArgs
	}

	// 3. ID 前缀匹配（要求唯一）
	var matches []string
	for id := range d.containers {
		if strings.HasPrefix(id, idOrName) {
			matches = append(matches, id)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", container.ErrContainerNotFound
	default:
		return "", fmt.Errorf("ambiguous: %d containers match prefix %q", len(matches), idOrName)
	}
}
