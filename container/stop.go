package container

import (
	"fmt"
	"litcontainer/config"
	"litcontainer/enum"
	"litcontainer/pkg/logger"
	"syscall"
)

func StopContainer(containerIdOrName string) error {
	containerConfig, err := config.GetContainerConfig(containerIdOrName)
	if err != nil {
		return fmt.Errorf("failed to find container %s: %w", containerIdOrName, err)
	}

	// 检查是否还在运行
	if containerConfig.State != enum.ContainerRunningState {
		return fmt.Errorf("container %s is not running", containerIdOrName)
	}

	err = syscall.Kill(containerConfig.Pid, syscall.SIGTERM)
	if err != nil {
		logger.Error("Failed to stop container %s: %v", containerIdOrName, err)
		return fmt.Errorf("failed to stop container %s: %w", containerIdOrName, err)
	}

	logger.Info("Stop container %s successfully", containerIdOrName)
	return nil
}
