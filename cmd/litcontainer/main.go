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
	}

	if err := app.Run(os.Args); err != nil {
		logger.Error("App run Error: %v", err)
	}
}
