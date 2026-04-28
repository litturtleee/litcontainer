package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
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
