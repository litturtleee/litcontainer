package model

import (
	"encoding/json"
	"time"
)

// User 用户信息
type UserBase struct {
	ID          string            `json:"id"`
	Username    string            `json:"username"`
	Email       string            `json:"email"`
	Roles       []string          `json:"roles"`
	Permissions []string          `json:"permissions"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	LastLoginAt *time.Time        `json:"last_login_at,omitempty"`
	IsActive    bool              `json:"is_active"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type User struct {
	UserBase
	Password string `json:"password"`
}

// Role 角色信息
type Role struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
}

// Permission 权限信息
type Permission struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Resource    string    `json:"resource"`
	Action      string    `json:"action"`
	CreatedAt   time.Time `json:"created_at"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token   string    `json:"token"`
	User    *UserBase `json:"user"`
	Expires int64     `json:"expires"`
}

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username string   `json:"username" binding:"required"`
	Email    string   `json:"email" binding:"required,email"`
	Password string   `json:"password" binding:"required,min=8"`
	Roles    []string `json:"roles,omitempty"`
}

// AuthContext 认证上下文
type AuthContext struct {
	UserID      string   `json:"user_id"`
	Username    string   `json:"username"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

// MarshalJSON 实现自定义的JSON序列化逻辑
func (u *User) MarshalJSON() ([]byte, error) {
	// 定义一个别名类型 Alias 来避免递归调用 MarshalJSON 方法
	// 如果直接使用 User 类型，会导致无限递归，因为 json.Marshal 会再次调用 User 的 MarshalJSON 方法
	type Alias User
	return json.Marshal(&struct {
		*Alias
		CreatedAt   string  `json:"created_at"`
		UpdatedAt   string  `json:"updated_at"`
		LastLoginAt *string `json:"last_login_at,omitempty"`
	}{
		Alias:       (*Alias)(u),
		CreatedAt:   u.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   u.UpdatedAt.Format(time.RFC3339),
		LastLoginAt: formatTime(u.LastLoginAt),
	})
}

func formatTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := t.Format(time.RFC3339)
	return &formatted
}
