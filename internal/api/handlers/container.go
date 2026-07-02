package handlers

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"litcontainer/internal/api/errdefs"
	"litcontainer/internal/api/types"
	"litcontainer/internal/daemon"
	"litcontainer/internal/logger"
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

// CreateContainer 创建容器
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
	containerInfo, err := h.daemon.ContainerInspect(id)
	if err != nil {
		responseError(c, err)
		return
	}
	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, containerInfo, nil))
}

func (h *ContainerHandler) LogsContainer(c *gin.Context) {
	id, ok := h.checkParamId(c)
	if !ok {
		return
	}
	follow := c.DefaultQuery("follow", "false") == "true"

	// 设置为chunck HTTP 流
	c.Header("Content-Type", "application/vnd.lit.raw-stream")

	fw := &flushingWriter{w: c.Writer}

	err := h.daemon.ContainerLogsStream(c.Request.Context(), id, follow, fw)
	if err != nil {
		if c.Writer.Written() {
			logger.Warn("logs stream mid-error: %v", err)
			return
		}
		responseError(c, err)
	}
}

func (h *ContainerHandler) ExecContainer(c *gin.Context) {
	id, ok := h.checkParamId(c)
	if !ok {
		return
	}

	var req types.ExecRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, types.Error(errdefs.ErrInvalidParameter, err.Error(), err.Error()))
		return
	}

	// 将http连接升级为hijack连接
	hijacker, ok := c.Writer.(http.Hijacker)
	if !ok {
		c.JSON(http.StatusInternalServerError,
			types.Error(errdefs.ErrInternalServerError, "hijack not supported",
				"the server does not support hijacking"))
		return
	}
	conn, bufioRw, err := hijacker.Hijack()
	if err != nil {
		c.JSON(http.StatusInternalServerError,
			types.Error(errdefs.ErrInternalServerError, "hijack failed",
				fmt.Sprintf("failed to hijack connection: %v", err)))
		return
	}
	defer conn.Close()

	// hijack后, gin不再响应，手动通知client握手成功，发送200响应行和Content-Type头，告知client后续是exec-stream
	if _, err := conn.Write([]byte("HTTP/1.1 200 OK\r\n" +
		"Content-Type: application/vnd.lit.exec-stream\r\n\r\n")); err != nil {
		logger.Warn("exec: write 200 line: %v", err)
		return
	}

	if err := h.daemon.ContainerExec(id, req, conn, bufioRw.Reader); err != nil {
		errJSON := fmt.Sprintf(`{"ok":false,"error":%q}`+"\n", err.Error())
		_, _ = conn.Write([]byte(errJSON))
		logger.Warn("exec: %v", err)
	}
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

// flushingWriter 包装 gin.ResponseWriter，每次 Write 后立即 Flush
// 用途：保证 chunked HTTP 流的数据立刻送达 client（而不是等内部 buffer 满）
type flushingWriter struct {
	w gin.ResponseWriter
}

func (fw *flushingWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	fw.w.Flush()
	return n, err
}
