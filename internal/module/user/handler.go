package user

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// registerService 是 handler 对 service 的最小依赖契约（便于 handler 单测注入 fake）。
type registerService interface {
	Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error)
}

// handler 把 HTTP 与 service 解耦。
type handler struct {
	svc registerService
}

func newHandler(svc registerService) *handler {
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
