package main

import (
	"github.com/urfave/cli"
	commands2 "litcontainer/internal/cli/commands"
	"litcontainer/internal/db"
	"litcontainer/internal/logger"
	"litcontainer/internal/network"
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
	logger.Info("LitContainer start")
	if !isInitOrExecContainer() {
		InitBoltDB()
	}

	app := cli.NewApp()
	app.Name = version.AppName
	app.Usage = version.AppUsage
	app.Version = version.AppVersion

	app.Commands = []cli.Command{
		commands2.RunCommand,
		commands2.InitCommand,
		commands2.ExportCommand,
		commands2.PsCommand,
		commands2.LogCommand,
		commands2.ExecCommand,
		commands2.ExecContainerCommand,
		commands2.StopContainerCommand,
		commands2.RemoveContainerCommand,
		commands2.NetworkCommands,
	}

	if err := app.Run(os.Args); err != nil {
		logger.Error("App run Error: %v", err)
	}

	// 阻塞等待直到所有容器退出
	commands2.WaitAll()
}

func InitBoltDB() {
	err := db.WithBoltDB(network.DefaultNetworkDBPath, func(dbClient *db.BoltDB) error {
		if bucketErr := dbClient.CreateBucketIfNotExists(network.DefaultNetworkTable); bucketErr != nil {
			return bucketErr
		}
		if bucketErr := dbClient.CreateBucketIfNotExists(network.AllocatedIPKeyTable); bucketErr != nil {
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
