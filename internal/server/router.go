package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vibeshop/vibeshop/internal/cache"
	"github.com/vibeshop/vibeshop/internal/config"
	"github.com/vibeshop/vibeshop/internal/database"
	"github.com/vibeshop/vibeshop/internal/middleware"
	"github.com/vibeshop/vibeshop/internal/module/user"
)

// Version 和 BuildTime 由编译时 ldflags 注入，默认 "dev"
var (
	Version   = "dev"
	BuildTime = "unknown"
)

// SetupRouter 创建并配置 Gin 路由
func SetupRouter(cfg *config.Config, db *database.Manager, rds *cache.RedisManager) *gin.Engine {
	// 非 debug 模式使用 release mode
	if !cfg.App.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	// 使用 gin.New() 替代 gin.Default()，注册自定义中间件
	r := gin.New()

	// 注册中间件：panic 恢复 + 请求日志
	r.Use(middleware.Recovery())
	r.Use(middleware.RequestLogger())

	// ====== 基础端点 ======

	// 健康检查（Docker healthcheck / K8s probe / 开发验证）
	r.GET("/health", func(c *gin.Context) {
		status := "ok"

		// 检查 MySQL 连接
		mysqlOK := true
		if err := db.PingMySQL(); err != nil {
			mysqlOK = false
			status = "degraded"
		}

		// 检查 PostgreSQL 连接
		postgresOK := true
		if err := db.PingPostgres(); err != nil {
			postgresOK = false
			status = "degraded"
		}

		// 检查 Redis 连接
		redisOK := true
		if err := rds.Ping(); err != nil {
			redisOK = false
			status = "degraded"
		}

		c.JSON(http.StatusOK, gin.H{
			"status":      status,
			"service":     cfg.App.Name,
			"version":     Version,
			"env":         cfg.App.Env,
			"mysql_ok":    mysqlOK,
			"postgres_ok": postgresOK,
			"redis_ok":    redisOK,
		})
	})

	// ====== 业务路由（按模块分组注册）======
	v1 := r.Group("/api/v1")

	userMod := user.NewModule(db.MySQL)
	userMod.RegisterRoutes(v1)

	return r
}
