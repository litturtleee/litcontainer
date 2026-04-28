package handlers

import (
	"github.com/gin-gonic/gin"
	"litcontainer/internal/api/errdefs"
	"litcontainer/internal/api/types"
	"litcontainer/internal/daemon"
	"net/http"
)

type ImageHandler struct {
	daemon *daemon.Daemon
}

func NewImageHandler(d *daemon.Daemon) *ImageHandler {
	return &ImageHandler{
		daemon: d,
	}
}

func (h *ImageHandler) ExportImage(c *gin.Context) {
	var req types.ImageExportRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, types.Error(errdefs.ErrInvalidParameter, err.Error(), err.Error()))
		return
	}
	err = h.daemon.ExportImage(req.ContainerName, req.OutputName)
	if err != nil {
		responseError(c, err)
		return
	}
	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, nil, nil))
}
