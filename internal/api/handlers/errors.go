package handlers

import (
	"errors"
	"github.com/gin-gonic/gin"
	"litcontainer/internal/api/errdefs"
	"litcontainer/internal/api/types"
	"litcontainer/internal/container"
	"net/http"
)

// responseError 响应错误映射
func responseError(c *gin.Context, err error) {
	var status int
	var code string

	switch {
	case errors.Is(err, container.ErrContainerNotFound):
		status, code = http.StatusNotFound, errdefs.ErrContainerNotFound
	case errors.Is(err, container.ErrContainerIsRunning):
		status, code = http.StatusConflict, errdefs.ErrContainerIsRunning
	case errors.Is(err, container.ErrContainerNotRunning):
		status, code = http.StatusConflict, errdefs.ErrContainerNotRunning
	case errors.Is(err, container.ErrInitInvalidArgs):
		status, code = http.StatusBadRequest, errdefs.ErrInitInvalidArgs
	default:
		status, code = http.StatusInternalServerError, errdefs.ErrInternalServerError
	}

	c.JSON(status, types.Error(code, err.Error(), err.Error()))
}
