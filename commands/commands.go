package commands

import (
	"fmt"
	"github.com/urfave/cli"
	"litcontainer/config"
	"litcontainer/container"
	"litcontainer/image"
	"litcontainer/pkg/logger"
	"sync"
)

var wg sync.WaitGroup

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
		&cli.BoolFlag{
			Name:  "d",
			Usage: "Run container in detached mode",
		},
		&cli.StringFlag{
			Name:  "name",
			Usage: "Assign a name to the container",
		},
		&cli.StringFlag{
			Name:  "m",
			Usage: "Memory limit for the container, e.g., 100m 1g",
		},
		&cli.StringFlag{
			Name:  "cpus",
			Usage: "CPU limit for the container, e.g., 1 1.5",
		},
		&cli.StringSliceFlag{
			Name:  "v",
			Usage: "Mount a volume, e.g., -v /tmp:/tmp -v /data:/data",
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
		detached := c.Bool("d")
		mountVolumes := c.StringSlice("v")
		containerName := c.String("name")
		memoryLimit := c.String("m")
		cpuLimit := c.String("cpus")

		if enableTTY && detached {
			logger.Error("it and d can not be used together")
			return fmt.Errorf("it and d can not be used together, %w", ErrInvalidArguments)
		}

		if containerName == "" {
			logger.Error("container name can not be empty")
			return fmt.Errorf("container name can not be empty, %w", ErrInvalidArguments)
		}

		logger.Debug(
			"enableTTY %s, memory limit: %s, cpu limit: %s, mountVolumes: %s, detached: %s", enableTTY, memoryLimit,
			cpuLimit, mountVolumes, detached,
		)
		// 调用container.Run
		if err := container.Run(args, enableTTY, detached, containerName, memoryLimit, cpuLimit, mountVolumes,
			&wg); err != nil {
			logger.Error("run command error: %v", err)
			return err
		}
		return nil
	},
}

var ExportCommand = cli.Command{
	Name:  "export",
	Usage: "Package the current running container into a tar file (docker export -o <tarfile> <imageName>)",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "o",
			Usage: "Output file name for the tar file",
		},
	},
	Action: func(c *cli.Context) error {
		output := c.String("o")
		if output == "" {
			output = "container"
		}
		if err := image.Export(output); err != nil {
			logger.Error("export command error: %v", err)
			return err
		}
		return nil
	},
}

var PsCommand = cli.Command{
	Name:  "ps",
	Usage: "List all running containers",
	Action: func(c *cli.Context) error {
		if err := config.PrintContainersInfo(); err != nil {
			logger.Error("ps command error: %v", err)
			return err
		}
		return nil
	},
}

func WaitAll() {
	wg.Wait()
}
