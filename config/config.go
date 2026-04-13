package config

type ContainerConfig struct {
	Command []string      `json:"command"`
	Mounts  []MountConfig `json:"mounts,omitempty"`
}

type MountConfig struct {
	Source      string `json:"source"`      // 宿主机路径
	Destination string `json:"destination"` // 容器内路径
}
