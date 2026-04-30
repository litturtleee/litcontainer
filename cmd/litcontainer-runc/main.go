package main

import (
	"github.com/urfave/cli"
	cli2 "litcontainer/internal/cli"
	"litcontainer/internal/logger"
	"litcontainer/internal/version"
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
	app.Name = version.AppName
	app.Usage = version.AppUsage
	app.Version = version.AppVersion

	app.Commands = []cli.Command{
		cli2.RuntimeRunCommand,
		cli2.RuntimeInitCommand,
	}

	if err := app.Run(os.Args); err != nil {
		logger.Error("App run Error: %v", err)
	}
}
