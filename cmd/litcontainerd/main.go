package main

import (
	"context"
	"github.com/gin-gonic/gin"
	"litcontainer/internal/api/routes"
	"litcontainer/internal/container"
	"litcontainer/internal/daemon"
	"litcontainer/internal/logger"
	"litcontainer/internal/network"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

const (
	root       = "/var/lib/litcontainer"
	socketPath = "/var/run/litcontainer.sock"
)

func main() {
	// ★ 子进程入口：识别 init 参数，跑容器初始化后立即返回
	if len(os.Args) > 1 && os.Args[1] == "init" {
		if err := container.InitContainerProcess(); err != nil {
			logger.Error("container init failed: %v", err)
			os.Exit(1)
		}
		return
	}

	err := network.Init(network.DefaultNetworkDBPath)
	if err != nil {
		panic(err)
	}

	d, err := daemon.New(root)
	if err != nil {
		panic(err)
	}
	logger.Info("daemon start, root: %s", root)

	r := gin.Default()
	routes.SetupRoutes(r, d)

	// 清理可能残留的Socket文件
	if err := os.RemoveAll(socketPath); err != nil {
		logger.Error("remove stale socket: %v", err)
	}

	// 创建 Unix listener
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		logger.Error("listen unix: %v", err)
		os.Exit(1)
	}

	// 调权限
	if err := os.Chmod(socketPath, 0660); err != nil {
		logger.Error("chmod socket: %v", err)
	}

	// 4. 注册信号处理优雅关闭
	srv := &http.Server{Handler: r}
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		logger.Info("received signal %v, shutting down", sig)
		srv.Shutdown(context.Background())
	}()

	logger.Info("listening on %s", socketPath)
	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		logger.Error("serve: %v", err)
		os.Exit(1)
	}

	// 5. socket 文件清理
	os.Remove(socketPath)
}
