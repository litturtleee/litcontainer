package handlers

import "C"
import (
	"github.com/gin-gonic/gin"
	"litcontainer/internal/api/errdefs"
	"litcontainer/internal/api/types"
	"litcontainer/internal/container"
	"net/http"
)

func ListContainers(c *gin.Context) {
	allConfig, err := container.GetAllConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.Error(errdefs.ErrInternalServerError, err.Error(), err.Error()))
	}
	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, allConfig, nil))
}

func GetContainer(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest,
			types.Error(errdefs.ErrInvalidContainerID, "invalid container id", "container id cannot be empty"))
	}
	containerConfig, err := container.GetContainerConfigById(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, types.Error(errdefs.ErrInternalServerError, err.Error(), err.Error()))
		return
	}
	if containerConfig == nil {
		c.JSON(http.StatusNotFound,
			types.Error(errdefs.ErrContainerNotFound, "container not found", "container not found"))
		return
	}
	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, containerConfig, nil))
}
