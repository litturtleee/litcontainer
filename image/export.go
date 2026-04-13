package image

import (
	"fmt"
	"litcontainer/container"
	"litcontainer/pkg/logger"
	"os"
	"os/exec"
	"path/filepath"
)

func Export(imageName string) error {
	logger.Debug("starting export, imageName: %s, mountPoint: %s", imageName, container.MountPoint)
	if imageName == "" {
		return ErrImageNameInvalid
	}

	// 输出路径
	outputDir := "/var/local/image"
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			logger.Error("create output directory failed, err: %v", err)
			return fmt.Errorf("create output directory failed, err: %w", err)
		}
	}

	tarPath := filepath.Join(outputDir, imageName+".tar")
	output, err := exec.Command("tar", "-cvf", tarPath, "-C", container.MountPoint, ".").CombinedOutput()
	if err != nil {
		logger.Error("failed to export image: err %v output %s", err, string(output))
		return fmt.Errorf("failed to export image: err %w", err)
	}
	logger.Debug("export image to %s success", tarPath)
	return nil
}
