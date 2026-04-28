package client

import (
	"litcontainer/internal/api/types"
	"litcontainer/internal/network"
)

func (c *Client) NetworkCreate(req types.NetworkCreateRequest) error {
	path := "/api/v1/networks/create"
	return c.do("POST", path, req, nil)
}

func (c *Client) NetworkRemove(name string) error {
	path := "/api/v1/networks/" + name
	return c.do("DELETE", path, nil, nil)
}

func (c *Client) NetworkList() ([]*network.Network, error) {
	path := "/api/v1/networks/list"
	var res []*network.Network
	return res, c.do("GET", path, nil, &res)
}

func (c *Client) NetworkInspect(name string) (network.Network, error) {
	path := "/api/v1/networks/" + name
	var res network.Network
	return res, c.do("GET", path, nil, &res)
}
