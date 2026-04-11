package container

import (
	"litcontainer/pkg/logger"
	"os"
	"syscall"
)

func InitContainerProcess(args []string) error {
	// 挂载proc
	if err := MountProc(); err != nil {
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
