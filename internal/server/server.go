package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vibeshop/vibeshop/internal/cache"
	"github.com/vibeshop/vibeshop/internal/config"
	"github.com/vibeshop/vibeshop/internal/database"
	"go.uber.org/zap"
)

// Server 封装 HTTP server
type Server struct {
	httpServer *http.Server
	cfg        *config.Config
	db         *database.Manager
	redis      *cache.RedisManager
}

// NewServer 创建 HTTP server，从配置读取端口和超时参数
func NewServer(cfg *config.Config, db *database.Manager, rds *cache.RedisManager) (*Server, error) {
	router, err := SetupRouter(cfg, db, rds)
	if err != nil {
		return nil, err
	}

	return &Server{
		cfg:   cfg,
		db:    db,
		redis: rds,
		httpServer: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", cfg.Gateway.Host, cfg.Gateway.Port),
			Handler:      router,
			ReadTimeout:  cfg.Gateway.ReadTimeout,
			WriteTimeout: cfg.Gateway.WriteTimeout,
		},
	}, nil
}

// Run 启动 HTTP server，支持优雅关闭
func (s *Server) Run() error {
	// 优雅关闭信号监听
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 异步启动 server
	go func() {
		zap.L().Info("[server] VibeShop listening",
			zap.String("addr", s.httpServer.Addr),
		)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zap.L().Fatal("[server] listen error", zap.Error(err))
		}
	}()

	// 阻塞等待关闭信号
	sig := <-quit
	zap.L().Info("[server] shutting down...", zap.String("signal", sig.String()))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}

	zap.L().Info("[server] exited cleanly")
	return nil
}
