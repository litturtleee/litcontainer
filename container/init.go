package container

import (
	"litcontainer/filesys"
	"litcontainer/pkg/logger"
	"os"
	"syscall"
)

func InitContainerProcess(args []string) error {
	// 挂载proc
	if err := filesys.MountProc(); err != nil {
		logger.Error("mount proc error: %v", err)
		return err
	}

	logger.Debug("start init container args: %v", args)
	if err := syscall.Exec(args[0], args, os.Environ()); err != nil {
		logger.Error("exec init container error: %v", err)
		return err
	}

	return nil
}
