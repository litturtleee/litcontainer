package client

import (
	"encoding/json"
	"fmt"
	"io"
	"litcontainer/internal/api/types"
	"litcontainer/internal/container"
	"litcontainer/internal/stdcopy"
	"net"
	"net/http"
	"strconv"
)

func (c *Client) ContainerCreate(req types.ContainerCreateRequest) (string, error) {
	var resp types.CreateContainerResp
	err := c.do("POST", "/api/v1/containers/create", req, &resp)
	return resp.ID, err
}

func (c *Client) StartContainer(id string) error {
	return c.do("POST", "/api/v1/containers/"+id+"/start", nil, nil)
}

func (c *Client) StopContainer(idOrName string, timeout int) error {
	path := "/api/v1/containers/" + idOrName + "/stop"
	if timeout > 0 {
		path += "?timeout=" + strconv.Itoa(timeout)
	}
	return c.do("POST", path, nil, nil)
}

func (c *Client) KillContainer(id string, signal string) error {
	path := "/api/v1/containers/" + id + "/kill"
	if signal != "" {
		path += "?signal=" + signal
	}
	return c.do("POST", path, nil, nil)
}

func (c *Client) WaitContainer(id string) error {
	path := "/api/v1/containers/" + id + "/wait"
	return c.do("POST", path, nil, nil)
}

func (c *Client) RemoveContainer(idOrName string, force bool) error {
	path := "/api/v1/containers/" + idOrName
	if force {
		path += "?force=true"
	}
	return c.do("DELETE", path, nil, nil)
}

func (c *Client) ListContainers() ([]*container.Info, error) {
	var containers []*container.Info
	err := c.do("GET", "/api/v1/containers/list", nil, &containers)
	return containers, err
}

func (c *Client) InspectContainer(idOrName string) (*container.Info, error) {
	var containerInfo *container.Info
	err := c.do("GET", "/api/v1/containers/"+idOrName, nil, &containerInfo)
	return containerInfo, err
}

func (c *Client) LogsContainerStream(idOrName string, follow bool, stdout, stderr io.Writer) error {
	path := "http://x/api/v1/containers/" + idOrName + "/logs"
	if follow {
		path += "?follow=true"
	}
	// 不用内部的do方法, 因为这里要用raw stream
	req, _ := http.NewRequest("GET", path, nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("logs request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("logs failed (status %d): %s", resp.StatusCode, string(body))
	}
	return stdcopy.Demux(resp.Body, stdout, stderr)
}

func (c *Client) ExecContainer(idOrName string, req types.ExecRequest, stdin io.Reader,
	stdout, stderr io.Writer) (int, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return -1, fmt.Errorf("marshal request: %w", err)
	}

	conn, br, err := c.openStream("POST", "/api/v1/containers/"+idOrName+"/exec", body)
	if err != nil {
		return -1, err
	}
	defer conn.Close()

	// 读握手 JSON
	handshakeLine, err := br.ReadBytes('\n')
	if err != nil {
		return -1, fmt.Errorf("read handshake: %w", err)
	}
	var handshake struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(handshakeLine, &handshake); err != nil {
		return -1, fmt.Errorf("parse handshake: %w", err)
	}
	if !handshake.OK {
		return -1, fmt.Errorf("exec rejected: %s", handshake.Error)
	}

	go func() {
		_, _ = io.Copy(conn, stdin)
		if uc, ok := conn.(*net.UnixConn); ok {
			uc.CloseWrite()
		}
	}()

	return stdcopy.DemuxExec(br, stdout, stderr)
}
