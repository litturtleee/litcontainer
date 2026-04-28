package types

import "litcontainer/internal/filesys"

type ContainerCreateRequest struct {
	Name         string                `json:"name" binding:"required"`
	Image        string                `json:"image" binding:"required"`
	Command      []string              `json:"command" binding:"omitempty"`
	Env          []string              `json:"env" binding:"omitempty"`
	Mounts       []filesys.MountConfig `json:"mounts" binding:"omitempty"`
	CPULimit     string                `json:"cpuLimit" binding:"omitempty"`
	MemoryLimit  string                `json:"memoryLimit" binding:"omitempty"`
	Network      string                `json:"network" binding:"omitempty"`
	PortMappings []string              `json:"portMappings" binding:"omitempty"`
	TTY          bool                  `json:"tty" binding:"omitempty"`
}

type NetworkCreateRequest struct {
	Name   string `json:"name" binding:"required"`
	Driver string `json:"driver" binding:"required"`
	Subnet string `json:"subnet" binding:"required"`
}

type ImageExportRequest struct {
	ContainerName string `json:"containerName" binding:"required"`
	OutputName    string `json:"outputName" binding:"required"`
}
