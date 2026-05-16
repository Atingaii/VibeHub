# 04 — 主链路走读

> 4 条核心链路的白话讲解。每条配上"小白也能看懂的步骤" + 关键技术点 + 现状标注。
>
> 不熟的词请查 [00-glossary.md](00-glossary.md)。
>
> 注意：阶段 1-12 的业务模块都 [规划中]，本文叙述的是**已完成的设计**，不是已运行的代码。

---

## 链路 A — 拼团下单（开团 / 跟团 / 自动成团）

### 业务流程白话版

1. 用户 A 在某商品页点"开团"——选规格、付钱、把团号 `T123` 分享给朋友
2. 朋友 B 拿到链接进来"跟团"——同样选规格、付钱
3. 凑够人数（比如 3 人）→ 团号 `T123` **自动成团**，每个成员的订单变成"待发货"
4. 24 小时没凑够 → **自动退款**给所有已支付成员

### 后端怎么跑

```
前端 ─POST /groupbuy/create──┐
                              │
                              ▼
                          路由 router.go
                              │
                              ▼
                          groupbuy.Handler.Create
                              │
              ┌───────────────┼───────────────┐
              │               │               │
              ▼               ▼               ▼
        product.Service   stock.Lua脚本  order.Service
        查 SKU/价格      Redis 原子扣减  写订单 (MySQL)
              │               │               │
              └───────┬───────┴───────────────┘
                      │
                      ▼
                  发 NATS 消息
                  "groupbuy.timeout.30min"
                      │
            （30 分钟后由 NATS Consumer 收到）
                      │
                      ▼
                查团状态：满员？
                ├─ 是 → 全员订单标"待发货"
                └─ 否 → 退款（调 payment.Service）
```

### 关键技术点

- **库存预扣防超卖**：用 [Lua 脚本](00-glossary.md#lua)在 Redis 端原子完成"读 → 判 > 0 → 扣"，整段不会被打断。Pool 走 `PoolStock`（DB 2，noeviction，不能静默丢）。
- **订单写 MySQL**：因为是交易数据（强一致 + 行级锁），按 [ADR-002](../adr/002-dual-database.md) 选 MySQL。
- **超时机制走 NATS JetStream 延迟投递**：发消息时带 `delivery delay 30min`，到点 NATS 投到 Consumer。比起业务侧轮询数据库优雅得多。
- **跨库一致性**（订单 MySQL ↔ 积分 / Feed PG）：走 [Saga 模式](00-glossary.md#saga)，本地事务 + 失败补偿。

### 现状

[规划中] 阶段 4 实现。`internal/cache/keys.go:PoolStock` 已就绪。Lua 脚本（`internal/cache/stock.lua`）和 NATS 客户端封装都还没写。

→ 权威：[docs/architecture.md](../architecture.md) "购物链路（拼团为例）" 段、[ADR-003](../adr/003-redis-unified-cache.md)、[ADR-004](../adr/004-nats-messaging.md)

---

## 链路 B — Feed 写扩散（普通作者发文 → 粉丝看到）

### 业务流程白话版

1. 普通作者 X（粉丝 800，< 2000 阈值）发了一篇博文
2. 平台希望粉丝**尽快**在自己的关注流里看到
3. 实现：发文时**立刻把帖子塞到所有粉丝的"收件箱"**——粉丝下次刷 Feed 时直接读自己收件箱即可

### 后端怎么跑

```
作者 X 点"发布"
    │
    ▼
content.Handler.Post (PG)
    │   写入 posts 表
    ▼
作者 outbox 也存一份
    │   ZADD feed:outbox:{X}  score=ts  member=postId
    │   (Redis Pool feed)
    ▼
发 NATS 消息 "feed.push.fanout"
    │
    ▼
Feed Consumer 收到
    │
    ├─ 查 X 的粉丝列表（PG follows 表，分批 1000）
    │
    └─ 对每个粉丝 P：
         ZADD feed:inbox:{P}  score=ts  member=postId
         (Redis Pool feed)

──── 粉丝 P 后来刷 Feed ──────

P ─GET /feed?cursor=...──┐
                          ▼
                  feed.Handler.List
                          │
                          ▼
                  ZREVRANGEBYSCORE feed:inbox:{P}
                          │
                          ▼
                  返回 Top N 帖子 + 下一页游标
```

### 关键技术点

- **为什么不在发布时同步等所有粉丝写完**：发布响应要快（用户体感）。NATS 异步把扩散搬到后台，发布 API 立刻返回。
- **inbox SortedSet 上限**：1000 条 + 7 天 TTL。超出由 Consumer 顺手 `ZREMRANGEBYRANK` 淘汰最旧。
- **大 V（粉丝 ≥ 2000）跳过**：见下一条 "读扩散" 链路。
- **游标分页**：用上一页最后一条的 `score` 当下一页起点，不用 OFFSET。新帖子插入不影响分页位置。

### 现状

[规划中] 阶段 7 实现。Pool 抽象（`PoolFeed`）已就绪。NATS 客户端 + Feed Consumer 都还没写。

→ 权威：[ADR-005](../adr/005-feed-push-pull-hybrid.md)、[docs/architecture.md](../architecture.md) "Feed 写扩散" 段

---

## 链路 C — Feed 读扩散（大 V 发文 / 粉丝读 Feed 时聚合）

### 业务流程白话版

1. 大 V Y（粉丝 100 万，≥ 2000 阈值）发了一篇博文
2. 平台**不**给所有粉丝写收件箱（写 100 万次会爆）
3. 改成：粉丝刷 Feed 时**临时去关注的所有大 V outbox 拉一波**，和自己 inbox 合并

### 后端怎么跑

```
大 V Y 点"发布"
    │
    ▼
content.Handler.Post (PG)
    │
    ▼
ZADD feed:outbox:{Y}
    │   只写自己 outbox，不扩散
    │
    ✗ 不发 fanout 消息

──── 粉丝 P 后来刷 Feed ──────

P ─GET /feed─┐
              ▼
      feed.Handler.List
              │
   ┌──────────┴──────────┐
   ▼                      ▼
 拉 inbox             拉关注大 V 的 outbox
 ZREVRANGEBYSCORE       1) 查 P 关注的大 V 列表
 feed:inbox:{P}            (与 feed:bigv:set 求交)
                         2) 对每个大 V K：
                            ZREVRANGEBYSCORE feed:outbox:{K}
   │                      │
   └──────────┬──────────┘
              ▼
       合并 + 按 ts 排序
              │
              ▼
       (可选) Wilson Score + 时间衰减重新排
              │
              ▼
       Top N + 下一页游标
```

### 关键技术点

- **大 V 写时不扩散**：避免"发一次写 100 万次 Redis"的灾难。
- **读时实时聚合**：略慢一点（需要拉多个 outbox）但只发生在用户主动刷 Feed 那一刻，不影响发布。
- **2000 阈值是可调超参**（`pkg/feed/config.go:PushThreshold`）：根据实际数据调整。
- **大 V 列表是查询缓存**（`feed:bigv:set`）：每个粉丝一次 `SINTER` 求交集就能拿到自己关注的大 V。

### 现状

[规划中] 阶段 7-8 实现。

→ 权威：[ADR-005](../adr/005-feed-push-pull-hybrid.md)、[docs/architecture.md](../architecture.md) "Feed 读扩散" 段

---

## 链路 D — AI 总结（博文发布 → 自动总结）

### 业务流程白话版

1. 任何作者发了一篇 > 500 字的博文
2. 平台后台自动调 AI 生成"摘要 + 关键词"，缓存起来
3. 读者点进博文页时，前端在标题下方显示 AI 摘要——不用读全文也能 get 大意

### 后端怎么跑

```
作者发博文
    │
    ▼
content.Handler.Post (PG)
    │
    ▼
发 NATS 消息 "ai.summarize.requested"  payload={postId}
    │
    ▼
AI Consumer 收到
    │
    ▼
查 token 预算（用户当日 / 全局令牌桶）
    │
    ▼
调 MCP Gateway: POST /mcp  body=tools/call summarize_article
    │
    ▼
Gateway 路由 → AI Summary MCP Server (gRPC)
    │
    ├─ 检查 PG 缓存（ai_summaries 表）：命中即返回
    │
    └─ 否则按降级链调 LLM：
         Ollama (本地) ─timeout─▶ Claude ─timeout─▶ OpenAI ─失败─▶ 返回首段截取
                │
                ▼
         结果回写 PG ai_summaries (Saga：本地写 + 失败补偿)
                │
                ▼
         同时回写 Redis Pool general 做读侧缓存
                │
                ▼
   读者点进博文 → content.Handler.Get
                  ├─ 命中 Redis 缓存 → 直接返回
                  └─ 否则查 PG ai_summaries → 回填 Redis → 返回
```

### 关键技术点

- **三层限流**（[ADR-006](../adr/006-mcp-gateway.md)）：Gateway 全局令牌桶 + per-user 滑动窗口 + per-tool 配额（`configs/mcp-tools.yaml`）。
- **多模型降级链**：本地 Ollama 优先（零成本）→ Claude 长文质量好 → OpenAI 兜底 → 全失败返回博文首段截取。保证用户**总能拿到结果**。
- **Token 预算**：用户每天有 token 上限，超出直接拒。避免被薅羊毛或失控成本。
- **PG ai_summaries 是权威源**，Redis 是读侧缓存：缓存失效不影响数据存在。
- **传输层**：Gateway 对外 HTTP + SSE（兼容 Claude Desktop 等标准 MCP Client）；对内 gRPC（编译期类型安全 + 双向流）。

### 现状

[规划中] 阶段 9-11。NATS 客户端 / MCP Gateway / AI Summary Server 都还没写。

→ 权威：[ADR-006](../adr/006-mcp-gateway.md)、[docs/architecture.md](../architecture.md) "AI 总结链路" 段

---

## 4 条链路对比

| 链路 | 触发 | 关键中间件 | 写入位置 | 同步 / 异步 | 当前阶段 |
|---|---|---|---|---|---|
| 拼团下单 | 用户开团 / 跟团 | Redis Lua + NATS 延迟 | MySQL orders + Redis stock | **同步**扣库存写订单 + **异步**超时处理 | [规划中，阶段 4] |
| Feed 写扩散 | 普通作者发文 | NATS fanout + Redis SortedSet | PG posts + Redis inbox/outbox | 发布同步 + 扩散异步 | [规划中，阶段 7] |
| Feed 读扩散 | 大 V 发文 / 粉丝读 Feed | Redis SINTER + ZREVRANGEBYSCORE | PG posts + Redis outbox | 写时同步 / 读时实时聚合 | [规划中，阶段 7-8] |
| AI 总结 | 博文发布触发 | NATS + MCP Gateway + LLM | PG ai_summaries + Redis cache | **完全异步**（用户不等） | [规划中，阶段 9-11] |

---

## 接下来读什么

- 想看技术栈选型表：[05 — 技术栈选型](05-tech-stack-rationale.md)
- 想跟一次具体请求走完：[06 — 一次请求的生命周期](06-how-it-runs.md)
- 想看每个决策为什么这么做：[03 — 设计决策摘要](03-design-decisions.md)

---

## 权威来源

- 整体数据流图 → [docs/architecture.md](../architecture.md) + `docs/pic/{groupbuy-flow,feed-push,feed-pull,ai-summary}/`
- 拼团 / 库存预扣 → [ADR-003](../adr/003-redis-unified-cache.md) "库存预扣方案" 段
- Feed 推拉 → [ADR-005](../adr/005-feed-push-pull-hybrid.md)
- 异步消息 → [ADR-004](../adr/004-nats-messaging.md)
- AI Gateway → [ADR-006](../adr/006-mcp-gateway.md)
- 阶段进度 → [docs/plan.md](../plan.md)
