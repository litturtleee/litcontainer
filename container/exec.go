package container

import (
	"fmt"
	"litcontainer/config"
	"litcontainer/pkg/logger"
	"os"
	"os/exec"
)

const ExecContainerPidEnv = "LITCONTAINER_EXEC_PID"

func Exec(enableTTY bool, containerName string, args []string) error {
	containerConfig, err := config.GetContainerConfigByName(containerName)
	if err != nil {
		logger.Error("get container config failed: %v", err)
		return fmt.Errorf("get container config failed: %w", err)
	}

	logger.Debug("exec target container pid: %s, args: %s", containerConfig.Pid, args)

	self, _ := os.Executable()
	args = append([]string{"exec-container"}, args...)
	execCmd := exec.Command(self, args...)
	env := os.Environ()
	env = append(env, fmt.Sprintf("%s=%d", ExecContainerPidEnv, containerConfig.Pid))
	execCmd.Env = env

	if enableTTY {
		execCmd.Stdin = os.Stdin
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr
	}

	if err := execCmd.Run(); err != nil {
		logger.Error("exec in container %s failed: %v", containerName, err)
		return fmt.Errorf("exec in container %s failed: %w", containerName, err)
	}

	return nil
}
