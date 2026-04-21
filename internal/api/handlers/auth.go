package handlers

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

type AuthHandler struct {
	service *service.AuthService
}

func NewAuthHandler(service *service.AuthService) *AuthHandler {
	return &AuthHandler{
		service: service,
	}
}

// Login 用户登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error("Login request parameter binding failed: %v", err)
		c.JSON(http.StatusBadRequest,
			types.Error(errdefs.ErrInvalidParameter, "Invalid request parameters", err.Error()))
		return
	}

	response, err := h.service.Login(&req)
	if err != nil {
		logger.Error("User Login Failure: %v", err)
		c.JSON(http.StatusUnauthorized, types.Error(errdefs.ErrLoginFailed, "Login Failure", err.Error()))
		return
	}

	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, response, nil))
}

// Register 用户注册
func (h *AuthHandler) Register(c *gin.Context) {
	var req model.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error("Registration request parameter binding failed: %v", err)
		c.JSON(http.StatusBadRequest,
			types.Error(errdefs.ErrInvalidParameter, "Invalid request parameters", err.Error()))
		return
	}

	user, err := h.service.Register(&req)
	if err != nil {
		logger.Error("User registration failure: %v", err)
		c.JSON(http.StatusInternalServerError,
			types.Error(errdefs.ErrRegisterFailed, "Registration Failure", err.Error()))
		return
	}

	c.JSON(http.StatusCreated, types.Success(types.ApiVersionV1, user, nil))
}

// GetProfile 获取用户信息
func (h *AuthHandler) GetProfile(c *gin.Context) {
	authContext, exists := c.Get("auth_context")
	if !exists {
		logger.Error("Missing authentication information")
		c.JSON(http.StatusUnauthorized, types.Error(errdefs.ErrUnauthorized, "Missing authentication information", ""))
		return
	}

	ctx, ok := authContext.(*model.AuthContext)
	if !ok {
		logger.Error("The authentication context is invalid")
		c.JSON(http.StatusUnauthorized,
			types.Error(errdefs.ErrUnauthorized, "The authentication context is invalid", ""))
		return
	}

	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, ctx, nil))
}

// Logout 用户退出
func (h *AuthHandler) Logout(c *gin.Context) {
	// 从 Authorization 头中提取 Bearer Token
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		logger.Error("Missing or invalid authentication information")
		c.JSON(http.StatusUnauthorized,
			types.Error(errdefs.ErrUnauthorized, "Missing or invalid authentication information", ""))
		return
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")

	err := h.service.Logout(token)
	if err != nil {
		logger.Error("User logout failed: %v", err)
		c.JSON(http.StatusInternalServerError, types.Error(errdefs.ErrLogoutFailed, "User logout failed", err.Error()))
		return
	}

	c.JSON(http.StatusOK, types.Success(types.ApiVersionV1, nil, nil))

}
