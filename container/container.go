package container

import (
	"github.com/urfave/cli"
	"litcontainer/pkg/logger"
	"os"
	"os/exec"
	"syscall"
)

// Run 启动容器并在隔离的命名空间中执行用户命令
func Run(args cli.Args, enableTTY bool) error {
	// 构造 init 子命令的参数列表
	argv := append([]string{"init"}, args...)
	logger.Debug("container Run args: %v, enableTTY %v", argv, enableTTY)
	self, _ := os.Executable()
	initCmd := exec.Command(self, argv...)

	// 配置 Linux 命名空间隔离标志
	initCmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWIPC,
	}

	// 配置交互式终端的标准流
	if enableTTY {
		initCmd.Stdin = os.Stdin
		initCmd.Stdout = os.Stdout
		initCmd.Stderr = os.Stderr
	}

	// 启动容器进程并等待其完成
	if err := initCmd.Start(); err != nil {
		logger.Error("Failed to run initCmd, err: %v", err)
		return err
	}

	if err := initCmd.Wait(); err != nil {
		logger.Error("Failed to wait initCmd, err: %v", err)
		return err
	}

	return nil
}

// MountProc 挂载 proc 文件系统
func MountProc() error {
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
