package runtime

import (
	"encoding/json"
	"litcontainer/internal/logger"
	"os"
	"path/filepath"
)

const (
	OciVersion          = "1.0.0-litcontainer"
	DefaultSpecFileName = "config.json"
)

// Spec 运行时规范
// 符合oci标准，runc create时runc唯一读取的文件
type Spec struct {
	Version  string   `json:"ociVersion"`
	Root     *Root    `json:"root"`
	Process  *Process `json:"process"`
	Hostname string   `json:"hostname,omitempty"` // 容器主机名
	Mounts   []*Mount `json:"mounts,omitempty"`
	Linux    *Linux   `json:"linux,omitempty"`
	Hooks    *Hooks   `json:"hooks,omitempty"`
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

type Hooks struct {
	Prestart []Hook `json:"prestart,omitempty"`
	Poststop []Hook `json:"poststop,omitempty"`
}

type Hook struct {
	Path    string   `json:"path"`              // 可执行文件绝对路径
	Args    []string `json:"args,omitempty"`    // 参数（含 argv[0]）
	Env     []string `json:"env,omitempty"`     // 额外环境变量
	Timeout *int     `json:"timeout,omitempty"` // 超时秒数
}

func LoadSpec(bundleDir string) (*Spec, error) {
	content, err := os.ReadFile(filepath.Join(bundleDir, DefaultSpecFileName))
	if err != nil {
		logger.Error("failed to read spec file: %v", err)
		return nil, err
	}

	var spec Spec
	err = json.Unmarshal(content, &spec)
	if err != nil {
		logger.Error("failed to unmarshal spec: %v", err)
		return nil, err
	}

	return &spec, nil
}
