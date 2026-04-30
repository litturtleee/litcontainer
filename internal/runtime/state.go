package runtime

type ContainerState struct {
	Version     string            `json:"ociVersion"`
	ID          string            `json:"id"`
	Status      string            `json:"status"` // created / running / stopped
	PID         int               `json:"pid,omitempty"`
	Bundle      string            `json:"bundle"` // bundle 目录绝对路径
	Annotations map[string]string `json:"annotations,omitempty"`
}
