// Package cache 提供 Redis 缓存层统一管理。
//
// 本文件定义 Redis 用途的语义分组（Pool）。每个 Pool 对应一个固定的逻辑 DB
// 编号 + 一份元数据（淘汰期望、TTL 约束）。业务调用 RedisManager.Pool(p)
// 拿对应 client，避免硬编码 DB 数字。
//
// 单实例阶段：所有 Pool 共享同一个 server，仅 DB 不同；server 端实例级
// maxmemory-policy 统一为 noeviction（见 ADR-003）。RequiresTTL 等元数据
// 是阶段一约束，不在写路径强校验。
//
// 多实例阶段：未来可在 RedisConfig 上扩 pools map[Pool]PoolConfig，让某些
// Pool 走独立 server / Cluster；返回 redis.UniversalClient 就是为这步留口子。
package cache

import (
	"fmt"
	"strconv"
)

// Pool 标识一个语义分组，对应 ADR-003 的用途分区。
type Pool string

const (
	PoolGeneral Pool = "general" // 通用缓存（商品/用户）
	PoolFeed    Pool = "feed"    // Feed 时间线 SortedSet
	PoolStock   Pool = "stock"   // 库存 / 分布式锁 / 计数
	PoolSession Pool = "session" // 会话 / Token
	PoolNotify  Pool = "notify"  // 消息 / 通知
	PoolRank    Pool = "rank"    // 热数据排行榜
)

// pools 注册表是 Pool 拓扑唯一权威来源。新增用途必须先在这里登记。
var pools = map[Pool]poolMeta{
	PoolGeneral: {db: 0, requiresTTL: true, desc: "通用缓存（商品/用户）"},
	PoolFeed:    {db: 1, requiresTTL: false, desc: "Feed 时间线 SortedSet"},
	PoolStock:   {db: 2, requiresTTL: false, desc: "库存预扣 / 分布式锁 / 计数"},
	PoolSession: {db: 3, requiresTTL: true, desc: "会话 / Token（必须 TTL）"},
	PoolNotify:  {db: 4, requiresTTL: true, desc: "消息 / 通知"},
	PoolRank:    {db: 5, requiresTTL: false, desc: "热数据排行榜"},
}

type poolMeta struct {
	db          int
	requiresTTL bool
	desc        string
}

// DBIndex 返回该 Pool 在单实例阶段使用的逻辑 DB 编号。
func (p Pool) DBIndex() int {
	m, ok := pools[p]
	if !ok {
		panic(fmt.Sprintf("cache: unknown pool %q", p))
	}
	return m.db
}

// RequiresTTL 表示写入此 Pool 的 key 是否必须带 TTL。
// 元数据仅供调用方自检；实例级策略已是 noeviction，不会静默淘汰。
func (p Pool) RequiresTTL() bool {
	m, ok := pools[p]
	if !ok {
		panic(fmt.Sprintf("cache: unknown pool %q", p))
	}
	return m.requiresTTL
}

// Description 返回 Pool 用途的人类可读说明。
func (p Pool) Description() string {
	m, ok := pools[p]
	if !ok {
		return string(p)
	}
	return m.desc
}

// === 业务 key 模式 ===
// 所有跨模块共享的 Redis key pattern 集中在这里（change-impact R2 规则）。
// 单模块自用的临时 key 可在模块内定义，但任何在多个 endpoint / 任务间共享的 key
// 必须在此登记，便于 grep 出"谁在写这个 key"。

// UserRefreshKey 生成 user 模块的 refresh token 存储 key。
// Pool 必须是 PoolSession（DB3，RequiresTTL=true）。
// 由 1.2 用户登录引入，详见 docs/features/1.2-user-login.md。
func UserRefreshKey(uid uint64, jti string) string {
	return "user:refresh:" + strconv.FormatUint(uid, 10) + ":" + jti
}
