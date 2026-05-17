package user

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newMiniRedisStore(t *testing.T) (RefreshStore, *miniredis.Miniredis, redis.UniversalClient) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisRefreshStore(client), mr, client
}

func TestRefreshStore_SaveAndRotate(t *testing.T) {
	store, mr, _ := newMiniRedisStore(t)
	ctx := context.Background()

	if err := store.Save(ctx, 1, "jti-old", "hash-old", time.Minute); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Rotate(ctx, 1, "jti-old", "hash-old", "jti-new", "hash-new", time.Minute); err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if mr.Exists("user:refresh:1:jti-old") {
		t.Fatal("old key should have been deleted")
	}
	if !mr.Exists("user:refresh:1:jti-new") {
		t.Fatal("new key should exist")
	}
}

func TestRefreshStore_Rotate_MismatchHash(t *testing.T) {
	store, _, _ := newMiniRedisStore(t)
	ctx := context.Background()
	_ = store.Save(ctx, 1, "jti", "hash-real", time.Minute)
	err := store.Rotate(ctx, 1, "jti", "hash-wrong", "jti2", "h2", time.Minute)
	if !errors.Is(err, ErrSessionMismatch) {
		t.Fatalf("expected ErrSessionMismatch, got %v", err)
	}
}

func TestRefreshStore_Rotate_KeyNotFound(t *testing.T) {
	store, _, _ := newMiniRedisStore(t)
	err := store.Rotate(context.Background(), 1, "absent", "h", "new", "h2", time.Minute)
	if !errors.Is(err, ErrSessionMismatch) {
		t.Fatalf("expected ErrSessionMismatch, got %v", err)
	}
}

// 并发只允许一次成功——对应 v2 codex 反馈的核心担忧。
func TestRefreshStore_Rotate_ConcurrentOnlyOneWins(t *testing.T) {
	store, _, _ := newMiniRedisStore(t)
	ctx := context.Background()
	_ = store.Save(ctx, 1, "jti-old", "hash-old", time.Minute)

	const goroutines = 16
	var success atomic.Int32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			err := store.Rotate(ctx, 1, "jti-old", "hash-old", "jti-new-"+string(rune('a'+idx)), "h-new", time.Minute)
			if err == nil {
				success.Add(1)
			} else if !errors.Is(err, ErrSessionMismatch) {
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if got := success.Load(); got != 1 {
		t.Fatalf("expected exactly 1 success in concurrent rotate, got %d", got)
	}
}

func TestRefreshStore_Revoke_HashMatch(t *testing.T) {
	store, mr, _ := newMiniRedisStore(t)
	ctx := context.Background()
	_ = store.Save(ctx, 1, "jti", "hash", time.Minute)
	if err := store.Revoke(ctx, 1, "jti", "hash"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if mr.Exists("user:refresh:1:jti") {
		t.Fatal("key should have been deleted")
	}
}

func TestRefreshStore_Revoke_HashMismatch_DoesNotDelete(t *testing.T) {
	store, mr, _ := newMiniRedisStore(t)
	ctx := context.Background()
	_ = store.Save(ctx, 1, "jti", "hash-real", time.Minute)
	// 用别人/旧 token 的 hash 调 Revoke：仍返回 nil（幂等），但 key 不应被删
	if err := store.Revoke(ctx, 1, "jti", "hash-wrong"); err != nil {
		t.Fatalf("Revoke should be idempotent: %v", err)
	}
	if !mr.Exists("user:refresh:1:jti") {
		t.Fatal("key should NOT have been deleted on hash mismatch")
	}
}

func TestRefreshStore_Revoke_KeyNotFound_Idempotent(t *testing.T) {
	store, _, _ := newMiniRedisStore(t)
	if err := store.Revoke(context.Background(), 1, "absent", "h"); err != nil {
		t.Fatalf("Revoke on missing key should be nil: %v", err)
	}
}

func TestRefreshStore_TTL(t *testing.T) {
	store, mr, _ := newMiniRedisStore(t)
	ctx := context.Background()
	_ = store.Save(ctx, 1, "jti", "h", time.Hour)
	ttl := mr.TTL("user:refresh:1:jti")
	if ttl <= 0 || ttl > time.Hour {
		t.Fatalf("expected TTL in (0, 1h], got %v", ttl)
	}
}

func TestRefreshStore_Rotate_Corrupted(t *testing.T) {
	store, mr, _ := newMiniRedisStore(t)
	ctx := context.Background()
	// 直接写入坏数据
	if err := mr.Set("user:refresh:1:jti", "{not-json"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := store.Rotate(ctx, 1, "jti", "h", "newjti", "newh", time.Minute)
	if !errors.Is(err, ErrSessionCorrupted) {
		t.Fatalf("expected ErrSessionCorrupted, got %v", err)
	}
}

func TestRefreshStore_Revoke_Corrupted(t *testing.T) {
	store, mr, _ := newMiniRedisStore(t)
	if err := mr.Set("user:refresh:1:jti", "garbage"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := store.Revoke(context.Background(), 1, "jti", "h"); !errors.Is(err, ErrSessionCorrupted) {
		t.Fatalf("expected ErrSessionCorrupted, got %v", err)
	}
}

// 各种"可解码但 schema 不对"的 value 都应识别为 corrupted。
func TestRefreshStore_Revoke_SchemaErrors(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"empty_object", `{}`},
		{"array", `[]`},
		{"missing_token_hash", `{"foo":"bar"}`},
		{"token_hash_not_string", `{"token_hash":123}`},
		{"token_hash_null", `{"token_hash":null}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, mr, _ := newMiniRedisStore(t)
			if err := mr.Set("user:refresh:1:jti", tc.value); err != nil {
				t.Fatalf("seed: %v", err)
			}
			if err := store.Revoke(context.Background(), 1, "jti", "h"); !errors.Is(err, ErrSessionCorrupted) {
				t.Fatalf("expected ErrSessionCorrupted for %s, got %v", tc.name, err)
			}
		})
	}
}

func TestRefreshStore_Rotate_SchemaErrors(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"empty_object", `{}`},
		{"array", `[]`},
		{"missing_token_hash", `{"foo":"bar"}`},
		{"token_hash_not_string", `{"token_hash":123}`},
		{"token_hash_null", `{"token_hash":null}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, mr, _ := newMiniRedisStore(t)
			if err := mr.Set("user:refresh:1:jti", tc.value); err != nil {
				t.Fatalf("seed: %v", err)
			}
			err := store.Rotate(context.Background(), 1, "jti", "h", "newjti", "newh", time.Minute)
			if !errors.Is(err, ErrSessionCorrupted) {
				t.Fatalf("expected ErrSessionCorrupted for %s, got %v", tc.name, err)
			}
		})
	}
}
