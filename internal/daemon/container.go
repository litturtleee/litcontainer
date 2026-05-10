package daemon

import (
	"errors"
	"fmt"
	"litcontainer/internal/container"
	"litcontainer/internal/filesys"
	"litcontainer/internal/logger"
	"litcontainer/internal/network"
	"litcontainer/internal/shim"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	// 初始化网络（网络不下放给shim，属于docker业务，shim只负责容器的生命周期管理）
	var netnsPath string
	if cfg.Network != "" {
		netnsPath, err = network.CreateNetns(id)
		if err != nil {
			logger.Error("Create netns failed, err: %v", err)
			return err
		}
		success := false
		defer func() {
			if !success {
				network.RemoveNetns(id)
			}
		}()
		ip, err := d.netCtrl.Connect(cfg.Network, &network.ContainerEndpointConfig{
			ID:           id,
			PortMappings: cfg.PortMappings,
		})
		if err != nil {
			logger.Error("Connect network failed, err: %v", err)
			return err
		}
		cfg.IpAddress = ip.String()
		success = true
	}

	// create的时候不会初始化网络，这里初始化了要更新，这样容器ioc启动可以从net的path里拿到想要的内容
	spec := configToSpec(cfg)
	if netnsPath != "" {
		for _, ns := range spec.Linux.Namespaces {
			if ns.Type == "network" {
				ns.Path = netnsPath
				break
			}
		}
	}
	if err := writeSpec(id, spec); err != nil {
		logger.Error("Write spec failed, err: %v", err)
		return err
	}

	if cfg.State != container.CreatedState && cfg.State != container.StoppedState {
		logger.Error("Container %s is not in created or stopped state", id)
		return fmt.Errorf("container %s is not in created or stopped state", id)
	}

	// 准备shim socket目录
	socketPath := shim.SocketPath(id)
	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		logger.Error("Failed to create socket dir: %v", err)
		return fmt.Errorf("failed to create shim socket dir: %w", err)
	}

	// 调用shim
	bundleDir := filepath.Join(container.DefaultLitContainerDir, id)
	// ready pipe传递给shim，shim启动完成后会写入ready pipe，daemon在这里等待
	shimReadyR, shimReadyW, err := os.Pipe()
	if err != nil {
		logger.Error("Failed to create ready pipe: %v", err)
		return fmt.Errorf("failed to create ready pipe: %w", err)
	}
	defer shimReadyR.Close()
	shimCmd := exec.Command("litcontainer-shim",
		"--bundle", bundleDir,
		"--id", id,
		"--socket", socketPath,
		"--ready-fd", "3",
	)
	shimCmd.ExtraFiles = []*os.File{shimReadyW}
	// dup前的日志会打到daemon里，dup后的日志会打到shim.log里
	shimCmd.Stdout = os.Stdout
	shimCmd.Stderr = os.Stderr

	if err := shimCmd.Start(); err != nil {
		shimReadyW.Close()
		return fmt.Errorf("failed to start shim: %w", err)
	}
	// 子进程已经有了写端的副本了,如果这里不关，shim异常下面就没办法读到EOF（需要等待5s超时）
	shimReadyW.Close()

	//
	go func() {
		if err := shimCmd.Wait(); err != nil {
			logger.Warn("first-gen shim wait: %v", err)
		}
	}()

	// 等待shim启动完成,读pipe
	// 设置超时时间
	if err := shimReadyR.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		logger.Error("Failed to set read deadline: %v", err)
	}
	buf := make([]byte, 16)
	n, err := shimReadyR.Read(buf)
	if err != nil || !strings.Contains(string(buf[:n]), "READY") {
		// 尽力杀掉可能存在的第二代（通过 shim.pid 文件）
		shimDir := filepath.Join(shim.DefaultShimRoot, id)
		if pidBytes, e := os.ReadFile(filepath.Join(shimDir, "shim.pid")); e == nil {
			if pid, e := strconv.Atoi(strings.TrimSpace(string(pidBytes))); e == nil && pid > 0 {
				if e := syscall.Kill(pid, syscall.SIGKILL); e != nil && !errors.Is(e, syscall.ESRCH) {
					logger.Warn("kill shim %d: %v", pid, e)
				}
			}
		}
		os.RemoveAll(shimDir)
		os.Remove(socketPath)
		logger.Error("Shim not ready, output: %q, err: %v", string(buf[:n]), err)
		return fmt.Errorf("shim not ready: read=%q err=%w", string(buf[:n]), err)
	}

	logger.Info("Shim for container %s is ready", id)

	// 更新config
	d.mu.Lock()
	cfg.State = container.RunningState
	cfg.UpdateAt = time.Now().Format(time.DateTime)
	state.done = make(chan struct{})
	d.mu.Unlock()

	err = container.WriteContainerConfig(cfg)
	if err != nil {
		logger.Error("Failed to update container serverconfig, err: %v", err)
		return err
	}

	// 起goroutine 监控容器
	go d.waitContainerBySocket(state)
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

	// shim stop
	shimClient := shim.NewShimClient(id)
	if err := shimClient.Stop(int(syscall.SIGTERM), int(timeout.Seconds())); err != nil {
		logger.Error("Failed to stop container via shim, err: %v", err)
		return err
	}

	// 等daemon自己的cleanup
	<-state.done
	return nil
}

// ContainerKill 杀死容器
// Kill异步，不等容器退出，直接返回
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

	// shim kill
	return shim.NewShimClient(id).Kill(int(signal))
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
func (d *Daemon) ContainerList() []*container.Info {
	d.mu.RLock()
	cfgs := make([]container.Config, 0, len(d.containers))
	for _, s := range d.containers {
		// 拷贝一份config，避免读写冲突
		cfgs = append(cfgs, *s.Config)
	}
	d.mu.RUnlock()

	out := make([]*container.Info, 0, len(cfgs))
	for _, cfg := range cfgs {
		info := &container.Info{
			Config: &cfg,
		}
		if cfg.State == container.RunningState {
			if data, err := shim.NewShimClient(cfg.ID).State(); err != nil {
				logger.Warn("list: query shim state failed for %s: %v", cfg.ID, err)
			} else {
				info.RuntimeState = &container.RuntimeState{
					Pid:      data.Pid,
					Status:   data.Status,
					ExitCode: data.Exit,
				}
			}
		}
		out = append(out, info)
	}
	return out
}

// ContainerInspect 获取容器信息
func (d *Daemon) ContainerInspect(idOrName string) (*container.Info, error) {
	id, err := d.resolveID(idOrName)
	if err != nil {
		logger.Error("Resolve id failed, err: %v", err)
		return nil, err
	}

	d.mu.RLock()
	state, ok := d.containers[id]
	if !ok {
		d.mu.RUnlock()
		return nil, container.ErrContainerNotFound
	}
	cfgCopy := *state.Config
	d.mu.RUnlock()

	info := &container.Info{
		Config: &cfgCopy,
	}

	// 只有running状态才通过shim获取pid和status，其他状态直接返回config里的状态就好
	if cfgCopy.State == container.RunningState {
		if data, err := shim.NewShimClient(id).State(); err != nil {
			logger.Warn("inspect: query shim state failed for %s: %v", id, err)
		} else {
			info.RuntimeState = &container.RuntimeState{
				Pid:      data.Pid,
				Status:   data.Status,
				ExitCode: data.Exit,
			}
		}
	}
	return info, nil
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

func (d *Daemon) waitContainerBySocket(state *ContainerState) {
	cli := shim.NewShimClient(state.Config.ID)
	if _, err := cli.Wait(); err != nil {
		logger.Error("waitContainerBySocket %s: %v", state.Config.ID, err)
	}
	d.cleanupContainer(state)

	// 通知shim退出
	if err := cli.Delete(); err != nil {
		logger.Warn("shim delete %s: %v", state.Config.ID, err)
	}

	close(state.done)
}

func (d *Daemon) cleanupContainer(state *ContainerState) {
	cfg := state.Config

	if err := filesys.UmountOverlayFS(cfg.ID); err != nil {
		logger.Error("Failed to umount overlayfs: %v", err)
	}

	if cfg.Network != "" {
		epCfg := &network.ContainerEndpointConfig{
			ID:           cfg.ID,
			IPAddress:    cfg.IpAddress,
			PortMappings: cfg.PortMappings,
		}
		if err := d.netCtrl.Disconnect(cfg.Network, epCfg); err != nil {
			logger.Error("Failed to disconnect network: %v", err)
		}
		if err := network.RemoveNetns(cfg.ID); err != nil {
			logger.Error("Failed to remove network namespace: %v", err)
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
