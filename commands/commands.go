package commands

import (
	"fmt"
	"github.com/urfave/cli"
	"litcontainer/container"
	"litcontainer/pkg/logger"
)

var InitCommand = cli.Command{
	Name:  "init",
	Usage: "Init container process run user's process in container. Do not call it outside",
	Action: func(c *cli.Context) error {
		logger.Debug("init command, args: %v", c.Args())
		return container.InitContainerProcess()
	},
}

var RunCommand = cli.Command{
	Name:  "run",
	Usage: "Run a container",
	// 设置命令行参数
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "it",
			Usage: "Run in interactive mode",
		},
		&cli.StringFlag{
			Name:  "m",
			Usage: "Memory limit for the container, e.g., 100m 1g",
		},
		&cli.StringFlag{
			Name:  "cpus",
			Usage: "CPU limit for the container, e.g., 1 1.5",
		},
	},
	// 解析命令并执行
	Action: func(c *cli.Context) error {
		// 获取参数列表
		args := c.Args()
		logger.Debug("run command args: %v.", args)
		if len(args) == 0 {
			logger.Error("run command need at least one argument")
			return fmt.Errorf("run command neeed at least one argument, %w", ErrInvalidArguments)
		}
		// 获取参数
		enableTTY := c.Bool("it")
		memoryLimit := c.String("m")
		cpuLimit := c.String("cpus")
		logger.Debug("enableTTY %s, memory limit: %s, cpu limit: %s", enableTTY, memoryLimit, cpuLimit)
		// 调用container.Run
		if err := container.Run(args, enableTTY, memoryLimit, cpuLimit); err != nil {
			logger.Error("run command error: %v", err)
			return err
		}
		return nil
	},
}
