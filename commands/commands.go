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
	Name:   "init",
	Usage:  "Init container process run user's process in container. Do not call it outside",
	Hidden: true,
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

var LogCommand = cli.Command{
	Name:  "log",
	Usage: "Show the log of a container",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "f",
			Usage: "Follow the log",
		},
	},
	Action: func(c *cli.Context) error {
		containerID := c.Args().First()
		if len(containerID) < 12 {
			logger.Error("container id is invalid")
			return fmt.Errorf("container id is invalid, %w", ErrInvalidArguments)
		}
		follow := c.Bool("f")
		if err := container.PrintContainerLog(containerID, follow); err != nil {
			logger.Error("log command error: %v", err)
			return err
		}
		return nil
	},
}

var ExecCommand = cli.Command{
	Name:  "exec",
	Usage: "Execute a command in a running container",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "it",
			Usage: "Run in interactive mode",
		},
	},
	Action: func(c *cli.Context) error {
		if len(c.Args()) < 2 {
			return fmt.Errorf("usage: litcontainer exec [-it] <name> <command> [args...], %w", ErrInvalidArguments)
		}
		enableTTY := c.Bool("it")
		containerName := c.Args().Get(0)
		args := c.Args()[1:]

		if err := container.Exec(enableTTY, containerName, args); err != nil {
			logger.Error("exec command error: %v", err)
			return err
		}
		return nil
	},
}

var ExecContainerCommand = cli.Command{
	Name:   "exec-container",
	Usage:  "Execute a command in a running container, Do not call it outside",
	Hidden: true,
	Action: func(c *cli.Context) error {
		if err := container.ExecContainer(c.Args()); err != nil {
			logger.Error("exec-container command error: %v", err)
			return err
		}
		return nil
	},
}

var StopContainerCommand = cli.Command{
	Name:  "stop",
	Usage: "Stop a running container",
	Action: func(c *cli.Context) error {
		if c.NArg() == 0 {
			logger.Error("at least one container name or ID must be specified")
			return fmt.Errorf("at least one container name or ID must be specified, %w", ErrInvalidArguments)
		}
		containerIdOrName := c.Args().First()
		if len(containerIdOrName) == 0 {
			logger.Error("container name cannot be empty")
			return fmt.Errorf("container name cannot be empty, %w", ErrInvalidArguments)
		}
		if err := container.StopContainer(containerIdOrName); err != nil {
			logger.Error("stop command error: %v", err)
			return err
		}
		return nil
	},
}

func WaitAll() {
	wg.Wait()
}
