package filesys

import (
	"litcontainer/config"
	"litcontainer/pkg/logger"
	"os"
	"path/filepath"
	"syscall"
)

// Mount 实现runc的mount
// 设置挂载传播->
func Mount(mountConfigs []config.MountConfig) error {
	// 进入子进程时设置merged目录为子进程的工作目录
	rootfs, err := os.Getwd()
	logger.Debug("Current rootfs directory: %s", rootfs)
	if err != nil {
		logger.Error("Failed to get current working directory, err: %v", err)
		return err
	}
	// 1.阻断子进程到父进程的share挂载
	err = SetMountPropagation()
	if err != nil {
		logger.Error("Failed to set mount propagation, err: %v", err)
		return err
	}
	// pivot_root要求new_rootfs必须是挂载点
	err = prepareMountRootfs(rootfs)
	if err != nil {
		logger.Error("Failed to prepare mount rootfs, err: %v", err)
		return err
	}

	// 2.挂载特殊文件系统(/proc、/dev等)
	err = MountProc(rootfs)
	if err != nil {
		logger.Error("Failed to mount proc, err: %v", err)
		return err
	}

	err = MountTmpfs(rootfs)
	if err != nil {
		logger.Error("Failed to mount tmpfs, err: %v", err)
		return err
	}

	// 3.挂载volumes
	err = MountVolume(rootfs, mountConfigs)
	if err != nil {
		logger.Error("Failed to mount volumes, err: %v", err)
		return err
	}

	// 4.pivot_root
	err = MountPivotRoot(rootfs)
	if err != nil {
		logger.Error("Failed to mount pivot root, err: %v", err)
		return err
	}
	return nil
}

// MountProc 挂载 proc 文件系统
func MountProc(rootfs string) error {
	// 设置挂载标志位：
	// MS_NODEV: 不允许访问设备文件，增强安全性
	// MS_NOEXEC: 不允许执行二进制文件，防止恶意代码执行
	// MS_NOSUID: 忽略 setuid 和 setgid 位，防止权限提升
	mountFlags := syscall.MS_NODEV | syscall.MS_NOEXEC | syscall.MS_NOSUID
	if err := syscall.Mount("proc", filepath.Join(rootfs, "proc"), "proc", uintptr(mountFlags), ""); err != nil {
		logger.Error("Failed to mount proc, err: %v", err)
		return err
	}
	return nil
}

// SetMountPropagation 设置子进程挂载传播
func SetMountPropagation() error {
	// 现代 Linux 发行版（systemd 系统）默认把 / 及其下的挂载点都设成了 shared。所以当你的子进程mount的时候会传播会宿主机
	// 所以先把所有挂载点设置为private
	// MS_REC: 递归设置子目录为private
	// MS_PRIVATE: 设置为private
	if err := syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		logger.Error("Failed to set all mounts to private, err: %v", err)
		return err
	}
	return nil
}
func prepareMountRootfs(rootfs string) error {
	// pivot_root要求new_root必须是一个挂载点
	if err := syscall.Mount(rootfs, rootfs, "bind", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		logger.Error("Failed to mount rootfs, err: %v", err)
		return err
	}
	return nil
}

func MountPivotRoot(rootfs string) error {
	// 准备pivot_root需要的old_root
	pivotOldDir := filepath.Join(rootfs, ".pivot_root")
	logger.Debug("root is %v, PivotOldDir is %v", rootfs, pivotOldDir)
	if _, err := os.Stat(pivotOldDir); err == nil {
		if err := os.RemoveAll(pivotOldDir); err != nil {
			logger.Error("Failed to remove exiting pivotDir, err: %v", err)
			return err
		}
	}

	if err := os.Mkdir(pivotOldDir, 0755); err != nil {
		logger.Error("Failed to create pivotDir, err: %v", err)
		return err
	}

	// 调用pivot_root
	// 1.完成挂载点切换
	// 2.修改当前工作目录为新的根目录
	// 3.解挂载旧根目录
	// 4.删除旧根目录
	if err := syscall.PivotRoot(rootfs, pivotOldDir); err != nil {
		logger.Error("Failed to call pivot_root, err: %v", err)
		return err
	}

	if err := syscall.Chdir("/"); err != nil {
		logger.Error("Failed to chdir /, err: %v", err)
		return err
	}

	oldRootfsMount := filepath.Join("/", ".pivot_root")
	if err := syscall.Unmount(oldRootfsMount, syscall.MNT_DETACH); err != nil {
		logger.Error("Failed to unmount old_root, err: %v", err)
		return err
	}

	if err := os.RemoveAll(oldRootfsMount); err != nil {
		logger.Error("Failed to remove old_root, err: %v", err)
		return err
	}
	return nil
}

// MountTmpfs 该函数的作用是将一个 tmpfs 文件系统挂载到 /dev 目录。
// tmpfs 是一种基于内存的临时文件系统，常用于需要快速读写且不需要持久化的场景
// 容器需要基本的设备节点（如 /dev/null, /dev/zero 等）来运行程序。
// 使用 tmpfs 可以动态生成这些设备节点，并且是临时的，重启后不会保留。
// 理论上还有/proc、/sys等应该需要运行时挂载并处理
func MountTmpfs(rootfs string) error {
	moutflags := syscall.MS_NOSUID | syscall.MS_STRICTATIME
	if err := syscall.Mount("tmpfs", filepath.Join(rootfs, "dev"), "tmpfs", uintptr(moutflags), "mode=755"); err != nil {
		logger.Error("Failed to mount tmpfs, err: %v", err)
		return err
	}
	// 处理/dev添加设备
	return nil
}
