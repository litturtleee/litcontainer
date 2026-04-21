package handlers

import (
	"github.com/gin-gonic/gin"
	"litcontainer/internal/api/types"
	"net/http"
)

func ListImages(c *gin.Context) {
	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, "images", nil))
}
