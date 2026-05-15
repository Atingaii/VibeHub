package main

import (
	"os"

	"github.com/vibeshop/vibeshop/internal/cache"
	"github.com/vibeshop/vibeshop/internal/config"
	"github.com/vibeshop/vibeshop/internal/database"
	"github.com/vibeshop/vibeshop/internal/logger"
	"github.com/vibeshop/vibeshop/internal/server"
	"go.uber.org/zap"
)

// Version 和 BuildTime 由 ldflags 注入
var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	// 将版本信息传递给 server 包
	server.Version = Version
	server.BuildTime = BuildTime

	// 加载配置（从 configs/ 目录读取，支持 APP_ENV 切换环境）
	cfg, err := config.Load("configs")
	if err != nil {
		// 配置加载失败时 logger 尚未初始化，用 stderr 输出
		os.Stderr.WriteString("[fatal] failed to load config: " + err.Error() + "\n")
		os.Exit(1)
	}

	// 初始化结构化日志
	if err := logger.Init(cfg.Observability.LogLevel, cfg.Observability.LogFormat); err != nil {
		os.Stderr.WriteString("[fatal] failed to init logger: " + err.Error() + "\n")
		os.Exit(1)
	}
	defer logger.Sync()

	zap.L().Info("[main] VibeShop starting",
		zap.String("version", Version),
		zap.String("build_time", BuildTime),
		zap.String("env", cfg.App.Env),
		zap.Int("port", cfg.Gateway.Port),
		zap.Bool("debug", cfg.App.Debug),
	)

	// 初始化数据库（MySQL + PostgreSQL 双连接池）
	dbManager, err := database.New(&cfg.Database, cfg.App.Debug)
	if err != nil {
		zap.L().Fatal("[main] failed to init database", zap.Error(err))
	}
	defer func() {
		if err := dbManager.Close(); err != nil {
			zap.L().Error("[main] database close error", zap.Error(err))
		}
	}()

	// 初始化 Redis 连接
	redisManager, err := cache.NewRedis(&cfg.Redis)
	if err != nil {
		zap.L().Fatal("[main] failed to init redis", zap.Error(err))
	}
	defer func() {
		if err := redisManager.Close(); err != nil {
			zap.L().Error("[main] redis close error", zap.Error(err))
		}
	}()

	// 创建并启动 HTTP server
	srv := server.NewServer(cfg, dbManager, redisManager)
	if err := srv.Run(); err != nil {
		zap.L().Fatal("[main] server error", zap.Error(err))
	}
}
