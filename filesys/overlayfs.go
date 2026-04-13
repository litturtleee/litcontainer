package filesys

import (
	"fmt"
	"litcontainer/pkg/logger"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// CreateOverlayFS 基于busybox镜像创建 overlay 文件系统
func CreateOverlayFS(busyboxDir, mountPoint, tarPath string) error {
	// 解压busybox镜像快照
	// 1. 检查busybox目录是否存在
	if _, err := os.Stat(busyboxDir); os.IsNotExist(err) {
		logger.Debug("start to untar busybox.tar, tarPath: %v", tarPath)
		if err := os.Mkdir(busyboxDir, 0755); err != nil {
			logger.Error("Error creating busybox directory: %v", err)
			return err
		}

		output, err := exec.Command("tar", "-xvf", tarPath, "-C", busyboxDir).CombinedOutput()
		if err != nil {
			logger.Error("failed to extract %s: err %v output %s", tarPath, err, string(output))
			return err
		}
	}

	// 2.准备upper和work目录
	lowerDir := busyboxDir
	upperDir := filepath.Join(filepath.Dir(busyboxDir), "upper")
	workDir := filepath.Join(filepath.Dir(busyboxDir), "work")

	// 3.使用overlayFS挂载
	err := MountOverlayFS(lowerDir, upperDir, workDir, mountPoint)
	if err != nil {
		logger.Error("Error mounting overlayfs: %v", err)
		return err
	}

	return nil
}

func MountOverlayFS(lowerDir, upperDir, workDir, mountPoint string) error {
	if err := os.MkdirAll(upperDir, 0755); err != nil {
		logger.Error("Error creating upper directory [%v]: %v", upperDir, err)
		return err
	}

	if err := os.MkdirAll(workDir, 0755); err != nil {
		logger.Error("Error creating work directory [%v]: %v", workDir, err)
		return err
	}

	mountOption := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDir, upperDir, workDir)
	logger.Debug("start to mount overlayfs, mountOption: %v， mountPoint: %v", mountOption, mountPoint)

	if err := syscall.Mount("overlay", mountPoint, "overlay", 0, mountOption); err != nil {
		logger.Error("Error mounting overlayfs: %v", err)
		return err
	}

	logger.Debug("Mount overlayfs success, mountPoint: %v", mountPoint)
	return nil
}

// UmountOverlayFS 卸载 overlay 文件系统
func UmountOverlayFS(busyboxDir, mountPoint string) error {
	// 先尝试普通 unmount，失败后 fallback 到 lazy unmount
	if err := syscall.Unmount(mountPoint, 0); err != nil {
		logger.Debug("Normal unmount failed for %v, trying MNT_DETACH: %v", mountPoint, err)
		if err := syscall.Unmount(mountPoint, syscall.MNT_DETACH); err != nil {
			logger.Error("Error unmounting overlayfs with MNT_DETACH, mountPoint: %v, err: %v", mountPoint, err)
			return fmt.Errorf("failed to unmount overlayfs: %w", err)
		}
	}

	// 删除upper和work（lazy unmount 后内核可能仍持有引用，删除失败仅告警不阻断）
	upperDir := filepath.Join(filepath.Dir(busyboxDir), "upper")
	if err := os.RemoveAll(upperDir); err != nil {
		logger.Warn("Failed to remove upper directory [%v]: %v", upperDir, err)
	}
	workDir := filepath.Join(filepath.Dir(busyboxDir), "work")
	if err := os.RemoveAll(workDir); err != nil {
		logger.Warn("Failed to remove work directory [%v]: %v", workDir, err)
	}

	logger.Debug("Umount overlayfs success, mountPoint: %v", mountPoint)
	return nil
}
