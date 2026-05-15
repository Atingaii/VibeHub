// Package cache 提供 Redis 缓存层统一管理。
// 代码锚点：internal/cache/redis.go（ADR-003）
package cache

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/vibeshop/vibeshop/internal/config"
	"go.uber.org/zap"
)

// RedisManager 管理 Redis 客户端连接
type RedisManager struct {
	client *redis.Client
}

// NewRedis 初始化 Redis 客户端，启动时 Ping 验证 + 重试
func NewRedis(cfg *config.RedisConfig) (*RedisManager, error) {
	// 日志脱敏：密码不输出
	maskedPassword := ""
	if cfg.Password != "" {
		maskedPassword = "***"
	}
	zap.L().Info("[cache] connecting to Redis...",
		zap.String("addr", cfg.Addr),
		zap.String("password", maskedPassword),
		zap.Int("db", cfg.DB),
		zap.Int("pool_size", cfg.PoolSize),
		zap.Int("min_idle_conns", cfg.MinIdleConns),
	)

	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
	})

	// Ping 验证 + 重试（3 次，间隔 2 秒）
	var err error
	for i := range 3 {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err = client.Ping(ctx).Err()
		cancel()
		if err == nil {
			break
		}

		zap.L().Warn("[cache] Redis connect failed, retrying...",
			zap.Int("attempt", i+1),
			zap.Error(err),
		)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("redis connect: after 3 attempts: %w", err)
	}

	// 获取 Redis server 信息
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	info, infoErr := client.Info(ctx, "server").Result()
	redisVersion := "unknown"
	if infoErr == nil {
		for line := range strings.SplitSeq(info, "\n") {
			if v, ok := strings.CutPrefix(line, "redis_version:"); ok {
				redisVersion = strings.TrimSpace(v)
				break
			}
		}
	}

	zap.L().Info("[cache] Redis connected",
		zap.String("addr", cfg.Addr),
		zap.Int("db", cfg.DB),
		zap.Int("pool_size", cfg.PoolSize),
		zap.Int("min_idle_conns", cfg.MinIdleConns),
		zap.String("redis_version", redisVersion),
	)

	return &RedisManager{client: client}, nil
}

// Client 返回底层 redis.Client（供业务层使用）
func (m *RedisManager) Client() *redis.Client {
	return m.client
}

// Ping 检查 Redis 连接健康状态
func (m *RedisManager) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return m.client.Ping(ctx).Err()
}

// Close 关闭 Redis 连接
func (m *RedisManager) Close() error {
	zap.L().Info("[cache] closing Redis connection")
	return m.client.Close()
}
