package runtime

type Spec struct {
	Version  string   `json:"ociVersion"`
	Root     *Root    `json:"root"`
	Process  *Process `json:"process"`
	Hostname string   `json:"hostname,omitempty"` // 容器主机名
	Mounts   []*Mount `json:"mounts,omitempty"`
	Linux    *Linux   `json:"linux,omitempty"`
}

type Root struct {
	Path string `json:"path"`
}

type Process struct {
	Args     []string `json:"args"`
	Env      []string `json:"env,omitempty"`
	Cwd      string   `json:"cwd"`
	Terminal bool     `json:"terminal,omitempty"`
}

type Mount struct {
	Destination string   `json:"destination"`
	Type        string   `json:"type,omitempty"`
	Source      string   `json:"source,omitempty"`
	Options     []string `json:"options,omitempty"`
}

type Linux struct {
	Namespaces  []*Namespace `json:"namespaces,omitempty"`
	CgroupsPath string       `json:"cgroupsPath,omitempty"`
	Resources   *Resources   `json:"resources,omitempty"`
}

type Namespace struct {
	Type string `json:"type"`
	Path string `json:"path,omitempty"`
}

type Resources struct {
	CPU    *CPUResources    `json:"cpu,omitempty"`
	Memory *MemoryResources `json:"memory,omitempty"`
}

type MemoryResources struct {
	Limit int64 `json:"limit,omitempty"`
}

type CPUResources struct {
	Quota  int64  `json:"quota,omitempty"`
	Period uint64 `json:"period,omitempty"`
}
