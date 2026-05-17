// Package user 用户模块：注册 / 登录 / 资料 / 标签 / 关注。
//
// 阶段 1.1 仅暴露 POST /api/v1/auth/register（密码注册）。
// 1.2 之后逐步加 login / JWT 中间件 / profile / tags / follow。
package user

import (
	"github.com/gin-gonic/gin"
	"github.com/vibeshop/vibeshop/internal/store"
	"gorm.io/gorm"
)

// Module 是用户模块的统一入口；持有内部 service / handler 引用，
// 对外仅暴露 RegisterRoutes。其他模块需要用户能力时通过 Module 上的方法获取，
// 不直接 import service / store / model。
type Module struct {
	handler *handler
	service *service
}

// NewModule 构造用户模块。db 必须是 MySQL（按 ADR-002）。
func NewModule(db *gorm.DB) *Module {
	userStore := store.NewUserStore(db)
	svc := newService(userStore)
	h := newHandler(svc)
	return &Module{handler: h, service: svc}
}

// RegisterRoutes 在给定路由组下注册用户模块的 HTTP 端点。
// 调用方应传入 /api/v1 这一级。
func (m *Module) RegisterRoutes(rg *gin.RouterGroup) {
	auth := rg.Group("/auth")
	auth.POST("/register", m.handler.Register)
}
