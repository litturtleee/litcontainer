package shim

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"litcontainer/internal/logger"
	"litcontainer/internal/runtime"
	"litcontainer/internal/stdcopy"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const RuntimeRoot = "/run/litcontainer-runc"

type Server struct {
	id       string
	runcPath string
	initPid  int
	listener net.Listener

	mu       sync.Mutex
	exited   bool // 避免重复设置
	exitCode int
	doneCh   chan struct{} // close 时表示 init 已退出

	deleteCh   chan struct{}
	deleteOnce sync.Once

	wg sync.WaitGroup
}

func NewServer(id, runcPath string, initPid int, listener net.Listener) *Server {
	return &Server{
		id:       id,
		runcPath: runcPath,
		initPid:  initPid,
		listener: listener,
		doneCh:   make(chan struct{}),
		deleteCh: make(chan struct{}),
	}
}

// WaitInit
// 阻塞直到init退出
func (s *Server) WaitInit() {
	var ws syscall.WaitStatus
	for {
		_, err := syscall.Wait4(s.initPid, &ws, 0, nil)
		// 表示系统调用被中断,并不是子进程退出了
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			logger.Error("wait4 init %d: %v", s.initPid, err)
			s.setExit(-1)
			return
		}
		break
	}
	code := 0
	switch {
	// 正常退出
	case ws.Exited():
		code = ws.ExitStatus()
	// 被信号终止
	case ws.Signaled():
		// 128+信号值是unix标准
		code = 128 + int(ws.Signal())
	}
	logger.Info("init %d exited with code %d", s.initPid, code)
	s.setExit(code)
}

func (s *Server) RuncDelete() error {
	cmd := exec.Command(s.runcPath, "delete", s.id)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error("runc delete failed: %v, output: %s", err, string(output))
		return fmt.Errorf("runc delete failed: %v, output: %s", err, string(output))
	}
	return nil
}

func (s *Server) Shutdown() {
	// 拒绝新请求
	s.listener.Close()
	// 等待正在处理的请求完成
	s.wg.Wait()
}

func (s *Server) Done() <-chan struct{} {
	return s.doneCh
}

func (s *Server) ExitCode() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitCode
}

func (s *Server) DeleteCh() <-chan struct{} {
	return s.deleteCh
}

// Server 启动服务器，处理来自daemon的请求
func (s *Server) Serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			logger.Warn("accept connection: %v", err)
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(conn)
		}()
	}
}

// --- 内部方法 ---
func (s *Server) setExit(code int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exited {
		return
	}
	s.exited = true
	s.exitCode = code
	close(s.doneCh)
}

func (s *Server) writeResponse(conn net.Conn, resp Response) error {
	rspByte, err := json.Marshal(resp)
	if err != nil {
		logger.Error("marshal response: %v", err)
		return err
	}
	// 加个换行符作为结束符
	rspByte = append(rspByte, '\n')
	_, err = conn.Write(rspByte)
	return err
}

func (s *Server) handleConn(conn net.Conn) {
	// 处理完请求后会关闭连接，客户端从这里感知
	defer conn.Close()

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		if !errors.Is(err, io.EOF) {
			logger.Warn("read request: %v", err)
		}
		return
	}

	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		s.writeResponse(conn, Response{OK: false, Error: fmt.Sprintf("invalid request: %v", err)})
		return
	}

	// exec命令单独处理
	if req.Cmd == CmdExec {
		s.handleExec(conn, reader, req.Args)
		return
	}

	rsp := s.dispatch(&req)
	if err := s.writeResponse(conn, *rsp); err != nil {
		logger.Error("write response: %v", err)
		return
	}
}

func (s *Server) dispatch(req *Request) *Response {
	switch req.Cmd {
	case CmdState:
		return s.handleState()
	case CmdStop:
		return s.handleStop(req.Args)
	case CmdWait:
		return s.handleWait()
	case CmdKill:
		return s.handleKill(req.Args)
	case CmdDelete:
		return s.handleDelete()
	default:
		return &Response{OK: false, Error: fmt.Sprintf("unknown command: %s", req.Cmd)}
	}
}

// handleState 返回当前容器状态(runtimeState)
func (s *Server) handleState() *Response {
	statePath := filepath.Join(RuntimeRoot, s.id, "state.json")
	stateBytes, err := os.ReadFile(statePath)
	if err != nil {
		return &Response{OK: false, Error: fmt.Sprintf("read state: %v", err)}
	}

	var containerState runtime.ContainerState
	if err := json.Unmarshal(stateBytes, &containerState); err != nil {
		return &Response{OK: false, Error: fmt.Sprintf("unmarshal state: %v", err)}
	}

	data := StateData{
		ID:     containerState.ID,
		Pid:    containerState.PID,
		Status: containerState.Status,
	}

	// 存在已stop, 但是state.json还没更新的情况，优先返回exitCode和stopped状态
	s.mu.Lock()
	if s.exited {
		data.Exit = s.exitCode
		data.Status = runtime.StateStopped
	}
	s.mu.Unlock()

	dataBytes, _ := json.Marshal(data)
	return &Response{OK: true, Data: dataBytes}
}

// handleStop 发送 SIGTERM 给 init 进程，优雅停止容器
func (s *Server) handleStop(rawArgs json.RawMessage) *Response {
	// 默认停止参数
	args := StopArgs{Signal: int(syscall.SIGTERM), Timeout: 10}
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return &Response{OK: false, Error: fmt.Sprintf("invalid stop args: %v", err)}
		}
		if args.Signal == 0 {
			args.Signal = int(syscall.SIGTERM)
		}
		if args.Timeout <= 0 {
			args.Timeout = 10
		}
	}

	// 检查是否已经退出了
	select {
	case <-s.doneCh:
		return &Response{OK: true}
	default:
	}
	logger.Info("sending signal %d to init %d", args.Signal, s.initPid)

	// 发送信号
	if err := syscall.Kill(s.initPid, syscall.Signal(args.Signal)); err != nil {
		// ESRCH 表示进程不存在了，认为已经停止了
		if err != syscall.ESRCH {
			return &Response{OK: false, Error: fmt.Sprintf("kill init: %v", err)}
		}
	}

	// 等待timeout
	select {
	case <-s.doneCh:
		return &Response{OK: true}
	case <-time.After(time.Duration(args.Timeout) * time.Second):
		_ = syscall.Kill(s.initPid, syscall.SIGKILL)
		<-s.doneCh
		logger.Info("init did not exit after %d seconds, sent SIGKILL", args.Timeout)
		return &Response{OK: true}
	}
}

// handleWait 阻塞直到 init 进程退出
func (s *Server) handleWait() *Response {
	<-s.doneCh
	data, _ := json.Marshal(StateData{
		ID:     s.id,
		Pid:    s.initPid,
		Status: runtime.StateStopped,
		Exit:   s.exitCode,
	})
	return &Response{OK: true, Data: data}
}

// handleKill 发送指定信号给 init 进程
// 没有超时，直接kill信号发送给init
func (s *Server) handleKill(rawArgs json.RawMessage) *Response {
	args := KillArgs{Signal: int(syscall.SIGKILL)}
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return &Response{OK: false, Error: fmt.Sprintf("invalid kill args: %v", err)}
		}
		if args.Signal == 0 {
			args.Signal = int(syscall.SIGKILL)
		}
	}

	// init已退出
	select {
	case <-s.doneCh:
		return &Response{OK: true}
	default:
		logger.Info("sending signal %d to init %d", args.Signal, s.initPid)
	}

	if err := syscall.Kill(s.initPid, syscall.Signal(args.Signal)); err != nil {
		if err != syscall.ESRCH {
			return &Response{OK: false, Error: fmt.Sprintf("kill init: %w", err)}
		}
	}
	logger.Info("signal %d sent to init %d", args.Signal, s.initPid)
	return &Response{OK: true}
}

func (s *Server) handleDelete() *Response {
	s.deleteOnce.Do(func() {
		close(s.deleteCh)
	})
	return &Response{OK: true}
}

func (s *Server) handleExec(conn net.Conn, reader *bufio.Reader, rawArgs json.RawMessage) {
	var args ExecArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		s.writeResponse(conn, Response{OK: false, Error: fmt.Sprintf("invalid exec args: %v", err)})
		return
	}
	if len(args.Cmd) == 0 {
		s.writeResponse(conn, Response{OK: false, Error: "empty command"})
		return
	}

	select {
	case <-s.doneCh:
		s.writeResponse(conn, Response{OK: false, Error: "container already exited"})
		return
	default:
	}

	cwd := args.Cwd
	if cwd == "" {
		cwd = "/"
	}

	// 调用exec-contianer "litcontainer-runc exec-container --cwd <cwd> -- <cmd...>"
	cmdArgs := append([]string{"exec-container", "--cwd", cwd, "--"}, args.Cmd...)
	cmd := exec.Command(s.runcPath, cmdArgs...)
	// 构造LITCONTAINER_EXEC_PID环境变量，触发nsenter
	cmd.Env = append(args.Env, fmt.Sprintf("%s=%d", runtime.ExecPidEnv, s.initPid))

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		s.writeResponse(conn, Response{OK: false, Error: fmt.Sprintf("create stdin pipe: %v", err)})
		stdinPipe.Close()
		return
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		s.writeResponse(conn, Response{OK: false, Error: fmt.Sprintf("create stdout pipe: %v", err)})
		stdoutPipe.Close()
		stdinPipe.Close()
		return
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		s.writeResponse(conn, Response{OK: false, Error: fmt.Sprintf("create stderr pipe: %v", err)})
		stderrPipe.Close()
		stdoutPipe.Close()
		stdinPipe.Close()
		return
	}

	if err := cmd.Start(); err != nil {
		s.writeResponse(conn, Response{OK: false, Error: fmt.Sprintf("start exec command: %v", err)})
		return
	}

	// exec成功启动，准备转换换输入输出流
	if err := s.writeResponse(conn, Response{OK: true}); err != nil {
		logger.Error("write exec response: %v", err)
		cmd.Process.Kill()
		_ = cmd.Wait()
		return
	}

	// 处理stdin,不需要设置frame
	go func() {
		// reader返回EOF或stdinPipe被关闭时，io.Copy会退出
		io.Copy(stdinPipe, reader)
		stdinPipe.Close()
	}()

	// stdout、stderr设置frame然后直接写到conn中
	writer := stdcopy.NewWriter(conn)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		// EOF或stdoutPipe被关闭时，io.Copy会退出
		io.Copy(writer.Stdout(), stdoutPipe)
	}()
	go func() {
		defer wg.Done()
		// EOF或stderrPipe被关闭时，io.Copy会退出
		io.Copy(writer.Stderr(), stderrPipe)
	}()
	wg.Wait()

	code := 0
	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			logger.Warn("exec wait: %v", err)
			code = -1
		}
	}

	// 写入exit code，通知shim对端exec命令已经结束了
	if err := writer.WriteExit(code); err != nil {
		logger.Warn("exec: write exit frame: %v", err)
	}
	logger.Info("exec finished, code=%d", code)
}
