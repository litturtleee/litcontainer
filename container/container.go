package container

import (
	"encoding/json"
	"fmt"
	"github.com/urfave/cli"
	"litcontainer/cgroups"
	"litcontainer/config"
	"litcontainer/enum"
	"litcontainer/filesys"
	"litcontainer/pkg/logger"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

const (
	BusyboxTarPath = "/var/local/busybox-rootfs.tar"
	MountPoint     = "/mnt/overlay"
	BusyboxDir     = "/var/local/busybox"
)

// Run 启动容器并在隔离的命名空间中执行用户命令
func Run(args cli.Args, enableTTY bool, memoryLimit, cpuLimit string, mountVolumes []string) error {
	logger.Debug("container Run args: %v", args)

	initCmd, writePipe, err := NewInitProcess(enableTTY)
	if err != nil {
		logger.Error("Failed to new init process: %v", err)
		return err
	}
	defer filesys.UmountOverlayFS(BusyboxDir, MountPoint)

	cg, err := SetupCGroup(initCmd.Process.Pid, memoryLimit, cpuLimit)
	if err != nil {
		logger.Error("Failed to setup cgroup: %v", err)
		return err
	}
	defer cg.Cleanup()

	// 将container配置通过管道发给子进程
	containerConfig, err := ParseContainerConfig(args, mountVolumes)
	if err != nil {
		logger.Error("Failed to parse container config: %v", err)
		return err
	}
	if err = SendInitConfig(writePipe, containerConfig); err != nil {
		logger.Error("Failed to send init command: %v", err)
		return err
	}

	// 等待容器退出
	if enableTTY {
		waitErr := initCmd.Wait()
		// 子进程挂载的volume，这里看不到，会报错
		//if err := filesys.UnMountVolume(MountPoint, containerConfig.Mounts); err != nil {
		//	logger.Error("Failed to unmount volume: %v", err)
		//}
		return waitErr
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

	// 挂载overlay，通过工作目录让子进程感知
	err = filesys.CreateOverlayFS(BusyboxDir, MountPoint, BusyboxTarPath)
	if err != nil {
		logger.Error("Failed to mount overlay: %v", err)
		return nil, nil, err
	}
	// 修改子进程工作目录，子进程启动后就是
	initCmd.Dir = MountPoint

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

func SendInitConfig(writePipe *os.File, containerConfig *config.ContainerConfig) error {
	defer writePipe.Close()
	encoder := json.NewEncoder(writePipe)
	if err := encoder.Encode(&containerConfig); err != nil {
		logger.Error("Failed to write config.json to pipe: %v", err)
		return fmt.Errorf("failed to write config.json [%v] to pipe: %w", containerConfig, err)
	}
	return nil
}

func ParseContainerConfig(cmd, mountVolumes []string) (*config.ContainerConfig, error) {
	volume, err := parseMountVolume(mountVolumes)
	if err != nil {
		logger.Error("Failed to parse container config: %v", err)
		return nil, fmt.Errorf("failed to parse container config: %w", err)
	}
	containerConfig := config.ContainerConfig{
		Command: cmd,
		Mounts:  volume,
	}
	logger.Debug("container config is :%v")
	return &containerConfig, nil
}

func parseMountVolume(mountVolumes []string) ([]config.MountConfig, error) {
	if len(mountVolumes) == 0 {
		return nil, nil
	}

	mounts := make([]config.MountConfig, 0)
	for _, volume := range mountVolumes {
		parts := strings.SplitN(volume, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid volume format: %s", volume)
		}
		mounts = append(mounts, config.MountConfig{
			parts[0],
			parts[1],
		})
	}
	return mounts, nil
}
