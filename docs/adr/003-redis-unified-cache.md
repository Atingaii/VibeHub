# ADR-003: Redis 统一缓存 + Pool 抽象 + Feed 流存储

## 状态
已采纳 — 2025-05-09（修订 2026-05-16：纠正 OSS per-DB 误述，引入 Pool 抽象）

## 背景

项目有多个需要低延迟响应的场景：
1. 商品缓存（页面渲染毫秒级）
2. Feed 流时间线（滑动刷新即时响应）
3. 库存预扣减（秒杀/拼团高并发）
4. 分布式锁（防重复下单/超卖）
5. 会话管理（JWT Token 黑名单）
6. 限流计数（API rate-limit）

需要一个统一的缓存基础设施来覆盖以上所有场景。

### OSS Redis 限制（重要）

Redis 7.x 单实例（OSS）的 `maxmemory-policy` 是**实例级**配置：所有 16 个逻辑 DB
共享同一个内存池、同一个 maxmemory、同一个淘汰策略。`SELECT n` 切换的只是
keyspace 哈希表，**不能**让 DB1 用 noeviction、DB2 用 allkeys-lru 共存。
要真正按用途分策略，必须拆物理实例（容器/集群）或上 Redis Enterprise。

本 ADR 的早期版本将"per-DB 淘汰策略"写成事实，是错误的——表里那一列是
**语义期望**，不是 server 配置。本次修订纠正这一点。

## 决策

### 1) 实例级实际策略

`deploy/docker/docker-compose.yml` 与 `docker-compose.infra.yml` 的 redis 服务
统一配 `--maxmemory 512mb --maxmemory-policy noeviction`。

为什么 noeviction：库存计数、分布式锁、Feed 时间线一旦被静默淘汰会引发超卖
和数据不一致。noeviction 让内存满时写入显式失败（OOM error），由业务层处理，
而不是悄悄丢数据。代价是必须监控内存使用 + 缓存类 Pool 主动用 TTL 收缩。

### 2) Pool 抽象

业务层不直接持有 `*redis.Client`，而是通过 `RedisManager.Pool(p)` 取语义分组
对应的 client。Pool 拓扑唯一权威定义在 `internal/cache/keys.go`。

| Pool | DB | 用途 | 数据结构 | TTL 约束 | 语义期望淘汰 |
|------|----|------|----------|---------|------------|
| `general` | 0 | 通用缓存（商品/用户） | String/Hash | 必须 TTL | LRU |
| `feed` | 1 | Feed 时间线 | SortedSet | 不带 TTL（按容量裁剪） | noeviction |
| `stock` | 2 | 库存 / 锁 / 计数 | String + Lua | 不带 TTL | noeviction |
| `session` | 3 | 会话 / Token | String(TTL) | 必须 TTL | volatile-ttl |
| `notify` | 4 | 消息 / 通知 | List/Stream | 必须 TTL | LRU |
| `rank` | 5 | 排行榜 | SortedSet | 不带 TTL | noeviction |

注：表中的 "TTL 约束" 与 "语义期望淘汰" 是**阶段一元数据**，不在写路径强校验。
单实例阶段实际跑的是 server 级统一 noeviction，元数据用于：
- 提醒新增缓存代码时要不要带 TTL（Pool.RequiresTTL()）
- 多实例化迁移时的拆分依据
- 后续可在 cache writer 层加单元测试或 lint 强制约束

### 3) 多实例化迁移路径

返回类型用 `redis.UniversalClient` 而非 `*redis.Client`，让某个 Pool 升级到
ClusterClient/Sentinel 时业务调用面不破坏。迁移步骤：

1. `RedisConfig` 扩字段 `pools map[Pool]PoolConfig`（`addr`/`password`/`mode`）
2. `RedisManager.Pool(p)` 优先读 pools 配置；缺省回退到 baseOpts
3. 把 `stock` 和 `feed` 先拆出去（最高价值，超卖风险大），剩余 Pool 暂留单实例
4. 业务代码 0 改动

### Feed 流存储设计

```
Key:   feed:inbox:{userId}
Type:  SortedSet
Score: Unix timestamp (ms)
Member: postId

Key:   feed:outbox:{authorId}
Type:  SortedSet
Score: Unix timestamp (ms)
Member: postId
```

**分页查询**（游标模式）：
```
ZREVRANGEBYSCORE feed:inbox:{userId} (lastScore 0 LIMIT offset count
```

### 库存预扣方案

```lua
-- Lua 脚本保证原子性
local stock = tonumber(redis.call('GET', KEYS[1]))
if stock <= 0 then return -1 end
redis.call('DECR', KEYS[1])
return stock - 1
```

### 分布式锁

采用 Redisson 模式（Go 实现：`go-redsync/redsync`）：
- 可重入锁
- 自动续期（watchdog）
- 超时自动释放

## 权衡

**不选 Dragonfly 的理由**：虽然吞吐量高 5 倍、内存效率好 30%，但生态太新、Go SDK 兼容性未经大规模验证、社区资源少。项目初期稳定性 > 极致性能。

**不选 KeyDB 的理由**：虽然多线程性能好，但社区活跃度下降、长期维护不确定。

**为什么不在配置里暴露 pool→DB 映射**：单实例阶段映射是实现细节，硬编码在
`internal/cache/keys.go` 配合 R2 同步检查表足够。等多实例化时再扩配置面。

**Pool 懒加载副作用**：每个 Pool 一个 `*redis.Client`，连接池独立。按
`pool_size=100` 上限算理论最多 600 连接。短期可接受；后续若成为瓶颈，
重新设计 RedisManager 内部复用策略（具体方案验证后再定）。

## 扩展路径

1. **初期**（当前）：Redis 单节点 + AOF + 实例级 noeviction + Pool 抽象（DB 区分用途）
2. **中期**：拆 stock/feed 到独立实例 + Sentinel（一主两从，自动故障转移）
3. **后期**：Redis Cluster（分片，Feed 流按 userId hash）

## 推翻条件

- Feed 流用户量 > 1000 万且 SortedSet 内存爆炸 → 考虑时间线数据下沉到 Cassandra/ScyllaDB
- 需要消息队列语义的场景过多 → 把 notify pool 的 Stream 迁移到 NATS JetStream
- 单实例 600 连接成为瓶颈 → 重新设计 RedisManager 内部复用策略（共享 conn pool 配合按调用切换 SELECT、或拆 Pool 到独立 server）

## 代码锚点

- `internal/cache/redis.go:RedisManager` — 连接管理 + Pool 懒加载
- `internal/cache/keys.go:Pool` — Pool 拓扑唯一权威 + 元数据
- `internal/cache/keys.go:pools` — DB/TTL/描述注册表
- `pkg/feed/timeline.go` — Feed 流 SortedSet 操作（规划）
- `internal/cache/stock.lua` — 库存扣减 Lua 脚本（规划）
- `internal/cache/lock.go` — 分布式锁封装（规划）
