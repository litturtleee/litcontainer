package client

import "litcontainer/internal/api/types"

func (c *Client) ExportImage(req types.ImageExportRequest) error {
	return c.do("POST", "/api/v1/images/export", req, nil)
}
