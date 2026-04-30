package daemon

import (
	"encoding/json"
	"fmt"
	"litcontainer/internal/container"
	"litcontainer/internal/filesys"
	"litcontainer/internal/logger"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func NewInitProcess(containerCfg *container.Config) (*exec.Cmd, *os.File, error) {
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
	if len(containerCfg.Envs) > 0 {
		initCmd.Env = append(os.Environ(), containerCfg.Envs...)
	}

	// 修改子进程工作目录，子进程启动后就是
	initCmd.Dir = filesys.GetMountPoint(containerCfg.ID)

	// 配置 Linux 命名空间隔离标志
	initCmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNS |
			syscall.CLONE_NEWNET |
			syscall.CLONE_NEWIPC,
	}

	// 配置交互式终端的标准流
	if containerCfg.TTY {
		initCmd.Stdin = os.Stdin
		initCmd.Stdout = os.Stdout
		initCmd.Stderr = os.Stderr
	} else {
		containerLogDir := filepath.Join(container.DefaultLitContainerDir, containerCfg.ID)
		if err := os.MkdirAll(containerLogDir, 0755); err != nil {
			logger.Error("Failed to create container directory: %v", err)
			return nil, nil, err
		}
		containerLogFile := filepath.Join(containerLogDir, container.DefaultContainerLogFileName)
		logFile, err := os.Create(containerLogFile)
		if err != nil {
			logger.Error("Failed to create log file: %v", err)
			return nil, nil, fmt.Errorf("failed to create log file: %w", err)
		}
		defer logFile.Close()
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

func SendInitConfig(writePipe *os.File, containerConfig *container.Config) error {
	defer writePipe.Close()
	encoder := json.NewEncoder(writePipe)
	if err := encoder.Encode(&containerConfig); err != nil {
		logger.Error("Failed to write serverconfig.json to pipe: %v", err)
		return fmt.Errorf("failed to write serverconfig.json [%v] to pipe: %w", containerConfig, err)
	}
	return nil
}
