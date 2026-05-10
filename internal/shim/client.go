package shim

import (
	"bufio"
	"encoding/json"
	"fmt"
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

// --- 内部方法 ---

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
