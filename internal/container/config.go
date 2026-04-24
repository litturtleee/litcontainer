package container

import (
	"litcontainer/internal/filesys"
	"time"
)

const (
	DefaultLitContainerDir = "/var/lib/litcontainer/container"
	DefaultConfigFileName  = "config.json"
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
	StartAt      string                `json:"startAt"`
	UpdateAt     string                `json:"updateAt"`
}

func NewContainerConfig(name, image string, cmd, envs []string, mounts []filesys.MountConfig) *Config {
	return &Config{
		ID:      generateRandomContainerID(),
		Name:    name,
		Image:   image,
		State:   RunningState,
		StartAt: time.Now().Format(time.DateTime),
		Command: cmd,
		Envs:    envs,
		Mounts:  mounts,
	}
}
