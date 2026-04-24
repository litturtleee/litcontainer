package image

import (
	"fmt"
	"litcontainer/internal/container"
	"litcontainer/internal/filesys"
	"litcontainer/internal/logger"
	"os"
	"os/exec"
	"path/filepath"
)

func Export(containerName, output string) error {
	logger.Debug("starting export, containerName: %s, output: %s", containerName, output)
	if containerName == "" {
		return fmt.Errorf("container name is nil, %w", ErrContainerNameInvalid)
	}

	containerConfig, err := container.GetContainerConfig(containerName)
	if err != nil {
		logger.Error("get container serverconfig failed, err: %v", err)
		return fmt.Errorf("get container serverconfig failed, err: %w", err)
	}

	// 输出路径
	outputDir := "/var/local/images"
	if output == "" {
		output = containerConfig.Image
	}
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			logger.Error("create output directory failed, err: %v", err)
			return fmt.Errorf("create output directory failed, err: %w", err)
		}
	}

	tarPath := filepath.Join(outputDir, fmt.Sprintf("%s.tar", output))
	mergePath := filepath.Join(filesys.DefaultOverlayFsDir, containerConfig.ID, "merged")
	cmdOutput, err := exec.Command("tar", "-cvf", tarPath, "-C", mergePath, ".").CombinedOutput()
	if err != nil {
		logger.Error("failed to export image: err %v cmdOutput %s", err, string(cmdOutput))
		return fmt.Errorf("failed to export image: err %w", err)
	}
	logger.Debug("export image to %s success", tarPath)
	return nil
}
