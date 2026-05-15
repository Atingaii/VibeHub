# ADR-003: Redis 统一缓存 + Feed 流存储

## 状态
已采纳 — 2025-05-09

## 背景

项目有多个需要低延迟响应的场景：
1. 商品缓存（页面渲染毫秒级）
2. Feed 流时间线（滑动刷新即时响应）
3. 库存预扣减（秒杀/拼团高并发）
4. 分布式锁（防重复下单/超卖）
5. 会话管理（JWT Token 黑名单）
6. 限流计数（API rate-limit）

需要一个统一的缓存基础设施来覆盖以上所有场景。

## 决策

**Redis 7.x 作为统一缓存层**，按职责分 DB（0-5）：

| DB | 用途 | 数据结构 | 淘汰策略 |
|----|------|---------|---------|
| 0 | 通用缓存（商品/用户） | String/Hash | allkeys-lru |
| 1 | Feed 流时间线 | SortedSet | noeviction |
| 2 | 库存/锁/计数 | String + Lua | noeviction |
| 3 | 会话/Token | String(TTL) | volatile-ttl |
| 4 | 消息/通知 | List/Stream | allkeys-lru |
| 5 | 热数据排行榜 | SortedSet | noeviction |

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

## 扩展路径

1. **初期**：Redis 单节点 + AOF 持久化
2. **中期**：Redis Sentinel（一主两从，自动故障转移）
3. **后期**：Redis Cluster（分片，Feed 流按 userId hash）

## 推翻条件

- Feed 流用户量 > 1000 万且 SortedSet 内存爆炸 → 考虑时间线数据下沉到 Cassandra/ScyllaDB
- 需要消息队列语义的场景过多 → 把 DB4 的 Stream 迁移到 NATS JetStream

## 代码锚点

- `internal/cache/redis.go` — Redis 连接池初始化
- `internal/cache/keys.go` — 所有 key pattern 定义
- `pkg/feed/timeline.go` — Feed 流 SortedSet 操作
- `internal/cache/stock.lua` — 库存扣减 Lua 脚本
- `internal/cache/lock.go` — 分布式锁封装
