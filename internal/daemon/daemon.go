package daemon

import (
	"litcontainer/internal/container"
	"litcontainer/internal/events"
	"litcontainer/internal/logger"
	"litcontainer/internal/network"
	"litcontainer/internal/shim"
	"sync"
)

type Daemon struct {
	mu         sync.RWMutex
	containers map[string]*ContainerState
	netCtrl    *network.Controller
	root       string
	eventBus   *events.EventBus
}

type ContainerState struct {
	Config *container.Config
	done   chan struct{}
	opMu   sync.Mutex // opMu 用于保护对容器状态的修改，避免并发操作导致状态不一致
}

func New(root string) (*Daemon, error) {
	d := &Daemon{
		containers: make(map[string]*ContainerState),
		netCtrl:    network.GetController(),
		root:       root,
		eventBus:   events.NewEventBus(),
	}

	configs, err := container.GetAllConfig()
	if err != nil {
		return nil, err
	}

	// 加载所有容器配置到内存
	for _, cfg := range configs {
		state := &ContainerState{
			Config: cfg,
			done:   closeChan(),
		}
		d.containers[cfg.ID] = state
	}

	// reconcile: 对每个标记running的容器尝试重连shim
	for _, state := range d.containers {
		if state.Config.State != container.RunningState {
			continue
		}
		d.reconcileContainer(state)
	}

	return d, nil
}

func (d *Daemon) GetEventBus() *events.EventBus {
	return d.eventBus
}

// --- 内部方法 ---
func closeChan() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

// reconcileContainer 尝试重连shim，重建对一个running容器的管控关系
func (d *Daemon) reconcileContainer(state *ContainerState) {
	logger.Info("Reconciling container %s", state.Config.ID)
	id := state.Config.ID
	cli := shim.NewShimClient(id)

	data, err := cli.State()
	if err != nil {
		// shim不通, 容器已死或异常，跑通daemon端cleanup
		logger.Warn("Failed to connect shim for container %s: %v", id, err)
		d.cleanupContainer(state)
		return
	}

	logger.Info("reconcile %s: shim alive, init pid=%d, status=%s", id, data.Pid, data.Status)
	d.mu.Lock()
	state.done = make(chan struct{})
	d.mu.Unlock()

	go d.waitContainerBySocket(state)
}
