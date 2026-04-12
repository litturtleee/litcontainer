package container

import (
	"fmt"
	"github.com/urfave/cli"
	"litcontainer/cgroups"
	"litcontainer/enum"
	"litcontainer/pkg/logger"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// Run 启动容器并在隔离的命名空间中执行用户命令
func Run(args cli.Args, enableTTY bool, memoryLimit, cpuLimit string) error {
	logger.Debug("container Run args: %v", args)

	initCmd, writePipe, err := NewInitProcess(enableTTY)
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

	// 将命令通过管道传给init命令
	if err = SendInitCommand(writePipe, args); err != nil {
		logger.Error("Failed to send init command: %v", err)
		return err
	}

	// 等待容器退出
	if enableTTY {
		if err := initCmd.Wait(); err != nil {
			logger.Error("Failed to wait initCmd, err: %v", err)
			return err
		}
	}

	return nil
}

func NewInitProcess(enableTTY bool) (*exec.Cmd, *os.File, error) {
	// 创建匿名通道
	read, write, err := os.Pipe()
	if err != nil {
		logger.Error("Failed to create pipe: %v", err)
		return nil, nil, err
	}

	self, _ := os.Executable()
	initCmd := exec.Command(self, "init")

	// 带着句柄创建子进程，read会变成子进程的 fd 3
	initCmd.ExtraFiles = []*os.File{read}

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
		return nil, nil, err
	}

	return initCmd, write, nil
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

func SendInitCommand(writePipe *os.File, cmd []string) error {
	logger.Debug("SendInitCommand cmd: %v", cmd)
	defer writePipe.Close()
	command := strings.Join(cmd, " ")
	if _, err := writePipe.WriteString(command); err != nil {
		logger.Error("Failed to write command to pipe: %v", err)
		return fmt.Errorf("failed to write command [%s] to pipe: %w", command, err)
	}
	return nil
}
