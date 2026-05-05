package runtime

import (
	"encoding/json"
	"litcontainer/internal/logger"
	"os"
	"path/filepath"
)

const (
	DefaultRuntimeStateRootPath = "/run/litcontainer-runc"
	StateRunning                = "running"
	StateStopped                = "stopped"
	StateCreated                = "created"
)

// ContainerState 容器状态
// runc状态文件
type ContainerState struct {
	Version     string            `json:"ociVersion"`
	ID          string            `json:"id"`
	Status      string            `json:"status"` // created / running / stopped
	PID         int               `json:"pid,omitempty"`
	Bundle      string            `json:"bundle"` // bundle 目录绝对路径
	Annotations map[string]string `json:"annotations,omitempty"`
}

func StateDir(id string) string {
	return filepath.Join(DefaultRuntimeStateRootPath, id)
}

func WriteState(state *ContainerState) error {
	dir := StateDir(state.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Error("create state dir %s failed: %v", dir, err)
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		logger.Error("marshal state failed: %v", err)
		return err
	}

	// 写临时文件 + rename，避免 daemon 读到半截
	tmpPath := filepath.Join(dir, "state.json.tmp")
	finalPath := filepath.Join(dir, "state.json")
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		logger.Error("write state file %s failed: %v", tmpPath, err)
		return err
	}
	return os.Rename(tmpPath, finalPath)
}

func LoadState(id string) (*ContainerState, error) {
	data, err := os.ReadFile(filepath.Join(StateDir(id), "state.json"))
	if err != nil {
		logger.Error("read state file failed: %v", err)
		return nil, err
	}
	var state ContainerState
	if err := json.Unmarshal(data, &state); err != nil {
		logger.Error("unmarshal state file failed: %v", err)
		return nil, err
	}
	return &state, nil
}

func RemoveState(id string) error {
	return os.RemoveAll(StateDir(id))
}
