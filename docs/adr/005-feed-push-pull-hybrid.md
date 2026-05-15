# ADR-005: 推拉结合 Feed 流

## 状态
已采纳 — 2025-05-09

## 背景

Feed 流是本平台的核心功能（类似知乎首页），需要在以下约束间平衡：
- 发布延迟：用户发博文后，粉丝应尽快看到
- 读取性能：用户刷 Feed 必须毫秒级响应
- 存储成本：不能因大 V 粉丝多导致存储爆炸
- 数据一致性：不能出现重复/丢失/乱序

## 决策

### 推拉结合模式

**阈值策略**：`PushThreshold = 2000`

```
IF author.followerCount < 2000:
    → Push 模式（写扩散）
    → 发布时异步写入所有粉丝的 inbox SortedSet
ELSE:
    → Pull 模式（读扩散）
    → 不写入粉丝 inbox，读取时实时聚合
```

### 数据结构

```
# 粉丝 inbox（推模式产物）
feed:inbox:{userId}  →  SortedSet{ postId: timestamp }
    TTL: 7 天内的帖子（超 7 天移到 archive）
    Max: 1000 条（超出自动 ZREMRANGEBYRANK 淘汰最旧）

# 作者 outbox（所有人都有）
feed:outbox:{authorId}  →  SortedSet{ postId: timestamp }
    TTL: 30 天
    Max: 500 条

# 大 V 列表（由 Feed 服务维护）
feed:bigv:set  →  Set{ authorId, ... }
```

### 读取流程

```
1. 从 inbox 取最新 N 条（推模式产物）
2. 获取用户关注的大 V 列表
3. 从各大 V outbox 取最新 M 条（拉模式）
4. 合并 + 按 timestamp 排序 + 去重
5. 应用 Rank 算法（可选：Wilson Score + 时间衰减）
6. 返回 Top K 条 + 游标
```

### 游标分页（核心创新）

**不用 OFFSET，用 keyset pagination**：

```go
type FeedCursor struct {
    LastTimestamp int64  // 上一页最后一条的时间戳
    LastPostID   string // 上一页最后一条的 ID（同一时间戳去重）
    Offset       int    // 同一时间戳内的偏移量
}
```

**查询**：
```
ZREVRANGEBYSCORE feed:inbox:{userId} (lastTimestamp 0 LIMIT offset count
```

**优势**：新数据插入不影响分页位置，彻底解决"翻页重复"问题。

### 排序算法

**默认排序**：时间线（最新优先）

**热度排序**（可选切换）：

```go
// Wilson Score + 时间衰减
func HotScore(upvotes, downvotes int, createdAt time.Time) float64 {
    n := float64(upvotes + downvotes)
    if n == 0 { return 0 }
    
    z := 1.96 // 95% 置信区间
    p := float64(upvotes) / n
    
    // Wilson Score Lower Bound
    wilson := (p + z*z/(2*n) - z*math.Sqrt(p*(1-p)/n+z*z/(4*n*n))) / (1 + z*z/n)
    
    // 时间衰减（半衰期 24 小时）
    hours := time.Since(createdAt).Hours()
    decay := math.Pow(0.5, hours/24.0)
    
    return wilson * decay
}
```

### 写扩散流程（NATS 异步）

```
用户发布 Post
    → 写入 PG posts 表
    → 写入 Redis feed:outbox:{authorId}
    → 发 NATS "feed.push.fanout" 消息
    
NATS Consumer:
    → 查询 author 的粉丝列表（分批 1000）
    → 批量 ZADD 到每个粉丝的 inbox
    → 大 V 跳过（不推）
```

## 权衡

| 方案 | 发布延迟 | 读取延迟 | 存储成本 | 复杂度 |
|------|---------|---------|---------|-------|
| 纯推 | 高（大V灾难） | 极低 | 极高 | 低 |
| 纯拉 | 无 | 高（聚合慢） | 低 | 中 |
| **推拉结合** | 低 | 低 | 中 | 高 |

**接受复杂度的理由**：Feed 流是核心体验，值得投入复杂度换取最优用户体验。2000 阈值可根据实际数据动态调整。

## 推翻条件

- 用户量 < 1 万 → 纯推模式即可（简单直接）
- 算法推荐成为主要分发手段 → Feed 流退化为推荐列表（不再依赖关注关系）
- Redis 内存成本过高 → inbox 数据下沉到 Cassandra（牺牲部分延迟）

## 代码锚点

- `pkg/feed/config.go:PushThreshold` — 推拉阈值
- `pkg/feed/timeline.go` — inbox/outbox 操作
- `pkg/feed/ranker.go` — 排序算法
- `pkg/feed/cursor.go` — 游标分页
- `cmd/feed-svc/logic/push_fanout.go` — 写扩散 NATS 消费者
- `cmd/feed-svc/logic/pull_merge.go` — 读聚合逻辑
