package filesys

import (
	"fmt"
	"litcontainer/config"
	"litcontainer/pkg/logger"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// CreateOverlayFS 基于busybox镜像创建 overlay 文件系统
func CreateOverlayFS(containerConfig *config.ContainerConfig) (string, error) {
	imageTarPath := "/var/local/" + containerConfig.Image + ".tar"
	imageDir := filepath.Join(config.DefaultImageDir, containerConfig.Image)
	mountPointDir := filepath.Join(config.DefaultOverlayfsDir, containerConfig.Id)

	// 解压busybox镜像快照
	// 1. 检查busybox目录是否存在
	if _, err := os.Stat(imageDir); os.IsNotExist(err) {
		logger.Debug("start to untar %s.tar, tarPath: %v", containerConfig.Image, imageTarPath)
		if err := os.MkdirAll(imageDir, 0755); err != nil {
			logger.Error("Error creating %s directory: %v", containerConfig.Image, err)
			return "", err
		}

		output, err := exec.Command("tar", "-xvf", imageTarPath, "-C", imageDir).CombinedOutput()
		if err != nil {
			logger.Error("failed to extract %s: err %v output %s", imageTarPath, err, string(output))
			return "", err
		}
	}

	// 2.准备upper和work目录
	lowerDir := imageDir
	upperDir := filepath.Join(mountPointDir, "upper")
	workDir := filepath.Join(mountPointDir, "work")
	mergeDir := filepath.Join(mountPointDir, "merged")

	if err := os.MkdirAll(upperDir, 0755); err != nil {
		logger.Error("Error creating %s directory: %v", upperDir, err)
		return "", err
	}
	if err := os.MkdirAll(workDir, 0755); err != nil {
		logger.Error("Error creating %s directory: %v", workDir, err)
		return "", err
	}
	if err := os.MkdirAll(mergeDir, 0755); err != nil {
		logger.Error("Error creating %s directory: %v", mergeDir, err)
		return "", err
	}

	// 3.使用overlayFS挂载
	err := MountOverlayFS(lowerDir, upperDir, workDir, mergeDir)
	if err != nil {
		logger.Error("Error mounting overlayfs: %v", err)
		return "", err
	}

	return mergeDir, nil
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
func UmountOverlayFS(containerConfig *config.ContainerConfig) error {
	mountPointDir := filepath.Join(config.DefaultOverlayfsDir, containerConfig.Id)
	mountPoint := filepath.Join(mountPointDir, "merged")

	// 先尝试普通 unmount，失败后 fallback 到 lazy unmount
	if err := syscall.Unmount(mountPoint, 0); err != nil {
		logger.Debug("Normal unmount failed for %v, trying MNT_DETACH: %v", mountPoint, err)
		if err := syscall.Unmount(mountPoint, syscall.MNT_DETACH); err != nil {
			logger.Error("Error unmounting overlayfs with MNT_DETACH, mountPoint: %v, err: %v", mountPoint, err)
			return fmt.Errorf("failed to unmount overlayfs: %w", err)
		}
	}

	// 删除mountPointDir 下面有upper、work、merged目录
	if err := os.RemoveAll(mountPointDir); err != nil {
		logger.Warn("Failed to remove mountPoint directory [%v]: %v", mountPointDir, err)
	}

	logger.Debug("Umount overlayfs success, mountPoint: %v", mountPoint)
	return nil
}
