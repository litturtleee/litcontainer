package handlers

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"litcontainer/internal/api/errdefs"
	"litcontainer/internal/api/types"
	"litcontainer/internal/daemon"
	"net/http"
	"strconv"
	"syscall"
	"time"
)

type ContainerHandler struct {
	daemon *daemon.Daemon
}

func NewContainerHandler(daemon *daemon.Daemon) *ContainerHandler {
	return &ContainerHandler{
		daemon: daemon,
	}
}

// Create 创建容器
func (h *ContainerHandler) CreateContainer(c *gin.Context) {
	var req types.ContainerCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, types.Error(errdefs.ErrInvalidParameter, err.Error(), err.Error()))
		return
	}
	id, err := h.daemon.ContainerCreate(&daemon.CreateOptions{
		Name:         req.Name,
		Image:        req.Image,
		Command:      req.Command,
		Env:          req.Env,
		Mounts:       req.Mounts,
		CPULimit:     req.CPULimit,
		MemoryLimit:  req.MemoryLimit,
		Network:      req.Network,
		PortMappings: req.PortMappings,
		EnableTTY:    req.TTY,
	})
	if err != nil {
		responseError(c, err)
		return
	}
	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, types.CreateContainerResp{ID: id}, nil))
}

func (h *ContainerHandler) StartContainer(c *gin.Context) {
	id, ok := h.checkParamId(c)
	if !ok {
		return
	}
	err := h.daemon.ContainerStart(id)
	if err != nil {
		responseError(c, err)
		return
	}
	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, types.StartContainerResp{ID: id}, nil))
}

func (h *ContainerHandler) StopContainer(c *gin.Context) {
	id, ok := h.checkParamId(c)
	if !ok {
		return
	}
	timeoutStr := c.DefaultQuery("timeout", "10")
	timeout, err := strconv.Atoi(timeoutStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, types.Error(errdefs.ErrInvalidParameter, err.Error(), err.Error()))
		return
	}

	err = h.daemon.ContainerStop(id, time.Duration(timeout)*time.Second)
	if err != nil {
		responseError(c, err)
		return
	}

	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, types.StopContainerResp{ID: id}, nil))
}

func (h *ContainerHandler) KillContainer(c *gin.Context) {
	id, ok := h.checkParamId(c)
	if !ok {
		return

	}

	signalStr := c.DefaultQuery("signal", "SIGTERM")
	signal, err := h.parseSignal(signalStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, types.Error(errdefs.ErrInvalidParameter, err.Error(), err.Error()))
		return
	}

	err = h.daemon.ContainerKill(id, signal)
	if err != nil {
		responseError(c, err)
		return
	}

	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, types.KillContainerResp{ID: id}, nil))
}

func (h *ContainerHandler) WaitContainer(c *gin.Context) {
	id, ok := h.checkParamId(c)
	if !ok {
		return
	}

	err := h.daemon.ContainerWait(id)
	if err != nil {
		responseError(c, err)
		return
	}
	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, types.WaitContainerResp{ID: id}, nil))
}

func (h *ContainerHandler) RemoveContainer(c *gin.Context) {
	id, ok := h.checkParamId(c)
	if !ok {
		return
	}
	forceStr := c.DefaultQuery("force", "false")
	force, err := strconv.ParseBool(forceStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, types.Error(errdefs.ErrInvalidParameter, err.Error(), err.Error()))
		return
	}
	err = h.daemon.ContainerRemove(id, force)
	if err != nil {
		responseError(c, err)
		return
	}
	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, nil, nil))
}

func (h *ContainerHandler) ListContainers(c *gin.Context) {
	list := h.daemon.ContainerList()
	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, list, nil))
}

func (h *ContainerHandler) InspectContainer(c *gin.Context) {
	id, ok := h.checkParamId(c)
	if !ok {
		return
	}
	containerCfg, err := h.daemon.ContainerInspect(id)
	if err != nil {
		responseError(c, err)
		return
	}
	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, containerCfg, nil))
}

func (h *ContainerHandler) LogsContainer(c *gin.Context) {
	id, ok := h.checkParamId(c)
	if !ok {
		return
	}
	logs, err := h.daemon.ContainerLogs(id)
	if err != nil {
		responseError(c, err)
		return
	}
	// JSON marshal的时候会把[]byte变成base64字符串, 所以用c.Data()
	c.Data(http.StatusOK, "text/plain; charset=utf-8", logs)
}

// --- 内部方法 ---
func (h *ContainerHandler) checkParamId(c *gin.Context) (string, bool) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest,
			types.Error(errdefs.ErrInvalidContainerID, "invalid container id", "container id cannot be empty"))
		return "", false
	}
	return id, true
}

func (h *ContainerHandler) parseSignal(signal string) (syscall.Signal, error) {
	switch signal {
	case "SIGTERM":
		return syscall.SIGTERM, nil
	case "SIGKILL":
		return syscall.SIGKILL, nil
	case "SIGHUP":
		return syscall.SIGHUP, nil
	case "SIGINT":
		return syscall.SIGINT, nil
	default:
		return 0, fmt.Errorf("invalid signal: %s", signal)
	}
}
