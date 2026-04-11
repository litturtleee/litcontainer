package filesys

import (
	"litcontainer/pkg/logger"
	"syscall"
)

// MountProc 挂载 proc 文件系统
func MountProc() error {
	// 现代 Linux 发行版（systemd 系统）默认把 / 及其下的挂载点都设成了 shared。所以当你的子进程mount的时候会传播会宿主机
	// 所以先把所有挂载点设置为private
	// MS_REC: 递归设置子目录为private
	// MS_PRIVATE: 设置为private
	if err := syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		logger.Error("Failed to set all mounts to private, err: %v", err)
		return err
	}

	// 设置挂载标志位：
	// MS_NODEV: 不允许访问设备文件，增强安全性
	// MS_NOEXEC: 不允许执行二进制文件，防止恶意代码执行
	// MS_NOSUID: 忽略 setuid 和 setgid 位，防止权限提升
	mountFlags := syscall.MS_NODEV | syscall.MS_NOEXEC | syscall.MS_NOSUID
	if err := syscall.Mount("proc", "/proc", "proc", uintptr(mountFlags), ""); err != nil {
		logger.Error("Failed to mount proc, err: %v", err)
		return err
	}
	return nil
}
