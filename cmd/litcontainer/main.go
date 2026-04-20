package main

import (
	"github.com/urfave/cli"
	"litcontainer/commands"
	"litcontainer/enum"
	"litcontainer/pkg/db"
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
	logger.Info("LitContainer start")
	if !isInitOrExecContainer() {
		InitBoltDB()
	}

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
		commands.NetworkCommands,
	}

	if err := app.Run(os.Args); err != nil {
		logger.Error("App run Error: %v", err)
	}

	// 阻塞等待直到所有容器退出
	commands.WaitAll()
}

func InitBoltDB() {
	err := db.WithBoltDB(enum.DefaultNetworkDBPath, func(dbClient *db.BoltDB) error {
		if bucketErr := dbClient.CreateBucketIfNotExists(enum.DefaultNetworkTable); bucketErr != nil {
			return bucketErr
		}
		if bucketErr := dbClient.CreateBucketIfNotExists(enum.AllocatedIPKeyTable); bucketErr != nil {
			return bucketErr
		}
		return nil
	})
	if err != nil {
		logger.Error("init bolt db failed: %v", err)
		panic(err)
	}
}

// 这两个是子进程避免重复加载DB
func isInitOrExecContainer() bool {
	return len(os.Args) > 1 && (os.Args[1] == "init" || os.Args[1] == "exec")
}
