package daemon

import (
	"litcontainer/internal/container"
	"litcontainer/internal/network"
	"os/exec"
	"sync"
)

type Daemon struct {
	mu         sync.RWMutex
	containers map[string]*ContainerState
	netCtrl    *network.Controller
	root       string
}

type ContainerState struct {
	Config *container.Config
	Cmd    *exec.Cmd
	done   chan struct{}
}

func New(root string) (*Daemon, error) {
	d := &Daemon{
		containers: make(map[string]*ContainerState),
		netCtrl:    network.GetController(),
		root:       root,
	}

	configs, err := container.GetAllConfig()
	if err != nil {
		return nil, err
	}

	for _, cfg := range configs {
		d.containers[cfg.ID] = &ContainerState{Config: cfg}
		// 注意：state.Cmd 是 nil，state.done 是 nil
	}
	// TODO(phase2): 通过 shim socket 重连真实运行状态
	return d, nil
}
