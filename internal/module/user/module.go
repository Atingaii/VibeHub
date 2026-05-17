// Package user 用户模块：注册 / 登录 / 资料 / 标签 / 关注。
//
// 1.1：POST /api/v1/auth/register（密码注册）
// 1.2：POST /api/v1/auth/{login,refresh,logout}
// 1.3+：JWT 中间件 / profile / tags / follow
package user

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
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

// Config 给 NewModule 用的最小配置；从 cfg.Auth 拷贝过来，避免直接依赖整个 Config。
type Config struct {
	JWTSecret       []byte
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

// NewModule 构造用户模块。
//
// db 必须是 MySQL（按 ADR-002）；redisSession 必须是 PoolSession 对应的 client（DB3，按 ADR-003）。
// 1.1 单元测试可以传 nil 给 redisSession 与 cfg（注册流程不依赖），但运行时必须齐备。
func NewModule(db *gorm.DB, cfg Config, redisSession redis.UniversalClient) (*Module, error) {
	userStore := store.NewUserStore(db)

	signer, err := NewJWTSigner(cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	if err != nil {
		return nil, err
	}
	refresh := NewRedisRefreshStore(redisSession)

	svc := newService(userStore, signer, refresh)
	h := newHandler(svc)
	return &Module{handler: h, service: svc}, nil
}

// RegisterRoutes 在给定路由组下注册用户模块的 HTTP 端点。
// 调用方应传入 /api/v1 这一级。
func (m *Module) RegisterRoutes(rg *gin.RouterGroup) {
	auth := rg.Group("/auth")
	auth.POST("/register", m.handler.Register)
	auth.POST("/login", m.handler.Login)
	auth.POST("/refresh", m.handler.Refresh)
	auth.POST("/logout", m.handler.Logout)
}
