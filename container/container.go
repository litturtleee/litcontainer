package container

import (
	"github.com/urfave/cli"
	"litcontainer/cgroups"
	"litcontainer/enum"
	"litcontainer/pkg/logger"
	"os"
	"os/exec"
	"syscall"
)

// Run 启动容器并在隔离的命名空间中执行用户命令
func Run(args cli.Args, enableTTY bool, memoryLimit, cpuLimit string) error {
	// 构造 init 子命令的参数列表
	argv := append([]string{"init"}, args...)
	logger.Debug("container Run args: %v", argv)
	initCmd, err := NewInitProcess(argv, enableTTY, memoryLimit, cpuLimit)
	if err != nil {
		logger.Error("Failed to new init process: %v", err)
		return err
	}

	cg, err := SetupCGroup(initCmd.Process.Pid, memoryLimit, cpuLimit)
	if err != nil {
		logger.Error("Failed to setup cgroup: %v", err)
		return err
	}
	defer cg.Cleanup()

	if err := initCmd.Wait(); err != nil {
		logger.Error("Failed to wait initCmd, err: %v", err)
		return err
	}

	return nil
}

func NewInitProcess(argv []string, enableTTY bool, memoryLimit string, cpuLimit string) (*exec.Cmd, error) {
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
		return nil, err
	}

	return initCmd, nil
}

func SetupCGroup(pid int, memoryLimit, cpuLimit string) (*cgroups.CGroupManager, error) {
	cg := cgroups.NewCGroupManager(enum.AppName)
	if memoryLimit != "" {
		if err := cg.SetMemoryLimit(memoryLimit); err != nil {
			return nil, err
		}
	}
	if cpuLimit != "" {
		if err := cg.SetCPULimit(cpuLimit); err != nil {
			return nil, err
		}
	}
	if err := cg.Apply(pid); err != nil {
		return nil, err
	}
	return cg, nil
}
