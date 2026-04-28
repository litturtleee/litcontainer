package client

import (
	"fmt"
	"io"
	"litcontainer/internal/api/types"
	"litcontainer/internal/container"
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

func (c *Client) ListContainers() ([]*container.Config, error) {
	var containers []*container.Config
	err := c.do("GET", "/api/v1/containers/list", nil, &containers)
	return containers, err
}

func (c *Client) InspectContainer(idOrName string) (*container.Config, error) {
	var containerCfg *container.Config
	err := c.do("GET", "/api/v1/containers/"+idOrName, nil, &containerCfg)
	return containerCfg, err
}

func (c *Client) LogsContainer(idOrName string) ([]byte, error) {
	req, _ := http.NewRequest("GET", "http://x/api/v1/containers/"+idOrName+"/logs", nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, ErrDaemonUnreachable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("logs failed, status: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
