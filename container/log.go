package container

import (
	"bufio"
	"fmt"
	"io"
	"litcontainer/config"
	"litcontainer/pkg/logger"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultContainerLogFileName = "container.log"
)

func PrintContainerLog(containerId string, follow bool) error {
	fullContainerId := ""
	dirs, err := os.ReadDir(config.DefaultLitContainerDir)
	if err != nil {
		logger.Error("Read container directory failed, err: %v", err)
		return fmt.Errorf("read container directory failed, %w", err)
	}
	for _, dir := range dirs {
		if dir.IsDir() && strings.HasPrefix(dir.Name(), containerId) {
			fullContainerId = dir.Name()
			break
		}
	}
	if fullContainerId == "" {
		logger.Error("Container %s not exists", containerId)
		return fmt.Errorf("container %s does not exist", containerId)
	}

	logDir := filepath.Join(config.DefaultLitContainerDir, fullContainerId)
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		logger.Error("Container log directory not exists, err: %v", err)
		return fmt.Errorf("log file for container %s does not exist, %w", containerId, err)
	}
	logFile := filepath.Join(logDir, DefaultContainerLogFileName)
	if _, err := os.Stat(logFile); err != nil && os.IsNotExist(err) {
		logger.Warn("Container log file not exists, err: %v", err)
		return nil
	}
	openedLogFile, err := os.Open(logFile)
	if err != nil {
		logger.Error("Open log file failed, err: %v", err)
		return fmt.Errorf("open log file failed, %w", err)
	}
	defer openedLogFile.Close()

	if follow {
		scanner := bufio.NewScanner(openedLogFile)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}

		reader := bufio.NewReader(openedLogFile)
		for {
			line, _, err := reader.ReadLine()
			if err != nil {
				if err == io.EOF {
					time.Sleep(time.Second * 1)
					continue
				}
				logger.Error("Read log file failed, err: %v", err)
				return fmt.Errorf("read log file failed, %w", err)
			}
			fmt.Println(string(line))
		}
	} else {
		content, err := io.ReadAll(openedLogFile)
		if err != nil {
			logger.Error("Read log file failed, err: %v", err)
			return fmt.Errorf("read log file failed, %w", err)
		}
		fmt.Println(string(content))
	}
	return nil
}
