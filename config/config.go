package config

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"litcontainer/enum"
	"litcontainer/pkg/logger"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"
)

const (
	DefaultLitContainerDir = "/var/lib/litcontainer/container"
	DefaultConfigFileName  = "config.json"
)

type ContainerConfig struct {
	Id       string        `json:"id"`
	Name     string        `json:"name"`
	Pid      int           `json:"pid"`
	Command  []string      `json:"command"`
	Mounts   []MountConfig `json:"mounts,omitempty"`
	State    string        `json:"state"`
	StartAt  string        `json:"startAt"`
	UpdateAt string        `json:"updateAt"`
}

type MountConfig struct {
	Source      string `json:"source"`      // 宿主机路径
	Destination string `json:"destination"` // 容器内路径
}

// WriteContainerConfig 将容器配置写入文件
func WriteContainerConfig(containerConfig *ContainerConfig) error {
	jsonStr, err := json.Marshal(containerConfig)
	if err != nil {
		logger.Error("Failed to unmarshal container config: %v", err)
		return err
	}

	// 创建目录
	dirPath := filepath.Join(DefaultLitContainerDir, containerConfig.Id)
	if err := os.MkdirAll(dirPath, 0644); err != nil {
		logger.Error("Failed to create container directory: %v", err)
		return err
	}

	// 写文件
	filePath := filepath.Join(dirPath, DefaultConfigFileName)
	if err := os.WriteFile(filePath, jsonStr, 0644); err != nil {
		logger.Error("Failed to write container config, err: %v", err)
		return err
	}
	return nil
}

// ParseContainerConfig 解析容器配置
func ParseContainerConfig(containerName string, cmd, mountVolumes []string) (*ContainerConfig,
	error) {
	volume, err := parseMountVolume(mountVolumes)
	if err != nil {
		logger.Error("Failed to parse container config: %v", err)
		return nil, fmt.Errorf("failed to parse container config: %w", err)
	}
	containerConfig := ContainerConfig{
		Id:      generateRandomContainerID(),
		Name:    containerName,
		State:   enum.ContainerRunningState,
		StartAt: time.Now().Format(time.DateTime),
		Command: cmd,
		Mounts:  volume,
	}
	logger.Debug("container config is :%v", containerConfig)
	return &containerConfig, nil
}

// PrintContainersInfo 输出所有容器信息
func PrintContainersInfo() error {
	configs, err := readAllContainerConfigs()
	if err != nil {
		logger.Error("Failed to read container config, err: %v", err)
		return err
	}
	if len(configs) == 0 {
		return nil
	}
	// 格式化输出
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tPID\tCOMMAND\tSTATE\tSTARTED_AT\tUPDATED_AT")
	for _, config := range configs {
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%s\n",
			config.Id[:12],
			config.Name,
			config.Pid,
			strings.Join(config.Command, " "),
			config.State,
			config.StartAt,
			config.UpdateAt,
		)
	}
	if err := w.Flush(); err != nil {
		logger.Error("Failed to flush container info, err: %v", err)
		return err
	}
	return nil
}

// UpdateContainerConfig 更新容器配置
func UpdateContainerConfig(containerId string, state string) error {
	config, err := readContainerConfig(filepath.Join(DefaultLitContainerDir, containerId, DefaultConfigFileName))
	if err != nil {
		logger.Error("Failed to update container config, err: %v", err)
		return err
	}
	config.State = state
	config.UpdateAt = time.Now().Format(time.DateTime)
	return WriteContainerConfig(config)
}

// GetContainerConfigByName 获取容器配置
func GetContainerConfigByName(name string) (*ContainerConfig, error) {
	configs, err := readAllContainerConfigs()
	if err != nil {
		logger.Error("Failed to read container config, err: %v", err)
		return nil, err
	}
	for _, config := range configs {
		if strings.EqualFold(config.Name, name) {
			return config, nil
		}
	}
	return nil, fmt.Errorf("container %s does not exist", name)
}

func readAllContainerConfigs() ([]*ContainerConfig, error) {
	if _, err := os.Stat(DefaultLitContainerDir); err != nil {
		logger.Error("Failed to read container config, err: %v", err)
		return nil, err
	}
	dirs, err := os.ReadDir(DefaultLitContainerDir)
	if err != nil {
		logger.Error("Failed to read container config, err: %v", err)
		return nil, err
	}
	var configs []*ContainerConfig
	for _, dir := range dirs {
		logger.Debug("dir name : %v", dir.Name())
		if !dir.IsDir() {
			continue
		}
		filePath := filepath.Join(DefaultLitContainerDir, dir.Name(), DefaultConfigFileName)
		config, err := readContainerConfig(filePath)
		if err != nil {
			logger.Error("Failed to read container config, filepath: %v, err: %v", filePath, err)
			continue
		}
		logger.Info("container info: %v", config)
		configs = append(configs, config)
	}
	return configs, nil
}

func readContainerConfig(filePath string) (*ContainerConfig, error) {
	fileStr, err := os.ReadFile(filePath)
	if err != nil {
		logger.Error("Failed to read container config, filepath: %v, err: %v", filePath, err)
		return nil, err
	}
	var config ContainerConfig
	err = json.Unmarshal(fileStr, &config)
	if err != nil {
		logger.Error("Failed to unmarshal container config, filepath: %v, err: %v", filePath, err)
		return nil, err
	}
	return &config, nil
}

func parseMountVolume(mountVolumes []string) ([]MountConfig, error) {
	if len(mountVolumes) == 0 {
		return nil, nil
	}

	mounts := make([]MountConfig, 0)
	for _, volume := range mountVolumes {
		parts := strings.SplitN(volume, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid volume format: %s", volume)
		}
		mounts = append(
			mounts, MountConfig{
				Source:      parts[0],
				Destination: parts[1],
			},
		)
	}
	return mounts, nil
}

func generateRandomContainerID() string {
	bytes := make([]byte, 32) // 64个十六进制字符
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return ""
	}
	return fmt.Sprintf("%x", bytes)
}
