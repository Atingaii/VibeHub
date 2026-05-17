package user

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// loginService 是 handler 对 service 的最小依赖契约（便于 handler 单测注入 fake）。
type loginService interface {
	Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error)
	Login(ctx context.Context, req LoginRequest) (*LoginResponse, error)
	Refresh(ctx context.Context, req RefreshRequest) (*LoginResponse, error)
	Logout(ctx context.Context, req RefreshRequest) error
}

// handler 把 HTTP 与 service 解耦。
type handler struct {
	svc loginService
}

func newHandler(svc loginService) *handler {
	return &handler{svc: svc}
}

// Register 处理 POST /api/v1/auth/register。
func (h *handler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorBody{
			Code:    "INVALID_REQUEST",
			Message: "request body is not valid JSON",
		})
		return
	}

	resp, err := h.svc.Register(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidIdentifier):
			c.JSON(http.StatusBadRequest, errorBody{
				Code:    "INVALID_REQUEST",
				Message: "exactly one of username/phone/email is required, and must match its format",
			})
		case errors.Is(err, ErrInvalidPassword):
			c.JSON(http.StatusBadRequest, errorBody{
				Code:    "INVALID_REQUEST",
				Message: "password length must be 8-72",
			})
		case errors.Is(err, ErrIdentifierTaken):
			c.JSON(http.StatusConflict, errorBody{
				Code:    "IDENTIFIER_TAKEN",
				Message: "the identifier is already in use",
			})
		default:
			zap.L().Error("[user] register internal error", zap.Error(err))
			c.JSON(http.StatusInternalServerError, errorBody{
				Code:    "INTERNAL",
				Message: "internal server error",
			})
		}
		return
	}

	c.JSON(http.StatusCreated, resp)
}

// Login 处理 POST /api/v1/auth/login。
func (h *handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorBody{
			Code:    "INVALID_REQUEST",
			Message: "request body is not valid JSON",
		})
		return
	}
	resp, err := h.svc.Login(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidIdentifier):
			c.JSON(http.StatusBadRequest, errorBody{
				Code:    "INVALID_REQUEST",
				Message: "identifier is required and must be a valid username/phone/email",
			})
		case errors.Is(err, ErrInvalidPassword):
			c.JSON(http.StatusBadRequest, errorBody{
				Code:    "INVALID_REQUEST",
				Message: "password length must be 8-72",
			})
		case errors.Is(err, ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, errorBody{
				Code:    "INVALID_CREDENTIALS",
				Message: "incorrect identifier or password",
			})
		default:
			zap.L().Error("[user] login internal error", zap.Error(err))
			c.JSON(http.StatusInternalServerError, errorBody{
				Code:    "INTERNAL",
				Message: "internal server error",
			})
		}
		return
	}
	c.JSON(http.StatusOK, resp)
}

// Refresh 处理 POST /api/v1/auth/refresh。
func (h *handler) Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorBody{
			Code:    "INVALID_REQUEST",
			Message: "request body is not valid JSON",
		})
		return
	}
	resp, err := h.svc.Refresh(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidIdentifier):
			c.JSON(http.StatusBadRequest, errorBody{
				Code:    "INVALID_REQUEST",
				Message: "refresh_token is required",
			})
		case errors.Is(err, ErrInvalidToken):
			c.JSON(http.StatusUnauthorized, errorBody{
				Code:    "INVALID_TOKEN",
				Message: "refresh token is invalid or expired",
			})
		default:
			zap.L().Error("[user] refresh internal error", zap.Error(err))
			c.JSON(http.StatusInternalServerError, errorBody{
				Code:    "INTERNAL",
				Message: "internal server error",
			})
		}
		return
	}
	c.JSON(http.StatusOK, resp)
}

// Logout 处理 POST /api/v1/auth/logout。
func (h *handler) Logout(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorBody{
			Code:    "INVALID_REQUEST",
			Message: "request body is not valid JSON",
		})
		return
	}
	err := h.svc.Logout(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidIdentifier):
			c.JSON(http.StatusBadRequest, errorBody{
				Code:    "INVALID_REQUEST",
				Message: "refresh_token is required",
			})
		case errors.Is(err, ErrInvalidToken):
			c.JSON(http.StatusUnauthorized, errorBody{
				Code:    "INVALID_TOKEN",
				Message: "refresh token is invalid or expired",
			})
		default:
			zap.L().Error("[user] logout internal error", zap.Error(err))
			c.JSON(http.StatusInternalServerError, errorBody{
				Code:    "INTERNAL",
				Message: "internal server error",
			})
		}
		return
	}
	c.Status(http.StatusNoContent)
}
