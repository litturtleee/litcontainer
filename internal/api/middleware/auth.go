package middleware

import (
	"github.com/gin-gonic/gin"
	"litcontainer/internal/api/errdefs"
	"litcontainer/internal/api/types"
	"litcontainer/internal/model"
	"litcontainer/internal/service"
	"litcontainer/pkg/logger"
	"net/http"
	"strings"
)

func AuthMiddleware(authService *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取认证信息
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			logger.Error("Missing authentication information")
			c.JSON(http.StatusUnauthorized,
				types.Error(errdefs.ErrUnauthorized, "Missing authentication information", ""))
			c.Abort()
			return
		}

		var authContext *model.AuthContext
		var err error

		// 解析认证头
		if !strings.HasPrefix(authHeader, "Bearer ") {
			logger.Error("Unsupported authentication methods")
			c.JSON(http.StatusUnauthorized,
				types.Error(errdefs.ErrUnauthorized, "Unsupported authentication methods", ""))
			c.Abort()
			return

		}
		// JWT Token 认证
		token := strings.TrimPrefix(authHeader, "Bearer ")
		authContext, err = authService.ValidateToken(token)
		if err != nil {
			logger.Error("authentication failure: %v", err)
			c.JSON(http.StatusUnauthorized, types.Error(errdefs.ErrAuthFailed, "Authentication failure", err.Error()))
			c.Abort()
			return
		}

		// 将认证上下文存储到请求中
		c.Set("auth_context", authContext)
		c.Next()
	}
}

func PermissionMiddleware(authService *service.AuthService, resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authContext, exists := c.Get("auth_context")
		logger.Debug("auth_context: %v", authContext)
		if !exists {
			logger.Error("Missing authentication information")
			c.JSON(http.StatusUnauthorized,
				types.Error(errdefs.ErrUnauthorized, "Missing authentication information", ""))
			c.Abort()
			return
		}

		ctx, ok := authContext.(*model.AuthContext)
		if !ok {
			logger.Error("Invalid authentication context")
			c.JSON(http.StatusUnauthorized, types.Error(errdefs.ErrUnauthorized, "Invalid authentication context", ""))
			c.Abort()
			return
		}

		// 检查权限
		if !authService.CheckPermission(ctx, resource, action) {
			logger.Error("Insufficient permissions: resource=%s, action=%s", resource, action)
			c.JSON(http.StatusForbidden,
				types.Error(errdefs.ErrAccessDenied, "Insufficient permissions",
					ctx.Username+" no right of access "+resource+" "+action))
			c.Abort()
			return
		}

		c.Next()
	}
}
