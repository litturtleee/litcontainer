package main

import (
	"flag"
	"fmt"
	"io"
	"litcontainer/internal/container"
	"litcontainer/internal/logger"
	"litcontainer/internal/shim"
	"litcontainer/internal/stdcopy"
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

// shimConfig 持有 shim 运行所需的全部参数和路径
type shimConfig struct {
	bundlePath       string
	id               string
	socketPath       string
	runcPath         string
	readyFd          int
	shimDir          string
	logPath          string // shim 自己的日志
	runcLogPath      string // runc 自己的诊断日志
	initPIDPath      string
	containerLogPath string // 容器 init 的 stdout/stderr
}

func main() {
	// detach 阶段
	if os.Getenv("LIT_SHIM_DETACHED") != "1" {
		detach()
		return
	}

	// 解析参数
	cfg := parseFlags()
	if err := runShim(cfg); err != nil {
		logger.Fatal("run shim: %v", err)
	}
}

// --- 内部方法 ---

// --- shim ----

func runShim(cfg *shimConfig) error {
	// 1. setsid, 创建 shimDir
	if err := setupSession(cfg); err != nil {
		return fmt.Errorf("setup session: %w", err)
	}
	// 2. 重定向 shim 日志
	if err := redirectShimLog(cfg); err != nil {
		return fmt.Errorf("redirect shim log: %w", err)
	}
	// 3. 写 shim.pid
	if err := writeShimPid(cfg); err != nil {
		return fmt.Errorf("write shim pid: %w", err)
	}
	// 4. 创建 socket
	listener, err := openSocket(cfg)
	if err != nil {
		return fmt.Errorf("open socket: %w", err)
	}

	// 5. 打开 container.log
	containerLogFile, err := openContainerLog(cfg)
	if err != nil {
		return fmt.Errorf("open container log: %w", err)
	}
	defer containerLogFile.Close()

	// 6. 设置子reaper,目的是让孙子进程的父进程退出后,父进程不会变成1号进程而是变成shim
	setSubreaper()

	// 7.runc create 和 start
	initPID, err := runcCreate(cfg, containerLogFile)
	if err != nil {
		return fmt.Errorf("runc create: %w", err)
	}
	if err := runcStart(cfg); err != nil {
		// init 已经创建，必须 runc delete 清状态
		_ = exec.Command(cfg.runcPath, "delete", cfg.id).Run()
		return err
	}
	logger.Info("container started, id: %s", cfg.id)

	// 8. 通知 daemon init 已经启动
	notifyReady(cfg.readyFd)

	srv := shim.NewServer(cfg.id, cfg.runcPath, initPID, listener)
	go srv.WaitInit()
	go srv.Serve()

	// 等 init 退出
	<-srv.Done()
	logger.Info("init exited code=%d, cleaning up", srv.ExitCode())

	// 等 daemon 的 delete
	<-srv.DeleteCh()
	logger.Info("delete command received, shutting down shim")

	srv.Shutdown()
	if err := srv.RuncDelete(); err != nil {
		logger.Error("runc delete: %v", err)
	}

	logger.Info("shim exited, id: %s", cfg.id)
	cleanupShimRuntime(cfg, listener)
	return nil
}

func parseFlags() *shimConfig {
	bundlePath := flag.String("bundle", "", "path to the bundle")
	id := flag.String("id", "", "id of the container")
	socketPath := flag.String("socket", "", "path to the socket")
	logPath := flag.String("log", "", "path to the shim log")
	runcPath := flag.String("runc", "litcontainer-runc", "path to the runc binary")
	readyFd := flag.Int("ready-fd", -1, "fd to write READY signal (passed by daemon)")
	flag.Parse()

	if *bundlePath == "" || *id == "" || *socketPath == "" {
		logger.Fatal("bundle, id, socket are required")
	}

	cfg := &shimConfig{
		bundlePath: *bundlePath,
		id:         *id,
		socketPath: *socketPath,
		runcPath:   *runcPath,
		readyFd:    *readyFd,
	}
	cfg.shimDir = filepath.Join(shim.DefaultShimRoot, cfg.id)
	if *logPath != "" {
		cfg.logPath = *logPath
	} else {
		cfg.logPath = filepath.Join(cfg.shimDir, "shim.log")
	}
	cfg.runcLogPath = filepath.Join(cfg.shimDir, "runc.log")
	cfg.initPIDPath = filepath.Join(cfg.shimDir, "init.pid")
	cfg.containerLogPath = filepath.Join(container.DefaultLitContainerDir, cfg.id,
		container.DefaultContainerLogFileName)
	return cfg
}

// setupSession 处理 setsid 并创建 shimDir
func setupSession(cfg *shimConfig) error {
	// setsid EPERM 表示已经是 session leader
	if _, err := syscall.Setsid(); err != nil && err != syscall.EPERM {
		return fmt.Errorf("setsid: %w", err)
	}
	if err := os.MkdirAll(cfg.shimDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", cfg.shimDir, err)
	}
	return nil
}

// redirectShimLog 把 shim 自己的 stdout/stderr dup 到 shim.log
func redirectShimLog(cfg *shimConfig) error {
	logFile, err := os.OpenFile(cfg.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open shim log: %w", err)
	}
	if err := syscall.Dup3(int(logFile.Fd()), int(os.Stdout.Fd()), 0); err != nil {
		return fmt.Errorf("dup3 stdout: %w", err)
	}
	if err := syscall.Dup3(int(logFile.Fd()), int(os.Stderr.Fd()), 0); err != nil {
		return fmt.Errorf("dup3 stderr: %w", err)
	}
	return nil
}

func writeShimPid(cfg *shimConfig) error {
	pidPath := filepath.Join(cfg.shimDir, "shim.pid")
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0644); err != nil {
		return fmt.Errorf("write shim.pid: %w", err)
	}
	return nil
}

// openSocket 创建 unix socket，并开始监听
// daemon 通过这个 socket 和 shim 通信
func openSocket(cfg *shimConfig) (net.Listener, error) {
	if err := os.Remove(cfg.socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove stale socket: %w", err)
	}
	l, err := net.Listen("unix", cfg.socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen unix: %w", err)
	}
	logger.Info("shim started, id: %s, pid: %d", cfg.id, os.Getpid())
	return l, nil
}

func openContainerLog(cfg *shimConfig) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.containerLogPath), 0755); err != nil {
		return nil, fmt.Errorf("mkdir container log dir: %w", err)
	}
	f, err := os.OpenFile(cfg.containerLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open container log: %w", err)
	}
	return f, nil
}

// setSubreaper 让 shim 成为子 reaper，这样它就能监控 init 的退出
// 通过系统调用prctl PR_SET_CHILD_SUBREAPER实现，调用后shim会成为一个子reaper，能够监控它的子进程（也就是init）的退出状态。
// 当init退出时，shim可以通过waitpid等系统调用获取init的退出状态，并进行相应的清理工作。
func setSubreaper() {
	// 让shim成为子reaper，这样它就能监控init的退出
	if _, _, err := syscall.Syscall6(syscall.SYS_PRCTL, uintptr(syscall.PR_SET_CHILD_SUBREAPER), 1, 0, 0, 0,
		0); err != 0 {
		logger.Fatal("prctl PR_SET_CHILD_SUBREAPER: %v", err)
	}
}

// cleanupShimRuntime 负责清理 shim 运行时的资源，包括关闭 socket，删除 shimDir 等
func cleanupShimRuntime(cfg *shimConfig, listener net.Listener) {
	if listener != nil {
		listener.Close()
	}
	os.Remove(cfg.socketPath)
	os.RemoveAll(cfg.shimDir)
}

// --- runc ---
func runcCreate(cfg *shimConfig, containerLog *os.File) (int, error) {
	stdcopyWriter := stdcopy.NewWriter(containerLog)
	// write给cmd, shim读read, 然后把read的copy给stdcopyWriter, 这样日志就写到了文件里
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return 0, fmt.Errorf("create stdout pipe: %w", err)
	}
	defer stdoutW.Close()
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		stdoutR.Close()
		return 0, fmt.Errorf("create stderr pipe: %w", err)
	}
	defer stderrW.Close()

	cmd := exec.Command(cfg.runcPath,
		"--log", cfg.runcLogPath,
		"--log-format", "json",
		"create",
		"--bundle", cfg.bundlePath,
		"--pid-file", cfg.initPIDPath,
		cfg.id)

	// 进程会把fd继承给init
	cmd.Stdout, cmd.Stderr = stdoutW, stderrW

	if err := cmd.Run(); err != nil {
		stdoutR.Close()
		stderrR.Close()
		return 0, fmt.Errorf("start runc create: %w", err)
	}

	// 读端交由goroutine处理
	go forwardToMuxer(stdcopyWriter.Stdout(), stdoutR, "stdout")
	go forwardToMuxer(stdcopyWriter.Stderr(), stderrR, "stderr")

	return readInitPid(cfg.initPIDPath)
}

func runcStart(cfg *shimConfig) error {
	cmd := exec.Command(cfg.runcPath,
		"--log", cfg.runcLogPath,
		"--log-format", "json",
		"start",
		cfg.id)

	// 这里的日志跟随shim重定向
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("start runc start: %w", err)
	}
	return nil
}

func forwardToMuxer(dst io.Writer, src *os.File, name string) {
	// 读端close
	defer src.Close()
	if _, err := io.Copy(dst, src); err != nil {
		logger.Warn("copy init %s: %v", name, err)
	}
}

func readInitPid(pidFile string) (int, error) {
	if _, err := os.Stat(pidFile); err != nil {
		return 0, fmt.Errorf("stat init.pid: %w", err)
	}
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, fmt.Errorf("read init.pid: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("invalid init pid: %s", string(pidBytes))
	}
	logger.Info("init process started, pid: %d", pid)
	return pid, nil
}

// 通知daemon shim已经启动完毕
func notifyReady(fd int) {
	if fd < 0 {
		return
	}

	f := os.NewFile(uintptr(fd), "ready-fd")
	if _, err := f.Write([]byte("READY\n")); err != nil {
		logger.Error("write ready signal: %v", err)
	}
	f.Close()
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
