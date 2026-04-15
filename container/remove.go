package container

import (
	"fmt"
	"litcontainer/config"
	"litcontainer/enum"
	"litcontainer/pkg/logger"
	"os"
	"path/filepath"
)

func RemoveContainer(containerIdOrName string, force bool) error {
	containerConfig, err := config.GetContainerConfig(containerIdOrName)
	if err != nil {
		return fmt.Errorf("failed to find container %s: %w", containerIdOrName, err)
	}

	if !force && containerConfig.State != enum.ContainerStoppedState {
		return fmt.Errorf("container %s is not stopped", containerIdOrName)
	}

	if force && containerConfig.State != enum.ContainerStoppedState {
		if err := StopContainer(containerIdOrName); err != nil {
			return fmt.Errorf("failed to stop container %s: %w", containerIdOrName, err)
		}
	}

	containerDir := filepath.Join(config.DefaultLitContainerDir, containerConfig.Id)
	err = os.RemoveAll(containerDir)
	if err != nil {
		return fmt.Errorf("failed to remove container directory %s: %w", containerDir, err)
	}

	logger.Info("Removed container %s", containerIdOrName)
	return nil
}
