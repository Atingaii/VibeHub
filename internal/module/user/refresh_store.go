package user

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/vibeshop/vibeshop/internal/cache"
)

// ErrSessionMismatch 表示 Rotate/Revoke 校验失败：旧 key 不存在 / hash 不匹配。
// Rotate 调用方（service.Refresh）映射为 401 INVALID_TOKEN；
// Revoke 调用方（service.Logout）按幂等吞掉。
var ErrSessionMismatch = errors.New("user: refresh session mismatch")

// ErrSessionCorrupted 表示 Redis 中存的 value 解码失败（理论上不应发生）。
// 调用方应映射为 500，便于运维排查（不要让损坏数据被默默当成 mismatch 处理）。
var ErrSessionCorrupted = errors.New("user: refresh session value corrupted")

// RefreshStore 提供 refresh token 在 Redis PoolSession 的存储能力。
//
// 所有 mutation 走 Lua 脚本，保证 compare-then-write/delete 原子，
// 防止并发场景下同一旧 token 通过校验后产出多个新 session（见
// docs/features/1.2-user-login.md v3 修订）。
type RefreshStore interface {
	// Save 登录时写入新 session（SETEX）。覆盖同 jti 已有 key（不应发生，jti 由 crypto/rand 生成）。
	Save(ctx context.Context, uid uint64, jti, tokenHash string, ttl time.Duration) error

	// Rotate 用旧 (jti, hash) 原子换成新 (newJti, newHash, ttl)：
	// Lua 内部一次完成 compare(oldKey, hash) → DEL oldKey → SET newKey newValue EX ttl。
	// 旧 hash 不匹配 / 旧 key 不存在 → 整体不动，返回 ErrSessionMismatch。
	Rotate(ctx context.Context, uid uint64, oldJTI, oldHash, newJTI, newHash string, ttl time.Duration) error

	// Revoke 校验 (jti, hash) 匹配则 DEL。不匹配 / 不存在均视为成功（幂等）。
	// 由 Lua 保证 compare-then-DEL 原子。
	Revoke(ctx context.Context, uid uint64, jti, hash string) error
}

// HashRefreshToken 对 refresh token 原文做 SHA-256 hex 编码。
// refresh token 是高熵随机串，SHA-256 已足够，不必上 bcrypt（refresh 是热路径）。
func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// refreshValue 是写入 Redis 的 value JSON。
type refreshValue struct {
	TokenHash string `json:"token_hash"`
	IssuedAt  int64  `json:"issued_at"`
}

// rotateScript: KEYS[1]=oldKey, KEYS[2]=newKey
// ARGV[1]=oldHash, ARGV[2]=newValue (json), ARGV[3]=ttlSeconds
// 返回值：
//
//	 1 = 成功
//	 0 = mismatch（旧 key 不存在 或 hash 不匹配）
//	-1 = decode/schema failure（Redis value 不是预期 JSON 或缺 token_hash 字段，调用方按 500 处理）
const rotateScript = `
local raw = redis.call("GET", KEYS[1])
if not raw then
  return 0
end
local ok, parsed = pcall(cjson.decode, raw)
if not ok or type(parsed) ~= "table" or type(parsed.token_hash) ~= "string" then
  return -1
end
if parsed.token_hash ~= ARGV[1] then
  return 0
end
redis.call("DEL", KEYS[1])
redis.call("SET", KEYS[2], ARGV[2], "EX", ARGV[3])
return 1
`

// revokeScript: KEYS[1]=key, ARGV[1]=expectedHash
// 返回值：
//
//	 1 = 已删除
//	 0 = mismatch / not-found（调用方按幂等成功处理）
//	-1 = decode/schema failure（调用方按 500 处理）
const revokeScript = `
local raw = redis.call("GET", KEYS[1])
if not raw then
  return 0
end
local ok, parsed = pcall(cjson.decode, raw)
if not ok or type(parsed) ~= "table" or type(parsed.token_hash) ~= "string" then
  return -1
end
if parsed.token_hash ~= ARGV[1] then
  return 0
end
redis.call("DEL", KEYS[1])
return 1
`

// redisRefreshStore 是默认实现，绑定到 PoolSession。
type redisRefreshStore struct {
	client redis.UniversalClient
	now    func() time.Time
}

// NewRedisRefreshStore 构造默认实现，从 RedisManager.Pool(PoolSession) 拿 client。
func NewRedisRefreshStore(client redis.UniversalClient) RefreshStore {
	return &redisRefreshStore{client: client, now: time.Now}
}

func (s *redisRefreshStore) Save(ctx context.Context, uid uint64, jti, tokenHash string, ttl time.Duration) error {
	val := refreshValue{TokenHash: tokenHash, IssuedAt: s.now().Unix()}
	raw, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("marshal refresh value: %w", err)
	}
	return s.client.Set(ctx, cache.UserRefreshKey(uid, jti), raw, ttl).Err()
}

func (s *redisRefreshStore) Rotate(ctx context.Context, uid uint64, oldJTI, oldHash, newJTI, newHash string, ttl time.Duration) error {
	val := refreshValue{TokenHash: newHash, IssuedAt: s.now().Unix()}
	raw, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("marshal refresh value: %w", err)
	}
	res, err := s.client.Eval(ctx, rotateScript,
		[]string{cache.UserRefreshKey(uid, oldJTI), cache.UserRefreshKey(uid, newJTI)},
		oldHash, string(raw), int(ttl.Seconds()),
	).Int64()
	if err != nil {
		return fmt.Errorf("rotate eval: %w", err)
	}
	switch res {
	case 1:
		return nil
	case 0:
		return ErrSessionMismatch
	case -1:
		return ErrSessionCorrupted
	default:
		return fmt.Errorf("rotate: unexpected lua return %d", res)
	}
}

func (s *redisRefreshStore) Revoke(ctx context.Context, uid uint64, jti, hash string) error {
	res, err := s.client.Eval(ctx, revokeScript,
		[]string{cache.UserRefreshKey(uid, jti)},
		hash,
	).Int64()
	if err != nil {
		return fmt.Errorf("revoke eval: %w", err)
	}
	switch res {
	case 1, 0:
		// 1 = 已删除（hash 匹配）
		// 0 = mismatch / not-found，按幂等成功处理（service 层不报错）
		return nil
	case -1:
		return ErrSessionCorrupted
	default:
		return fmt.Errorf("revoke: unexpected lua return %d", res)
	}
}
