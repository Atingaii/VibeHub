// Package cache 提供 Redis 缓存层统一管理。
// 代码锚点：internal/cache/redis.go（ADR-003）
package cache

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/vibeshop/vibeshop/internal/config"
	"go.uber.org/zap"
)

// RedisManager 管理 Redis 客户端连接。
//
// 单实例阶段：内部维护一个底层 Options（来自 cfg），按 Pool 懒加载独立的
// *redis.Client（每个 client 对应一个 DB 索引）。对外只返回 redis.UniversalClient
// 接口，为后续把某些 Pool 升级到 ClusterClient/Sentinel 留口子。
type RedisManager struct {
	baseOpts *redis.Options

	mu             sync.RWMutex
	pools          map[Pool]redis.UniversalClient
	closed         bool
	closedSentinel redis.UniversalClient // Close 后给 Pool() 返回的哨兵（命令均返回 ErrClosedClient）
}

// NewRedis 初始化 RedisManager，启动时通过 PoolGeneral 做 Ping 验证。
func NewRedis(cfg *config.RedisConfig) (*RedisManager, error) {
	maskedPassword := ""
	if cfg.Password != "" {
		maskedPassword = "***"
	}
	zap.L().Info("[cache] connecting to Redis...",
		zap.String("addr", cfg.Addr),
		zap.String("password", maskedPassword),
		zap.Int("pool_size", cfg.PoolSize),
		zap.Int("min_idle_conns", cfg.MinIdleConns),
	)

	m := &RedisManager{
		baseOpts: &redis.Options{
			Addr:         cfg.Addr,
			Password:     cfg.Password,
			PoolSize:     cfg.PoolSize,
			MinIdleConns: cfg.MinIdleConns,
		},
		pools: make(map[Pool]redis.UniversalClient),
	}

	general := m.Pool(PoolGeneral)

	var pingErr error
	for i := range 3 {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		pingErr = general.Ping(ctx).Err()
		cancel()
		if pingErr == nil {
			break
		}
		zap.L().Warn("[cache] Redis connect failed, retrying...",
			zap.Int("attempt", i+1),
			zap.Error(pingErr),
		)
		time.Sleep(2 * time.Second)
	}
	if pingErr != nil {
		_ = m.Close()
		return nil, fmt.Errorf("redis connect: after 3 attempts: %w", pingErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	info, infoErr := general.Info(ctx, "server").Result()
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
		zap.Int("pool_size", cfg.PoolSize),
		zap.Int("min_idle_conns", cfg.MinIdleConns),
		zap.String("redis_version", redisVersion),
	)

	return m, nil
}

// ErrClosed 表示 RedisManager 已被关闭。
//
// Close 后再调用 Pool/Client/Ping 会返回一个已关闭的 client；其上的命令会
// 由 go-redis 返回 `redis: client is closed`（语义等价 ErrClosed），不会
// 触发 nil-map panic。该错误常量本身用于上游显式比对的场景。
var ErrClosed = errors.New("cache: RedisManager is closed")

// Pool 返回指定 Pool 对应的 client，懒加载。
//
// 返回 redis.UniversalClient 是为了后续把某个 Pool 升级到 ClusterClient/
// Sentinel 时不破坏调用方签名。
//
// Close 后再调用会返回一个已关闭的哨兵 client；其上的命令会失败而不是
// 触发 nil-map panic。
func (m *RedisManager) Pool(p Pool) redis.UniversalClient {
	m.mu.RLock()
	if m.closed {
		s := m.closedSentinel
		m.mu.RUnlock()
		return s
	}
	if c, ok := m.pools[p]; ok {
		m.mu.RUnlock()
		return c
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return m.closedSentinel
	}
	if c, ok := m.pools[p]; ok {
		return c
	}

	opts := *m.baseOpts
	opts.DB = p.DBIndex()
	c := redis.NewClient(&opts)
	m.pools[p] = c
	return c
}

// Client 返回 PoolGeneral 对应的 client（DB0），保留旧调用面。
func (m *RedisManager) Client() redis.UniversalClient {
	return m.Pool(PoolGeneral)
}

// Ping 通过 PoolGeneral 检查 Redis 连接健康状态。
func (m *RedisManager) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return m.Pool(PoolGeneral).Ping(ctx).Err()
}

// Close 关闭所有已初始化的 Pool client。重复调用安全。
//
// Close 后 Pool/Client/Ping 会返回一个已关闭的哨兵 client；其上的命令会
// 由 go-redis 返回 `redis: client is closed`，而不是触发 nil-map panic。
func (m *RedisManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	zap.L().Info("[cache] closing Redis connections", zap.Int("pools", len(m.pools)))

	var errs []error
	sentinel := redis.NewClient(&redis.Options{Addr: m.baseOpts.Addr})
	if err := sentinel.Close(); err != nil {
		errs = append(errs, fmt.Errorf("init closed sentinel: %w", err))
	}
	m.closedSentinel = sentinel

	for p, c := range m.pools {
		if err := c.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close pool %s: %w", p, err))
		}
		delete(m.pools, p)
	}
	m.closed = true
	return errors.Join(errs...)
}
