package shim

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"time"
)

const (
	DefaultShimRoot = "/run/litcontainer/shim"
	// 建联超时时间
	ConnectTimeout = 3 * time.Second
	Unix           = "unix"
)

type Session struct {
	Conn         net.Conn
	Reader       *bufio.Reader // handshake后从这里读后续stream
	HandshakeRaw []byte        // shim 握手响应
	HandshakeRsp Response      // shim 握手响应
}

type Client struct {
	SocketPath string
}

func NewShimClient(id string) *Client {
	return &Client{
		SocketPath: SocketPath(id),
	}
}

func SocketPath(id string) string {
	return filepath.Join(DefaultShimRoot, id, "shim.sock")
}

func (c *Client) State() (*StateData, error) {
	rsp, err := c.do(&Request{Cmd: CmdState}, ConnectTimeout)
	if err != nil {
		return nil, err
	}
	if !rsp.OK {
		return nil, fmt.Errorf("shim state: %s", rsp.Error)
	}
	var data StateData
	if err := json.Unmarshal(rsp.Data, &data); err != nil {
		return nil, fmt.Errorf("unmarshal state data: %w", err)
	}
	return &data, nil
}

func (c *Client) Stop(signal int, timeout int) error {
	args, _ := json.Marshal(StopArgs{Signal: signal, Timeout: timeout})
	rsp, err := c.do(&Request{Cmd: CmdStop, Args: args}, ConnectTimeout)
	if err != nil {
		return err
	}
	if !rsp.OK {
		return fmt.Errorf("shim stop: %s", rsp.Error)
	}
	return nil
}

func (c *Client) Wait() (*StateData, error) {
	rsp, err := c.do(&Request{Cmd: CmdWait}, ConnectTimeout)
	if err != nil {
		return nil, err
	}
	if !rsp.OK {
		return nil, fmt.Errorf("shim wait: %s", rsp.Error)
	}
	var data StateData
	if err := json.Unmarshal(rsp.Data, &data); err != nil {
		return nil, fmt.Errorf("unmarshal wait data: %w", err)
	}
	return &data, nil
}

func (c *Client) Kill(signal int) error {
	args, _ := json.Marshal(KillArgs{Signal: signal})
	rsp, err := c.do(&Request{Cmd: CmdKill, Args: args}, ConnectTimeout)
	if err != nil {
		return err
	}
	if !rsp.OK {
		return fmt.Errorf("shim kill: %s", rsp.Error)
	}
	return nil
}

func (c *Client) Delete() error {
	rsp, err := c.do(&Request{Cmd: CmdDelete}, ConnectTimeout)
	if err != nil {
		return err
	}
	if !rsp.OK {
		return fmt.Errorf("shim delete: %s", rsp.Error)
	}
	return nil
}

// Exec 在容器内执行新进程
// 参数：
// - args: exec命令参数
// - stdin: hijacked连接的reader(client与daemon之间的reader)
// - stream: exec进程的标准输出和标准错误会写到这个stream里(client与daemon之间的writer)
func (c *Client) Exec(args ExecArgs, stdin io.Reader, stream io.Writer) error {
	// session是Daemon和shim之间的长连接
	session, err := c.doSession(CmdExec, args)
	if err != nil {
		return fmt.Errorf("exec session: %w", err)
	}
	defer session.Conn.Close()

	// 握手完成，转发握手响应
	if _, err := stream.Write(session.HandshakeRaw); err != nil {
		return fmt.Errorf("write handshake response: %w", err)
	}
	if !session.HandshakeRsp.OK {
		return nil
	}

	// 启动转发
	go func() {
		io.Copy(session.Conn, stdin)
		if uc, ok := session.Conn.(*net.UnixConn); ok {
			uc.CloseWrite() // 关闭写端，通知shim不再读取stdin数据
		}
	}()

	// shim写完exit后关闭conn
	_, err = io.Copy(stream, session.Reader)
	return err
}

// --- 内部方法 ---

// do 短连接
func (c *Client) do(req *Request, dialTimeout time.Duration) (*Response, error) {
	conn, err := net.DialTimeout(Unix, c.SocketPath, dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("dial shim socket: %w", err)
	}
	defer conn.Close()

	// send
	bytes, _ := json.Marshal(req)
	bytes = append(bytes, '\n')
	if _, err := conn.Write(bytes); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	// recv
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 4096), 1<<20) // 设置最大消息长度为1MB
	// scan方法会一直读取到\n为止
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		return nil, fmt.Errorf("read response: EOF")
	}
	var resp Response
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &resp, nil
}

// doSession 长连接，适用于exec命令
func (c *Client) doSession(cmd string, args interface{}) (*Session, error) {
	conn, err := net.DialTimeout(Unix, c.SocketPath, ConnectTimeout)
	if err != nil {
		return nil, fmt.Errorf("dial shim socket: %w", err)
	}

	var argsBytes []byte
	if args != nil {
		argsBytes, _ = json.Marshal(args)
	}

	req := &Request{Cmd: cmd, Args: argsBytes}
	reqBytes, _ := json.Marshal(req)
	reqBytes = append(reqBytes, '\n')
	if _, err := conn.Write(reqBytes); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write request: %w", err)
	}

	reader := bufio.NewReader(conn)
	handshakerRaw, err := reader.ReadBytes('\n')
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read handshake: %w", err)
	}
	var handshakeRsp Response
	if err := json.Unmarshal(handshakerRaw, &handshakeRsp); err != nil {
		conn.Close()
		return nil, fmt.Errorf("unmarshal handshake: %w", err)
	}

	return &Session{
		Conn:         conn,
		Reader:       reader,
		HandshakeRaw: handshakerRaw,
		HandshakeRsp: handshakeRsp,
	}, nil
}
