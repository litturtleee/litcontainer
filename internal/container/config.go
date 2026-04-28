package container

import (
	"litcontainer/internal/filesys"
	"time"
)

const (
	DefaultLitContainerDir = "/var/lib/litcontainer/container"
	DefaultConfigFileName  = "state.json"
)

type Config struct {
	ID           string                `json:"id"`
	Name         string                `json:"name"`
	Image        string                `json:"image"`
	Pid          int                   `json:"pid"`
	Network      string                `json:"network"`
	IpAddress    string                `json:"ipAddress"`
	PortMappings []string              `json:"portMappings"`
	Command      []string              `json:"command"`
	Envs         []string              `json:"envs"`
	Mounts       []filesys.MountConfig `json:"mounts,omitempty"`
	State        string                `json:"state"`
	TTY          bool                  `json:"tty"`
	CPULimit     string                `json:"cpuLimit"`
	MemoryLimit  string                `json:"memoryLimit"`
	CreatedAt    string                `json:"createdAt"`
	UpdateAt     string                `json:"updateAt"`
}

func NewContainerConfig(name, image, cpuLimit, memoryLimit, network string, cmd, envs, portMapping []string,
	mounts []filesys.MountConfig, enableTTY bool) *Config {
	return &Config{
		ID:           generateRandomContainerID(),
		Name:         name,
		Image:        image,
		State:        CreatedState,
		CreatedAt:    time.Now().Format(time.DateTime),
		Command:      cmd,
		Envs:         envs,
		Mounts:       mounts,
		TTY:          enableTTY,
		CPULimit:     cpuLimit,
		MemoryLimit:  memoryLimit,
		Network:      network,
		PortMappings: portMapping,
	}
}
