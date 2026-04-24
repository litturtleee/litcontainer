package main

import (
	"github.com/urfave/cli"
	cli2 "litcontainer/internal/cli"
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

	if err := network.Init(network.DefaultNetworkDBPath); err != nil {
		panic(err)
	}

	app := cli.NewApp()
	app.Name = version.AppName
	app.Usage = version.AppUsage
	app.Version = version.AppVersion

	app.Commands = []cli.Command{
		cli2.RunCommand,
		cli2.InitCommand,
		cli2.ExportCommand,
		cli2.PsCommand,
		cli2.LogCommand,
		cli2.ExecCommand,
		cli2.ExecContainerCommand,
		cli2.StopContainerCommand,
		cli2.RemoveContainerCommand,
		cli2.NetworkCommands,
	}

	if err := app.Run(os.Args); err != nil {
		logger.Error("App run Error: %v", err)
	}
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
