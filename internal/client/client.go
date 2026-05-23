package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

const SocketPath = "/var/run/litcontainer.sock"

type Client struct {
	httpClient *http.Client
}

type Result struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", SocketPath)
				},
			},
		},
	}
}

// do 发送请求
func (c *Client) do(method, path string, body any, out any) error {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}
	req, _ := http.NewRequest(method, "http://x"+path, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ErrDaemonUnreachable
	}
	defer resp.Body.Close()

	var result Result
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return err
	}
	if !result.Success {
		if result.Error == nil {
			return fmt.Errorf("daemon returned failure without error")
		}
		return fmt.Errorf("[%s] %s", result.Error.Code, result.Error.Message)
	}
	// 需要输出
	if out != nil && len(result.Data) > 0 {
		return json.Unmarshal(result.Data, out)
	}
	return nil
}

// openStream 建到 daemon 的 unix socket
// http.do方法不适合处理 exec/logs 这种长连接流式接口，所以这里手动实现握手，
// 返回底层的 net.Conn 和 bufio.Reader 供上层使用
func (c *Client) openStream(method, path string, body []byte) (net.Conn, *bufio.Reader, error) {
	conn, err := net.Dial("unix", SocketPath)
	if err != nil {
		return nil, nil, ErrDaemonUnreachable
	}

	// 发送请求
	httpReq := fmt.Sprintf(
		"%s %s HTTP/1.1\r\n"+
			"Host: x\r\n"+
			"Content-Type: application/json\r\n"+
			"Content-Length: %d\r\n"+
			"\r\n",
		method, path, len(body))
	if _, err := conn.Write([]byte(httpReq)); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("write request line: %w", err)
	}
	if len(body) > 0 {
		if _, err := conn.Write(body); err != nil {
			conn.Close()
			return nil, nil, fmt.Errorf("write body: %w", err)
		}
	}

	// 处理handshake
	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("read status: %w", err)
	}
	parts := strings.SplitN(strings.TrimSpace(statusLine), " ", 3)
	if len(parts) < 2 {
		conn.Close()
		return nil, nil, fmt.Errorf("invalid status line: %q", statusLine)
	}
	if parts[1] != "200" {
		body, _ := io.ReadAll(br)
		conn.Close()
		return nil, nil, fmt.Errorf("request failed (HTTP %s): %s", parts[1], strings.TrimSpace(string(body)))
	}

	// 跳过headers,bufio中就只剩下body了
	for {
		h, err := br.ReadString('\n')
		if err != nil {
			conn.Close()
			return nil, nil, fmt.Errorf("read header: %w", err)
		}
		if h == "\r\n" || h == "\n" {
			break
		}
	}
	// 返回连接和bufio.Reader供上层使用
	return conn, br, nil
}
