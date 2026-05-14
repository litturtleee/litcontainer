package cli

import (
	"encoding/json"
	"fmt"
	"github.com/urfave/cli"
	"litcontainer/internal/api/types"
	"litcontainer/internal/client"
	"litcontainer/internal/container"
	"litcontainer/internal/logger"
)

var CreateCommand = cli.Command{
	Name:  "create",
	Usage: "Create a container without starting it",
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "it", Usage: "Allocate a tty"},
		&cli.StringFlag{Name: "name", Usage: "Assign a name to the container"},
		&cli.StringFlag{Name: "m", Usage: "Memory limit, e.g., 100m 1g"},
		&cli.StringFlag{Name: "cpus", Usage: "CPU limit, e.g., 1 1.5"},
		&cli.StringSliceFlag{Name: "v", Usage: "Mount a volume"},
		&cli.StringSliceFlag{Name: "e", Usage: "Set environment variables"},
		&cli.StringFlag{Name: "net", Usage: "Assign a network"},
		&cli.StringSliceFlag{Name: "p", Usage: "Publish container's port(s) to host"},
	},
	Action: func(c *cli.Context) error {
		args := c.Args()
		if len(args) < 2 {
			return fmt.Errorf("create command needs at least two arguments (image + command), %w",
				client.ErrInvalidArguments)
		}
		containerName := c.String("name")
		if containerName == "" {
			return fmt.Errorf("container name can not be empty, %w", client.ErrInvalidArguments)
		}
		mounts, err := parseMountVolume(c.StringSlice("v"))
		if err != nil {
			return err
		}
		cliCl := client.NewClient()
		id, err := cliCl.ContainerCreate(types.ContainerCreateRequest{
			Name:         containerName,
			Image:        args[0],
			Command:      args[1:],
			Env:          c.StringSlice("e"),
			Mounts:       mounts,
			CPULimit:     c.String("cpus"),
			MemoryLimit:  c.String("m"),
			Network:      c.String("net"),
			PortMappings: c.StringSlice("p"),
			TTY:          c.Bool("it"),
		})
		if err != nil {
			return err
		}
		fmt.Println(id)
		return nil
	},
}

var StartCommand = cli.Command{
	Name:  "start",
	Usage: "Start one or more stopped (or created) containers",
	Action: func(c *cli.Context) error {
		if c.NArg() == 0 {
			return fmt.Errorf("at least one container name or ID must be specified, %w", client.ErrInvalidArguments)
		}
		idOrName := c.Args().First()
		cliCl := client.NewClient()
		if err := cliCl.StartContainer(idOrName); err != nil {
			return err
		}
		fmt.Println(idOrName)
		return nil
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
		&cli.StringSliceFlag{
			Name:  "e",
			Usage: "Set environment variables for the container, e.g., -e KEY=VALUE",
		},
		&cli.StringFlag{
			Name:  "net",
			Usage: "Assign a network to the container",
		},
		&cli.StringSliceFlag{
			Name:  "p",
			Usage: "Publish a container's port(s) to the host",
		},
	},
	// 解析命令并执行
	Action: func(c *cli.Context) error {
		// 获取参数列表
		args := c.Args()
		logger.Debug("run command args: %v.", args)
		if len(args) < 2 {
			logger.Error("run command need at least two argument")
			return fmt.Errorf("run command neeed at least two argument, %w", client.ErrInvalidArguments)
		}
		// 获取参数
		enableTTY := c.Bool("it")
		// todo:detach参数没有处理
		detached := c.Bool("d")
		mountVolumes := c.StringSlice("v")
		envs := c.StringSlice("e")
		containerName := c.String("name")
		memoryLimit := c.String("m")
		cpuLimit := c.String("cpus")
		imageName := args[0]
		network := c.String("net")
		portMappings := c.StringSlice("p")

		if enableTTY && detached {
			logger.Error("it and d can not be used together")
			return fmt.Errorf("it and d can not be used together, %w", client.ErrInvalidArguments)
		}

		if containerName == "" {
			logger.Error("container name can not be empty")
			return fmt.Errorf("container name can not be empty, %w", client.ErrInvalidArguments)
		}

		mounts, err := parseMountVolume(mountVolumes)
		if err != nil {
			logger.Error("parse mount volume error: %v", err)
			return err
		}

		logger.Debug(
			"enableTTY %v, memory limit: %s, cpu limit: %s, mountVolumes: %s, detached: %v, imageName: %s, containerName: %s",
			enableTTY, memoryLimit, cpuLimit, mountVolumes, detached, imageName, containerName,
		)
		// 调用container.Run
		cli := client.NewClient()
		id, err := cli.ContainerCreate(types.ContainerCreateRequest{
			Name:         containerName,
			Image:        imageName,
			Command:      args[1:],
			Env:          envs,
			Mounts:       mounts,
			CPULimit:     cpuLimit,
			MemoryLimit:  memoryLimit,
			Network:      network,
			PortMappings: portMappings,
			TTY:          enableTTY,
		})
		if err != nil {
			return err
		}
		err = cli.StartContainer(id)
		if err != nil {
			return err
		}
		if detached {
			fmt.Println(id)
			return nil
		}
		if err := cli.WaitContainer(id); err != nil {
			return err
		}
		logs, _ := cli.LogsContainer(id)
		fmt.Println(string(logs))
		return nil
	},
}

var PsCommand = cli.Command{
	Name:  "ps",
	Usage: "List all containers",
	Action: func(c *cli.Context) error {
		cli := client.NewClient()
		containers, err := cli.ListContainers()
		if err != nil {
			return err
		}
		err = printContainersInfo(containers)
		if err != nil {
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
		containerIDOrName := c.Args().First()

		follow := c.Bool("f")

		if follow {
			return fmt.Errorf("follow not support now")
		}

		cli := client.NewClient()
		logs, err := cli.LogsContainer(containerIDOrName)
		if err != nil {
			return err
		}
		fmt.Println(string(logs))
		return nil
	},
}

// todo:没改
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
			return fmt.Errorf("usage: litcontainer exec [-it] <name> <command> [args...], %w",
				client.ErrInvalidArguments)
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

// todo:没改
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
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "t",
			Usage: "Timeout in seconds",
			Value: 10,
		},
	},
	Action: func(c *cli.Context) error {
		if c.NArg() == 0 {
			logger.Error("at least one container name or ID must be specified")
			return fmt.Errorf("at least one container name or ID must be specified, %w", client.ErrInvalidArguments)
		}
		containerIdOrName := c.Args().First()
		if len(containerIdOrName) == 0 {
			logger.Error("container name cannot be empty")
			return fmt.Errorf("container name cannot be empty, %w", client.ErrInvalidArguments)
		}
		timeout := c.Int("t")

		cli := client.NewClient()
		return cli.StopContainer(containerIdOrName, timeout)
	},
}

var RemoveContainerCommand = cli.Command{
	Name:  "rm",
	Usage: "Remove a container",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "f",
			Usage: "Force remove a running container",
		},
	},
	Action: func(c *cli.Context) error {
		if c.NArg() == 0 {
			logger.Error("at least one container name or ID must be specified")
			return fmt.Errorf("at least one container name or ID must be specified, %w", client.ErrInvalidArguments)
		}
		force := c.Bool("f")
		containerIdOrName := c.Args().First()
		if len(containerIdOrName) == 0 {
			logger.Error("container name cannot be empty")
			return fmt.Errorf("container name cannot be empty, %w", client.ErrInvalidArguments)
		}

		cli := client.NewClient()
		return cli.RemoveContainer(containerIdOrName, force)
	},
}

var InspectContainerCommand = cli.Command{
	Name:  "inspect",
	Usage: "Inspect a container",
	Action: func(c *cli.Context) error {
		if c.NArg() == 0 {
			logger.Error("at least one container name or ID must be specified")
			return fmt.Errorf("at least one container name or ID must be specified, %w", client.ErrInvalidArguments)
		}
		containerIdOrName := c.Args().First()
		if len(containerIdOrName) == 0 {
			logger.Error("container name cannot be empty")
			return fmt.Errorf("container name cannot be empty, %w", client.ErrInvalidArguments)
		}
		cli := client.NewClient()
		inspectContainer, err := cli.InspectContainer(containerIdOrName)
		if err != nil {
			return err
		}
		configByte, _ := json.Marshal(inspectContainer)
		fmt.Println(string(configByte))
		return nil
	},
}
