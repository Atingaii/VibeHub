package cache

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/redis/go-redis/v9"
)

func newTestManager() *RedisManager {
	return &RedisManager{
		baseOpts: &redis.Options{Addr: "127.0.0.1:1"},
		pools:    map[Pool]redis.UniversalClient{},
	}
}

// TestPoolAfterClose 验证 Close 后再调用 Pool/Client/Ping 不触发 nil-map panic，
// 而是返回一个已关闭的哨兵 client（命令带 closed 错误）。
func TestPoolAfterClose(t *testing.T) {
	m := newTestManager()

	if err := m.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("second Close (idempotent): %v", err)
	}

	c := m.Pool(PoolGeneral)
	if c == nil {
		t.Fatal("Pool after Close returned nil")
	}
	if err := c.Ping(context.Background()).Err(); err == nil ||
		!strings.Contains(err.Error(), "closed") {
		t.Fatalf("expected closed-client error from Pool client, got %v", err)
	}
	if err := m.Ping(); err == nil ||
		!strings.Contains(err.Error(), "closed") {
		t.Fatalf("expected closed-client error from m.Ping, got %v", err)
	}
}

// TestConcurrentPoolAndClose 验证并发调用 Pool 和 Close 不会触发数据竞争或 panic。
// 用 `go test -race` 验证。
func TestConcurrentPoolAndClose(t *testing.T) {
	m := newTestManager()

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Pool panicked under race: %v", r)
				}
			}()
			_ = m.Pool(PoolGeneral)
			_ = m.Pool(PoolFeed)
			_ = m.Pool(PoolStock)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = m.Close()
	}()
	wg.Wait()

	if c := m.Pool(PoolSession); c == nil {
		t.Fatal("Pool after concurrent Close returned nil")
	}
}

func TestPoolMetadata(t *testing.T) {
	cases := []struct {
		p      Pool
		db     int
		ttlReq bool
	}{
		{PoolGeneral, 0, true},
		{PoolFeed, 1, false},
		{PoolStock, 2, false},
		{PoolSession, 3, true},
		{PoolNotify, 4, true},
		{PoolRank, 5, false},
	}
	for _, c := range cases {
		if got := c.p.DBIndex(); got != c.db {
			t.Errorf("%s.DBIndex()=%d want %d", c.p, got, c.db)
		}
		if got := c.p.RequiresTTL(); got != c.ttlReq {
			t.Errorf("%s.RequiresTTL()=%v want %v", c.p, got, c.ttlReq)
		}
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for unknown pool")
		}
	}()
	_ = Pool("unknown").DBIndex()
}
