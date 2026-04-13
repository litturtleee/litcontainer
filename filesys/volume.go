package filesys

import (
	"fmt"
	"litcontainer/config"
	"litcontainer/pkg/logger"
	"os"
	"path/filepath"
	"syscall"
)

func MountVolume(rootfs string, mountConfigs []config.MountConfig) error {
	if len(mountConfigs) == 0 {
		return nil
	}

	for _, mountConfig := range mountConfigs {
		// 校验宿主机目录
		if _, err := os.Stat(mountConfig.Source); err != nil {
			logger.Error("Invalid mount source, source: %s, err: %v", mountConfig.Source, err)
			return fmt.Errorf("invalid mount source, source: %s, err: %w", mountConfig.Source, err)
		}
		// 创建容器内容目录
		mountPoint := filepath.Join(rootfs, mountConfig.Destination)
		if err := os.MkdirAll(mountPoint, 0755); err != nil {
			logger.Error("Failed to create mount point, mountPoint: %s, err: %v", mountPoint, err)
			return fmt.Errorf("failed to create mount point, mountPoint: %s, err: %w", mountPoint, err)
		}
		// bind 挂载
		if err := syscall.Mount(mountConfig.Source, mountPoint, "none", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
			logger.Error("Failed to mount volume, err: %v", err)
			return fmt.Errorf("failed to mount volume, mountPoint: %s, err: %w", mountPoint, err)
		}
	}
	return nil
}

func UnMountVolume(rootfs string, mountConfigs []config.MountConfig) error {
	if len(mountConfigs) == 0 {
		return nil
	}

	for _, mountConfig := range mountConfigs {
		// 宿主机的目录
		mountPath := filepath.Join(rootfs, mountConfig.Destination)
		if err := syscall.Unmount(mountPath, 0); err != nil {
			logger.Error("Failed to unmount volume, err: %v", err)
			return fmt.Errorf("failed to unmount volume, mountPath: %s, err: %w", mountPath, err)
		}
		logger.Debug("Unmount volume success, mountPath: %s", mountPath)
	}
	return nil
}
