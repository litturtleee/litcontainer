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
	"sync"
	"syscall"
)

const (
	BusyboxTarPath = "/var/local/busybox-rootfs.tar"
	MountPoint     = "/mnt/overlay"
	BusyboxDir     = "/var/local/busybox"
	LogDir         = "/var/log/litcontainer"
)

// Run 启动容器并在隔离的命名空间中执行用户命令
func Run(args cli.Args, enableTTY, detached bool, containerName, memoryLimit, cpuLimit string, mountVolumes []string,
	wg *sync.WaitGroup) error {
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

	// 解析容器配置信息
	containerConfig, err := config.ParseContainerConfig(initCmd, containerName, args, mountVolumes)
	if err != nil {
		logger.Error("Failed to parse container config: %v", err)
		return err
	}
	// 写配置信息
	err = config.WriteContainerConfig(containerConfig)
	if err != nil {
		logger.Error("Failed to write container config: %v", err)
		return err
	}

	// 将container配置通过管道发给子进程
	if err = SendInitConfig(writePipe, containerConfig); err != nil {
		logger.Error("Failed to send init command: %v", err)
		return err
	}

	// 先简单让这个主进程在这等
	// todo: 由shim进程监控
	if detached {
		logger.Info("Container started in background with PID:%v", initCmd.Process.Pid)
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 清理资源
			defer cleanupResource(cg, containerConfig)
			waitErr := initCmd.Wait()
			if waitErr != nil {
				logger.Error("Container process exited with error: %v", waitErr)
			}
			logger.Info("Container process exited, PID: %v", initCmd.Process.Pid)
		}()
		return nil
	}

	// 等待容器退出
	if enableTTY {
		defer cleanupResource(cg, containerConfig)
		waitErr := initCmd.Wait()
		// 子进程挂载的volume，这里看不到，会报错
		//if err := filesys.UnMountVolume(MountPoint, containerConfig.Mounts); err != nil {
		//	logger.Error("Failed to unmount volume: %v", err)
		//}
		if waitErr != nil {
			logger.Error("Container process exited with error: %v", waitErr)
		}
		logger.Info("Container process exited, PID: %v", initCmd.Process.Pid)
		return waitErr
	}

	// 非交互单前台阻塞
	waitErr := initCmd.Wait()
	cleanupResource(cg, containerConfig)
	return waitErr
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
	} else {
		os.MkdirAll(LogDir, 0755)

		logFile, err := os.Create(fmt.Sprintf("%s/container.log", LogDir))
		if err != nil {
			logger.Error("Failed to create log file: %v", err)
			return nil, nil, fmt.Errorf("failed to create log file: %w", err)
		}
		initCmd.Stdout = logFile
		initCmd.Stderr = logFile
	}

	// 启动容器进程并等待其完成
	if err := initCmd.Start(); err != nil {
		logger.Error("Failed to run initCmd, err: %v", err)
		return nil, nil, err
	}

	logger.Info("InitPorcess start success, PID: %v", initCmd.Process.Pid)

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

// cleanupResource 释放资源(删除cgroup、解挂载overlayfs)
func cleanupResource(manager *cgroups.CGroupManager, containerConfig *config.ContainerConfig) {
	if err := manager.Cleanup(); err != nil {
		logger.Error("Failed to cleanup resource: %v", err)
	}

	if err := filesys.UmountOverlayFS(BusyboxDir, MountPoint); err != nil {
		logger.Error("Failed to umount overlayfs: %v", err)
	}

	if err := config.UpdateContainerConfig(containerConfig.Id, enum.ContainerStoppedState); err != nil {
		logger.Error("Failed to update container config: %v", err)
	}
}
