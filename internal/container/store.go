package container

import (
	"encoding/json"
	"fmt"
	"litcontainer/internal/logger"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GetAllConfig 获取所有容器配置
func GetAllConfig() ([]*Config, error) {
	return readAllContainerConfigs()
}

// GetContainerConfig 获取容器配置ByIdOrName
func GetContainerConfig(idOrName string) (*Config, error) {
	containerConfig, err := GetContainerConfigById(idOrName)
	if err != nil {
		return nil, err
	}

	if containerConfig != nil {
		logger.Debug("Find container serverconfig by id, id:%s", idOrName)
		return containerConfig, nil
	}

	containerConfig, err = GetContainerConfigByName(idOrName)
	if err != nil {
		return nil, err
	}
	if containerConfig != nil {
		logger.Debug("Find container serverconfig by name, name:%s", idOrName)
		return containerConfig, nil
	}

	logger.Error("Container cannot be found, idOrName: %s", idOrName)
	return nil, fmt.Errorf("container %s does not exist", idOrName)
}

// GetContainerConfigByName 获取容器配置
func GetContainerConfigByName(name string) (*Config, error) {
	configs, err := readAllContainerConfigs()
	if err != nil {
		logger.Error("Failed to read container serverconfig, err: %v", err)
		return nil, err
	}
	for _, config := range configs {
		if strings.EqualFold(config.Name, name) {
			return config, nil
		}
	}
	return nil, fmt.Errorf("container %s does not exist", name)
}

// GetContainerConfigById 获取容器配置
func GetContainerConfigById(containerId string) (*Config, error) {
	if len(containerId) >= 12 {
		fullId := ""
		dirs, err := os.ReadDir(DefaultLitContainerDir)
		if err != nil {
			logger.Error("Failed to read container serverconfig, err: %v", err)
			return nil, err
		}
		for _, dir := range dirs {
			if dir.IsDir() && strings.HasPrefix(dir.Name(), containerId) {
				fullId = dir.Name()
				break
			}
		}
		if fullId != "" {
			configPath := filepath.Join(DefaultLitContainerDir, fullId, DefaultConfigFileName)
			fileStr, err := os.ReadFile(configPath)
			if err != nil {
				logger.Error("Failed to read container serverconfig, filepath: %v, err: %v", configPath, err)
				return nil, err
			}
			var config Config
			err = json.Unmarshal(fileStr, &config)
			if err != nil {
				logger.Error("Failed to unmarshal container serverconfig, filepath: %v, err: %v", configPath, err)
				return nil, err
			}
			return &config, nil
		}
	}
	// 打印日志，id小于12，无法查出容器
	logger.Debug("Container cannot be found when ID length is less than 12 characters")
	return nil, nil
}

// WriteContainerConfig 将容器配置写入文件
func WriteContainerConfig(containerConfig *Config) error {
	jsonStr, err := json.Marshal(containerConfig)
	if err != nil {
		logger.Error("Failed to unmarshal container serverconfig: %v", err)
		return err
	}

	// 创建目录
	dirPath := filepath.Join(DefaultLitContainerDir, containerConfig.ID)
	if err := os.MkdirAll(dirPath, 0644); err != nil {
		logger.Error("Failed to create container directory: %v", err)
		return err
	}

	// 写文件
	filePath := filepath.Join(dirPath, DefaultConfigFileName)
	if err := os.WriteFile(filePath, jsonStr, 0644); err != nil {
		logger.Error("Failed to write container serverconfig, err: %v", err)
		return err
	}
	return nil
}

// UpdateContainerConfig 更新容器配置
func UpdateContainerConfig(containerId string, state string) error {
	config, err := readContainerConfig(filepath.Join(DefaultLitContainerDir, containerId, DefaultConfigFileName))
	if err != nil {
		logger.Error("Failed to update container serverconfig, err: %v", err)
		return err
	}
	config.State = state
	config.UpdateAt = time.Now().Format(time.DateTime)
	return WriteContainerConfig(config)
}

// --- 内部方法 ---
func readAllContainerConfigs() ([]*Config, error) {
	if _, err := os.Stat(DefaultLitContainerDir); err != nil {
		logger.Error("Failed to read container serverconfig, err: %v", err)
		return nil, err
	}
	dirs, err := os.ReadDir(DefaultLitContainerDir)
	if err != nil {
		logger.Error("Failed to read container serverconfig, err: %v", err)
		return nil, err
	}
	var configs []*Config
	for _, dir := range dirs {
		logger.Debug("dir name : %v", dir.Name())
		if !dir.IsDir() {
			continue
		}
		filePath := filepath.Join(DefaultLitContainerDir, dir.Name(), DefaultConfigFileName)
		config, err := readContainerConfig(filePath)
		if err != nil {
			logger.Error("Failed to read container serverconfig, filepath: %v, err: %v", filePath, err)
			continue
		}
		logger.Info("container info: %v", config)
		configs = append(configs, config)
	}
	return configs, nil
}

func readContainerConfig(filePath string) (*Config, error) {
	fileStr, err := os.ReadFile(filePath)
	if err != nil {
		logger.Error("Failed to read container serverconfig, filepath: %v, err: %v", filePath, err)
		return nil, err
	}
	var config Config
	err = json.Unmarshal(fileStr, &config)
	if err != nil {
		logger.Error("Failed to unmarshal container serverconfig, filepath: %v, err: %v", filePath, err)
		return nil, err
	}
	return &config, nil
}
