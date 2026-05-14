package filesys

import (
	"fmt"
	"litcontainer/internal/logger"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

const (
	DefaultImageDir     = "/var/lib/litcontainer/image"
	DefaultOverlayFsDir = "/var/lib/litcontainer/overlay"
)

// CreateOverlayFS 准备 overlay 持久层（解压镜像 lower + 创建 upper/work 目录）
// 不挂载 merged 视图（挂载属于运行时资源，由 MountOverlayFS 负责）
func CreateOverlayFS(image, containerID string) error {
	imageTarPath := "/var/local/" + image + ".tar"
	imageDir := filepath.Join(DefaultImageDir, image)
	mountPointDir := filepath.Join(DefaultOverlayFsDir, containerID)

	// 1. 检查镜像目录是否存在
	if _, err := os.Stat(imageDir); os.IsNotExist(err) {
		logger.Debug("start to untar %s.tar, tarPath: %v", image, imageTarPath)
		if err := os.MkdirAll(imageDir, 0755); err != nil {
			logger.Error("Error creating %s directory: %v", image, err)
			return err
		}
		output, err := exec.Command("tar", "-xvf", imageTarPath, "-C", imageDir).CombinedOutput()
		if err != nil {
			logger.Error("failed to extract %s: err %v output %s", imageTarPath, err, string(output))
			return err
		}
	}

	// 2.准备upper和work目录
	upperDir := filepath.Join(mountPointDir, "upper")
	workDir := filepath.Join(mountPointDir, "work")
	if err := os.MkdirAll(upperDir, 0755); err != nil {
		logger.Error("Error creating %s directory: %v", upperDir, err)
		return err
	}
	if err := os.MkdirAll(workDir, 0755); err != nil {
		logger.Error("Error creating %s directory: %v", workDir, err)
		return err
	}

	return nil
}

// MountOverlayFS 挂载 merged 视图（运行时资源，每次 start 调用）
// 调用方需保证 CreateOverlayFS 已执行过
func MountOverlayFS(image, containerID string) error {
	imageDir := filepath.Join(DefaultImageDir, image)
	mountPointDir := filepath.Join(DefaultOverlayFsDir, containerID)
	upperDir := filepath.Join(mountPointDir, "upper")
	workDir := filepath.Join(mountPointDir, "work")
	mergeDir := filepath.Join(mountPointDir, "merged")

	if err := os.MkdirAll(mergeDir, 0755); err != nil {
		return fmt.Errorf("create merged dir: %w", err)
	}
	return mountOverlayFS(imageDir, upperDir, workDir, mergeDir)
}

// UmountOverlayFS 卸载 merged 视图（运行时资源销毁）
// 持久层（upper/work/lower）保留
func UmountOverlayFS(containerID string) error {
	mountPointDir := filepath.Join(DefaultOverlayFsDir, containerID)
	mountPoint := filepath.Join(mountPointDir, "merged")

	if err := syscall.Unmount(mountPoint, 0); err != nil {
		logger.Debug("Normal unmount failed for %v, trying MNT_DETACH: %v", mountPoint, err)
		if err := syscall.Unmount(mountPoint, syscall.MNT_DETACH); err != nil {
			// 没挂载也算成功（幂等）：EINVAL 通常是 not mounted
			if err != syscall.EINVAL && !os.IsNotExist(err) {
				logger.Error("Error unmounting overlayfs, mountPoint: %v, err: %v", mountPoint, err)
				return fmt.Errorf("unmount overlay: %w", err)
			}
		}
	}

	if err := os.RemoveAll(mountPoint); err != nil {
		logger.Warn("Failed to remove mountPoint directory [%v]: %v", mountPoint, err)
	}

	logger.Debug("Umount overlayfs success, mountPoint: %v", mountPoint)
	return nil
}

func GetMountPoint(containerID string) string {
	return filepath.Join(DefaultOverlayFsDir, containerID, "merged")
}

// RemoveOverlayFS 删除 overlay 全部数据（持久层 + merged 残留），由 ContainerRemove 调用
func RemoveOverlayFS(containerID string) error {
	mountPointDir := filepath.Join(DefaultOverlayFsDir, containerID)
	if err := os.RemoveAll(mountPointDir); err != nil {
		logger.Warn("Failed to remove mountPoint directory [%v]: %v", mountPointDir, err)
	}
	return nil
}

// --- 内部方法 ---
// mountOverlayFS 底层 mount syscall
func mountOverlayFS(lowerDir, upperDir, workDir, mountPoint string) error {
	mountOption := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDir, upperDir, workDir)
	logger.Debug("mount overlayfs, opt: %v, mountPoint: %v", mountOption, mountPoint)
	if err := syscall.Mount("overlay", mountPoint, "overlay", 0, mountOption); err != nil {
		logger.Error("Error mounting overlayfs: %v", err)
		return fmt.Errorf("mount overlay: %w", err)
	}
	return nil
}
