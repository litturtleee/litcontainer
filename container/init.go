package container

import (
	"fmt"
	"io"
	"litcontainer/filesys"
	"litcontainer/pkg/logger"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func InitContainerProcess() error {
	// 管道内读取命令(阻塞式)
	// fd0-stdin
	// fd1-stdout
	// fd2-stderr
	// fd3-pipe
	pipe := os.NewFile(uintptr(3), "pipe")
	defer pipe.Close()
	msg, err := io.ReadAll(pipe)
	if err != nil {
		logger.Error("read pipe error: %v", err)
		return err
	}
	logger.Debug("read pipe message: %s", msg)

	// byte->string
	msgStr := string(msg)
	cmdArgs := strings.Split(msgStr, " ")
	if len(cmdArgs) == 0 {
		logger.Error("InitContainerProcess user cmd is empty", cmdArgs)
		return fmt.Errorf("InitContainerProcess user cmd is empty, %w", ErrInitInvalidArgs)
	}
	logger.Debug("InitContainerProcess user cmd: ", cmdArgs)

	// 挂载（pivot_root、proc、tmpfs)
	if err := filesys.Mount(); err != nil {
		logger.Error("mount error: %v", err)
		return err
	}

	// 在系统的PATH中寻找命令的绝对路径(因为用户可能只输入了命令名而没有输入绝对路径)
	path, err := exec.LookPath(cmdArgs[0])
	if err != nil {
		logger.Error("look path error: %v", err)
		return err
	}
	logger.Debug("InitContainerProcess user cmd abs path: ", path)

	if err := syscall.Exec(path, cmdArgs, os.Environ()); err != nil {
		logger.Error("exec init container error: %v", err)
		return err
	}

	return nil
}
