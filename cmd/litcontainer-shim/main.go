package main

import (
	"flag"
	"fmt"
	"litcontainer/internal/container"
	"litcontainer/internal/logger"
	"litcontainer/internal/shim"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const (
	DefaultShimRoot = "/run/litcontainer/shim"
)

func init() {
	logger.SetLevel(logger.DEBUG)
	logger.SetIncludeTrace(true)
	logger.SetOutput(os.Stdout)
	logger.SetIncludePID(true)
}

func main() {
	// 解析参数
	bundlePath := flag.String("bundle", "", "path to the bundle")
	id := flag.String("id", "", "id of the container")
	socketPath := flag.String("socket", "", "path to the socket")
	logPath := flag.String("log", "", "path to the log")
	runcPath := flag.String("runc", "litcontainer-runc", "path to the runc binary")
	flag.Parse()

	if *bundlePath == "" || *id == "" || *socketPath == "" {
		logger.Fatal("bundle, id, socket are required")
	}
	shimDir := filepath.Join(DefaultShimRoot, *id)
	if *logPath == "" {
		*logPath = filepath.Join(shimDir, "shim.log")
	}

	// setsid EPERM 错误表示已经是session leader了, 这种情况不需要再setsid了
	if _, err := syscall.Setsid(); err != nil && err != syscall.EPERM {
		logger.Fatal("setsid: %v", err)
	}

	if err := os.MkdirAll(shimDir, 0755); err != nil {
		logger.Fatal("mkdir %s failed: %v", shimDir, err)
	}

	// 日志重定向
	logFile, err := os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		logger.Fatal("open log file: %v", err)
	}
	if err := syscall.Dup3(int(logFile.Fd()), int(os.Stdout.Fd()), 0); err != nil {
		logger.Fatal("dup3 stdout: %v", err)
	}
	if err := syscall.Dup3(int(logFile.Fd()), int(os.Stderr.Fd()), 0); err != nil {
		logger.Fatal("dup3 stderr: %v", err)
	}

	// 生成shim.pid
	pidPath := filepath.Join(shimDir, "shim.pid")
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())),
		0644); err != nil {
		logger.Fatal("write shim.pid: %v", err)
	}

	// 创建socket
	if err := os.Remove(*socketPath); err != nil && !os.IsNotExist(err) {
		logger.Error("remove stale socket: %v", err)
	}
	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		logger.Fatal("listen unix: %v", err)
	}
	logger.Info("shim started, id: %s, pid: %d", *id, os.Getpid())

	// runc create
	initPIDPath := filepath.Join(shimDir, "init.pid")
	runcCreateCmd := exec.Command(*runcPath, "create", "--bundle", *bundlePath, "--pid-file", initPIDPath, *id)

	// runc日志全定向到container.log(相当于runc和容器的日志混合)
	containerLogPath := filepath.Join(container.DefaultLitContainerDir, *id, container.DefaultContainerLogFileName)
	if err := os.MkdirAll(filepath.Dir(containerLogPath), 0755); err != nil {
		logger.Fatal("mkdir %s failed: %v", filepath.Dir(containerLogPath), err)
	}
	containerLogFile, err := os.OpenFile(containerLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		logger.Fatal("create container log file: %v", err)
	}
	defer containerLogFile.Close()
	runcCreateCmd.Stdout, runcCreateCmd.Stderr = containerLogFile, containerLogFile

	// 让shim成为create, init的父进程, 这样才能监控init的退出
	if _, _, err := syscall.Syscall6(syscall.SYS_PRCTL, uintptr(syscall.PR_SET_CHILD_SUBREAPER), 1, 0, 0, 0,
		0); err != 0 {
		logger.Fatal("prctl PR_SET_CHILD_SUBREAPER: %v", err)
	}

	if err := runcCreateCmd.Run(); err != nil {
		os.RemoveAll(shimDir)
		logger.Fatal("runc create: %v", err)
	}

	// 读init.pid
	if _, err := os.Stat(initPIDPath); err != nil {
		logger.Fatal("stat init.pid: %v", err)
	}
	initPIDByte, err := os.ReadFile(initPIDPath)
	if err != nil {
		logger.Fatal("read init.pid: %v", err)
	}
	initPID, err := strconv.Atoi(strings.TrimSpace(string(initPIDByte)))
	if err != nil || initPID <= 0 {
		logger.Fatal("invalid init pid: %s", string(initPIDByte))
	}
	logger.Info("init process started, id: %s, pid: %s", *id, string(initPIDByte))

	// runc start
	runcStartCmd := exec.Command(*runcPath, "start", *id)
	// runc的日志就重定向到shim里
	runcStartCmd.Stdout, runcStartCmd.Stderr = logFile, logFile
	if err := runcStartCmd.Run(); err != nil {
		_ = exec.Command(*runcPath, "delete", *id).Run()
		os.RemoveAll(shimDir)
		logger.Fatal("runc start: %v", err)
	}
	logger.Info("container started, id: %s", *id)

	// 启动server
	srv := shim.NewServer(*id, *runcPath, initPID, listener)
	go srv.WaitInit()
	go srv.Serve()

	// 等init退出
	<-srv.Done()
	logger.Info("init exited code=%d, cleaning up", srv.ExitCode())

	srv.Shutdown()

	// 删除容器状态文件
	if err := srv.RuncDelete(); err != nil {
		logger.Error("runc delete: %v", err)
	}

	os.Remove(*socketPath)
	os.RemoveAll(shimDir)

	logger.Info("shim exited, id: %s", *id)
	return
}
