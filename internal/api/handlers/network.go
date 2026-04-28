package handlers

import (
	"github.com/gin-gonic/gin"
	"litcontainer/internal/api/errdefs"
	"litcontainer/internal/api/types"
	"litcontainer/internal/daemon"
	"net/http"
)

type NetworkHandler struct {
	daemon *daemon.Daemon
}

func NewNetworkHandler(d *daemon.Daemon) *NetworkHandler {
	return &NetworkHandler{
		daemon: d,
	}
}

func (h *NetworkHandler) NetworkCreate(c *gin.Context) {
	var req types.NetworkCreateRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, types.Error(errdefs.ErrInvalidParameter, err.Error(), err.Error()))
		return
	}

	err = h.daemon.NetworkCreate(req.Name, req.Driver, req.Subnet)
	if err != nil {
		responseError(c, err)
		return
	}

	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, nil, nil))
}

func (h *NetworkHandler) NetworkRemove(c *gin.Context) {
	name, ok := h.checkParamName(c)
	if !ok {
		return
	}
	err := h.daemon.NetworkRemove(name)
	if err != nil {
		responseError(c, err)
		return
	}
	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, nil, nil))
}

func (h *NetworkHandler) NetworkList(c *gin.Context) {
	networks, err := h.daemon.NetworkList()
	if err != nil {
		responseError(c, err)
		return
	}
	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, networks, nil))
}

func (h *NetworkHandler) NetworkInspect(c *gin.Context) {
	name, ok := h.checkParamName(c)
	if !ok {
		return
	}
	nw, err := h.daemon.NetworkInspect(name)
	if err != nil {
		responseError(c, err)
		return
	}
	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, nw, nil))
}

// --- 内部方法 ---

func (h *NetworkHandler) checkParamName(c *gin.Context) (string, bool) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest,
			types.Error(errdefs.ErrInvalidParameter, "invalid parameter", "name cannot be empty"))
		return "", false
	}
	return name, true
}
