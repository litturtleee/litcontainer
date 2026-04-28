package types

import "time"

const ApiVersionV1 = "v1"

// APIResponse 统一的API响应格式
type APIResponse struct {
	Success    bool        `json:"success"`
	Timestamp  int64       `json:"timestamp"`
	ApiVersion string      `json:"apiVersion"`
	Data       interface{} `json:"data"`
	Pagination *Pagination `json:"pagination,omitempty"`
	Error      *APIError   `json:"error,omitempty"`
}

type Pagination struct {
	Total int64 `json:"total"`
	Page  int64 `json:"page"`
	Limit int64 `json:"limit"`
}
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

type CreateContainerResp struct {
	ID string `json:"id"`
}

type StartContainerResp struct {
	ID string `json:"id"`
}

type StopContainerResp struct {
	ID string `json:"id"`
}

type KillContainerResp struct {
	ID string `json:"id"`
}

type WaitContainerResp struct {
	ID string `json:"id"`
}

func Success(apiVersion string, data interface{}, pagination *Pagination) *APIResponse {
	return &APIResponse{
		Success:    true,
		Timestamp:  time.Now().UnixMilli(),
		ApiVersion: apiVersion,
		Data:       data,
		Pagination: pagination,
	}
}

func Error(code, message string, details string) *APIResponse {
	return &APIResponse{
		Success:   false,
		Timestamp: time.Now().UnixMilli(),
		Error: &APIError{
			Code:    code,
			Message: message,
			Details: details,
		},
	}
}
