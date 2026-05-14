package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
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

// 容器状态机设计：
//                     ┌─────────────┐
//      create         │             │
//   ─────────────►    │   Created   │
//                     │             │
//                     └──────┬──────┘
//                            │ start(prepare runtime + spwan shim)
//                            │
//                     ┌──────▼──────┐
//                     │             │
//                     │   Running   ◄─────────┐
//                     │             │         │
//                     └──────┬──────┘         │
//                            │ stop/die(cleanup runtime)
//                            │                │
//                     ┌──────▼──────┐         │start(re-prepare runtime)
//                     │             │         │
//                     │   Stopped   ┼─────────┘
//                     │             │
//                     └──────┬──────┘
//                            │  remove(cleanup persistent)
//                            │
//                            ▼
//                          [delete]

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
	state, ok := d.lookup(id)
	if !ok {
		logger.Error("Container %s not found", id)
		return container.ErrContainerNotFound
	}

	// 串行化此容器的 lifecycle 操作
	state.opMu.Lock()
	defer state.opMu.Unlock()

	cfg := state.Config
	if cfg.TTY {
		return fmt.Errorf("tty not supported")
	}

	// 状态机检查（锁内）
	switch cfg.State {
	case container.CreatedState, container.StoppedState:
		// OK
	case container.RunningState:
		return container.ErrContainerIsRunning
	default:
		return fmt.Errorf("container %s is in unexpected state: %s", id, cfg.State)
	}

	// 准备运行时资源(overlay mount + netns + 网络连接)
	success := false
	defer func() {
		if !success {
			d.teardownRuntime(state)
		}
	}()

	if err := d.prepareRuntime(state); err != nil {
		logger.Error("Prepare runtime failed, err: %v", err)
		return err
	}

	// 写OCI spec（runtime资源准备好了，spec里才能写正确的内容）
	if err := d.writeContainerSpec(state); err != nil {
		logger.Error("Write container spec failed, err: %v", err)
		return err
	}

	// 启动shim
	if err := d.startShim(state); err != nil {
		logger.Error("Start shim failed, err: %v", err)
		return err
	}

	// 更新config
	d.mu.Lock()
	cfg.State = container.RunningState
	cfg.UpdateAt = time.Now().Format(time.DateTime)
	state.done = make(chan struct{})
	d.mu.Unlock()

	if err := container.WriteContainerConfig(cfg); err != nil {
		logger.Warn("Failed to persist container config, err: %v (continuing, will reconcile on restart)", err)
	}

	// 起goroutine 监控容器
	go d.waitContainerBySocket(state)
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

	state.opMu.Lock()
	if state.Config.State != container.RunningState {
		state.opMu.Unlock()
		return container.ErrContainerNotRunning
	}
	state.opMu.Unlock()

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

	state.opMu.Lock()
	defer state.opMu.Unlock()
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

	// force 路径需要先 stop，stop 自己持锁
	state.opMu.Lock()
	curState := state.Config.State
	if curState == container.RunningState && !force {
		state.opMu.Unlock()
		return container.ErrContainerIsRunning
	}
	state.opMu.Unlock()

	if curState == container.RunningState {
		// force-stop（ContainerStop 内部会再次取锁 + 等 done）
		if err := d.ContainerStop(id, 5*time.Second); err != nil {
			return err
		}
	}

	// 删除持久资源，锁内完成
	state.opMu.Lock()
	defer state.opMu.Unlock()

	containerDir := filepath.Join(container.DefaultLitContainerDir, id)
	if err := os.RemoveAll(containerDir); err != nil {
		logger.Error("Failed to remove container dir: %v", err)
		return err
	}
	if err := filesys.RemoveOverlayFS(id); err != nil {
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

// ContainerLogsStream 获取容器日志
func (d *Daemon) ContainerLogsStream(ctx context.Context, idOrName string, follow bool, w io.Writer) error {
	id, err := d.resolveID(idOrName)
	if err != nil {
		logger.Error("Resolve id failed, err: %v", err)
		return err
	}
	state, ok := d.lookup(id)
	if !ok {
		return container.ErrContainerNotFound
	}

	path := filepath.Join(container.DefaultLitContainerDir, id, container.DefaultContainerLogFileName)
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return d.tailLog(ctx, state, f, follow, w)
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
	d.teardownRuntime(state)

	cfg := state.Config
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

// prepareRuntime 准备容器运行时资源（overlay mount + netns + 网络连接）
// 与 teardownRuntime 对称。失败时调用方应触发 teardownRuntime 回滚
func (d *Daemon) prepareRuntime(state *ContainerState) error {
	cfg := state.Config

	// 1. mount overlay merged 视图
	if err := filesys.MountOverlayFS(cfg.Image, cfg.ID); err != nil {
		logger.Error("Mount overlay failed: %v", err)
		return fmt.Errorf("mount overlay: %w", err)
	}

	// 2. 网络（可选）
	if cfg.Network != "" {
		if _, err := network.CreateNetns(cfg.ID); err != nil {
			logger.Error("Create netns failed: %v", err)
			return fmt.Errorf("create netns: %w", err)
		}
		ip, err := d.netCtrl.Connect(cfg.Network, &network.ContainerEndpointConfig{
			ID:           cfg.ID,
			PortMappings: cfg.PortMappings,
		})
		if err != nil {
			logger.Error("Connect network failed: %v", err)
			return fmt.Errorf("connect network: %w", err)
		}
		cfg.IpAddress = ip.String()
	}
	return nil
}

// teardownRuntime 销毁运行时资源（网络连接、overlay挂载等）
// 幂等
func (d *Daemon) teardownRuntime(state *ContainerState) {
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
}

// writeContainerSpec 把 cfg 转 OCI spec 并写到 bundle 目录
// 注意：netns path 来自 prepareRuntime 之后的运行时状态
func (d *Daemon) writeContainerSpec(state *ContainerState) error {
	cfg := state.Config
	spec := configToSpec(cfg)
	if cfg.Network != "" {
		netnsPath := filepath.Join(network.NetnsRootDir, cfg.ID)
		for _, ns := range spec.Linux.Namespaces {
			if ns.Type == "network" {
				ns.Path = netnsPath
				break
			}
		}
	}
	if err := writeSpec(cfg.ID, spec); err != nil {
		logger.Error("Write spec failed: %v", err)
		return err
	}
	return nil
}

// startShim 启动 shim 二进制并等待 READY 信号
// 失败时返回 error，调用方负责通过 teardownRuntime 回滚
func (d *Daemon) startShim(state *ContainerState) error {
	id := state.Config.ID
	socketPath := shim.SocketPath(id)
	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		return fmt.Errorf("create shim socket dir: %w", err)
	}
	bundleDir := filepath.Join(container.DefaultLitContainerDir, id)

	shimReadyR, shimReadyW, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("create ready pipe: %w", err)
	}
	defer shimReadyR.Close()

	shimCmd := exec.Command("litcontainer-shim",
		"--bundle", bundleDir,
		"--id", id,
		"--socket", socketPath,
		"--ready-fd", "3",
	)
	shimCmd.ExtraFiles = []*os.File{shimReadyW}
	shimCmd.Stdout = os.Stdout
	shimCmd.Stderr = os.Stderr

	if err := shimCmd.Start(); err != nil {
		shimReadyW.Close()
		return fmt.Errorf("start shim: %w", err)
	}
	// 父端立刻关写端，否则 shim 死掉时管道不会 EOF，下面会等满 5s
	shimReadyW.Close()

	// 回收第一代 shim（detach 后立刻退出）
	go func() {
		if err := shimCmd.Wait(); err != nil {
			logger.Warn("first-gen shim wait: %v", err)
		}
	}()

	if err := shimReadyR.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		logger.Error("Failed to set read deadline: %v", err)
	}
	buf := make([]byte, 16)
	n, err := shimReadyR.Read(buf)
	if err != nil || !strings.Contains(string(buf[:n]), "READY") {
		// 尽力杀掉可能存在的第二代 shim
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
		return fmt.Errorf("shim not ready: read=%q err=%w", string(buf[:n]), err)
	}
	logger.Info("Shim for container %s is ready", id)
	return nil
}

func (d *Daemon) tailLog(ctx context.Context, state *ContainerState, f *os.File, follow bool, w io.Writer) error {
	buf := make([]byte, 4096)
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if errors.Is(rerr, io.EOF) {
			if !follow {
				return nil
			}
			// 简单实现,每隔200ms读取一次日志
			// 如果停止了则最后读一次
			if d.getState(state) != container.RunningState {
				d.drainLog(f, w, buf)
				return nil
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(200 * time.Millisecond):
			}
			continue
		}
		if rerr != nil {
			return rerr
		}
	}
}

func (d *Daemon) drainLog(f *os.File, w io.Writer, buf []byte) {
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil || n == 0 {
			return
		}
	}
}

func (d *Daemon) getState(state *ContainerState) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return state.Config.State
}
