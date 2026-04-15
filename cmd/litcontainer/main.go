package main

import (
	"github.com/urfave/cli"
	"litcontainer/commands"
	"litcontainer/enum"
	"litcontainer/pkg/logger"
	"os"
)

func init() {
	logger.SetLevel(logger.DEBUG)
	logger.SetIncludeTrace(true)
	logger.SetOutput(os.Stdout)
	logger.SetIncludePID(true)
}

func main() {
	app := cli.NewApp()
	app.Name = enum.AppName
	app.Usage = enum.AppUsage
	app.Version = enum.AppVersion

	app.Commands = []cli.Command{
		commands.RunCommand,
		commands.InitCommand,
		commands.ExportCommand,
		commands.PsCommand,
		commands.LogCommand,
		commands.ExecCommand,
		commands.ExecContainerCommand,
		commands.StopContainerCommand,
		commands.RemoveContainerCommand,
	}

	if err := app.Run(os.Args); err != nil {
		logger.Error("App run Error: %v", err)
	}

	// 阻塞等待直到所有容器退出
	commands.WaitAll()
}
