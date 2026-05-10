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

func init() {
	logger.SetLevel(logger.DEBUG)
	logger.SetIncludeTrace(true)
	logger.SetOutput(os.Stdout)
	logger.SetIncludePID(true)
}

func main() {
	// detach 阶段
	if os.Getenv("LIT_SHIM_DETACHED") != "1" {
		detach()
		return
	}

	// 解析参数
	bundlePath := flag.String("bundle", "", "path to the bundle")
	id := flag.String("id", "", "id of the container")
	socketPath := flag.String("socket", "", "path to the socket")
	logPath := flag.String("log", "", "path to the log")
	runcPath := flag.String("runc", "litcontainer-runc", "path to the runc binary")
	readyFd := flag.Int("ready-fd", -1, "fd to write READY signal (passed by daemon)")
	flag.Parse()

	if *bundlePath == "" || *id == "" || *socketPath == "" {
		logger.Fatal("bundle, id, socket are required")
	}
	shimDir := filepath.Join(shim.DefaultShimRoot, *id)
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

	// 日志重定向(上面之前的日志会打到dameon里, 之后的日志会打到shim.log里)
	logFile, err := os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		logger.Fatal("open log file: %v", err)
	}
	// 重定向dup日志
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
		logger.Fatal("remove stale socket: %v", err)
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
		listener.Close()
		os.RemoveAll(*socketPath)
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
	runcStartCmd.Stdout, runcStartCmd.Stderr = containerLogFile, containerLogFile
	if err := runcStartCmd.Run(); err != nil {
		_ = exec.Command(*runcPath, "delete", *id).Run()
		listener.Close()
		os.Remove(*socketPath)
		os.RemoveAll(shimDir)
		logger.Fatal("runc start: %v", err)
	}
	logger.Info("container started, id: %s", *id)

	// 通知daemon init已经启动
	if *readyFd >= 0 {
		f := os.NewFile(uintptr(*readyFd), "ready-fd")
		if _, err := f.Write([]byte("READY\n")); err != nil {
			logger.Error("write ready signal: %v", err)
		}
		f.Close()
	}

	// 启动server
	srv := shim.NewServer(*id, *runcPath, initPID, listener)
	go srv.WaitInit()
	go srv.Serve()

	// 等init退出
	<-srv.Done()
	logger.Info("init exited code=%d, cleaning up", srv.ExitCode())

	// 等dameon的delete命令
	<-srv.DeleteCh()
	logger.Info("delete command received, shutting down shim")

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

func detach() {
	cmd := exec.Command(os.Args[0], os.Args[1:]...)
	cmd.Env = append(os.Environ(), "LIT_SHIM_DETACHED=1")

	if readyFile := os.NewFile(3, "ready-fd"); readyFile != nil {
		cmd.ExtraFiles = []*os.File{readyFile}
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 启动孙子进程
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "shim detach: %v\n", err)
		os.Exit(1)
	}

	// 父进程正常退出
	os.Exit(0)
}
