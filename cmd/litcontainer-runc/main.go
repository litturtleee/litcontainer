package main

import (
	"fmt"
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

	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:  "log",
			Usage: "set the log file path where runc's diagnostic logs go",
		},
		&cli.StringFlag{
			Name:  "log-format",
			Usage: "set the log format ('text' or 'json')",
			Value: "text",
		},
	}

	// Before 在任何子命令执行前跑：把 logger 输出重定向到 --log 文件
	// 这样 shim 可以把 runc 的诊断和容器 stdout/stderr 分离到不同文件
	app.Before = func(c *cli.Context) error {
		logPath := c.String("log")
		if logPath == "" {
			return nil
		}
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("open runc log %s: %w", logPath, err)
		}
		// 注意：故意不 close —— 整个进程生命周期都需要这个 fd
		// 进程退出时 OS 自动回收
		logger.SetOutput(f)
		// --log-format 当前忽略，因为 logger 不支持 json
		return nil
	}

	app.Commands = []cli.Command{
		cli2.RuntimeRunCommand,
		cli2.RuntimeInitCommand,
		cli2.RuntimeCreateCommand,
		cli2.RuntimeStartCommand,
		cli2.RuntimeDeleteCommand,
		cli2.RuntimeExecContainerCommand,
	}

	if err := app.Run(os.Args); err != nil {
		logger.Error("App run Error: %v", err)
	}
}
