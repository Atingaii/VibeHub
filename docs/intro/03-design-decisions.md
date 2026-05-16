# 03 — 设计决策摘要

> 6 个核心决策的"是什么 / 为什么这么选 / 不选什么 / 现状 / 推翻条件"。每条结尾指向权威 ADR——本页是导读，不是事实源。
>
> 不熟的词请查 [00-glossary.md](00-glossary.md)。

---

## 决策 1：单体优先 + 模块化（Modular Monolith）

**是什么**：所有业务模块编译进同一个 Go 二进制（单进程），但内部按业务严格分包（`internal/module/{user,product,order,...}/`），模块之间只通过 Go 接口通信。

**为什么这么选**：
- 团队 1-2 人 / 业务初期 / 模块 10 个 → 微服务的"独立部署 / 服务发现 / RPC / 多进程调试"复杂度兜不住
- 单进程开发体验好：本地一个 `go run .` 全部启动，调试 / 重构方便
- 模块化纪律保留"未来按需拆服务"的可能——把一个 module 连接口实现一起搬到独立服务，对外接口不变

**不选什么 / 为什么**：
- **纯单体（不模块化）**：单一 main.go 里堆所有逻辑，初期最快但 6 个月后必腐烂
- **微服务（一开始就拆）**：10 个进程的运维成本对小团队是灾难
- **DDD 严格领域建模 + Saga 全域事件溯源**：复杂度兜不住，过度设计

**现状**：✓ 已落地。`main.go` 入口 / `internal/server/router.go` 注册 / `internal/module/` 目录骨架已建（业务模块内容 [规划中]）。

**推翻条件**（来自 ADR-001）：
- 单一模块需要独立扩缩容（如秒杀时库存服务需要 10x 实例）
- 团队 > 5 人需要独立交付节奏
- 单进程内存 / CPU 触顶

→ 权威：[ADR-001](../adr/001-modular-monolith.md)

---

## 决策 2：MySQL + PostgreSQL 双数据库

**是什么**：交易数据（用户 / 订单 / 库存 / 拼团 / 优惠券）→ MySQL；内容数据（博文 / 评论 / 标签 / Feed 元数据 / AI 总结）→ PostgreSQL。跨库一致性走 Saga（本地事务 + 失败补偿 + NATS 消息编排）。

**为什么这么选**：
- 两类数据特性截然不同：交易要 InnoDB 行级锁 + 高并发写 + 强一致；内容要 JSONB + GIN 全文索引 + 复杂查询
- 单库通吃必有一头吃亏：纯 MySQL 全文搜索弱、JSON 查询慢；纯 PG 高并发 TPS 比 MySQL 低
- Docker Compose 让本地两套数据库零成本；生产用托管 RDS 也运维差异不大

**不选什么 / 为什么**：
- **纯 MySQL**：内容侧的标签 / 全文搜索 / 推荐排序就要自己造轮子或上 ES
- **纯 PostgreSQL**：高并发订单写场景 TPS 偏低，秒杀 / 拼团成团时压力会先压垮它
- **TiDB**：HTAP 听起来美好，但运维门槛高、单集群成本对 1-2 人团队不合算
- **跨库 XA 分布式事务**：性能差 + 复杂度爆炸，Saga 是工业界主流妥协

**现状**：✓ 双连接池已建（`internal/database/`，0.4 阶段），启动时双 Ping 验证 + 断连重试。具体 DAO 实现 [规划中]。

**推翻条件**（来自 ADR-002）：
- 项目规模始终 < 5 万用户 → 退回纯 PG（简化架构）
- 全文搜索需求超出 PG 能力 → 引入 Elasticsearch / Meilisearch
- 跨库强一致场景增多 → 评估 TiDB

→ 权威：[ADR-002](../adr/002-dual-database.md)

---

## 决策 3：Redis 统一缓存 + Pool 抽象（含 ADR-003 的诚实修订）
<a id="decision-redis-pool"></a>

**是什么**：Redis 7.x 单实例承担六个用途——通用缓存（商品 / 用户）、Feed 时间线、库存 / 锁 / 计数、会话、消息通知、排行榜。每个用途对应一个语义 Pool（`internal/cache/keys.go:Pool`），Pool 在单实例阶段映射到不同 DB（0..5），未来可独立拆物理实例。

**为什么这么选**：
- Redis 是后端"贴身近的高速存储"：缓存、SortedSet（Feed）、Lua 原子脚本（库存）、SETNX（锁）、List/Stream（消息）一物多用
- 业务代码不直接 `SELECT 2`：Pool 抽象给业务统一入口（`RedisManager.Pool(PoolStock)`），未来要拆多实例时业务代码不动
- 返回 `redis.UniversalClient` 接口：为后续把某个 Pool 升级到 ClusterClient / Sentinel 留口子

**关于 OSS 的诚实修订**（ADR-003 v2 的关键变化）：
- ADR-003 早期版本写"DB1 noeviction，DB3 volatile-ttl"——这是错的
- 事实：OSS Redis 单实例的 `maxmemory-policy` 是**实例级**配置，16 个逻辑 DB 共享同一份内存池和淘汰策略，per-DB 配淘汰策略只有 Redis Enterprise 或拆物理实例才能做到
- 当前实例级真实配置：`--maxmemory-policy noeviction`（写满直接报错而非静默淘汰，避免库存 / 锁丢数据）
- Pool 表里的 "TTL 约束" 和 "语义期望淘汰" 是**阶段一元数据**，不在写路径强校验

**不选什么 / 为什么**：
- **Memcached**：只有 KV，没有 SortedSet / Lua，喂不饱 Feed 流和库存预扣
- **Dragonfly / KeyDB**：性能高但生态新，Go SDK 验证少，初期稳定 > 极致性能
- **MongoDB / 自建 KV**：用 Redis 做缓存 + 队列 + 锁本就是行业事实标准，没有切换的理由
- **每用途一个独立 Redis 实例**：当前数据量没到位，今天拆是过度投资；通过 Pool 抽象保留迁移口子即可

**现状**：✓ 已落地。`internal/cache/keys.go` 定义 6 个 Pool，`internal/cache/redis.go:RedisManager` 按 Pool 懒加载 client，加 `sync.RWMutex` 防并发竞争 + Close 后哨兵 client 防 nil-map panic（race 测试通过）。

**Close 后行为细节**：Close 后再调 `Pool/Client/Ping` 不返回 nil 也不 panic——返回一个已关闭的哨兵 client，命令上由 go-redis 报 `redis: client is closed`，调用方走错误路径而不是崩溃。

**推翻条件**（来自 ADR-003）：
- Feed 流用户量 > 1000 万且 SortedSet 内存爆炸 → 时间线下沉到 Cassandra / ScyllaDB
- 单实例 600 连接成为瓶颈 → 重新设计 RedisManager 内部复用策略（拆 Pool 到独立 server 等）

→ 权威：[ADR-003](../adr/003-redis-unified-cache.md) 与 `internal/cache/keys.go:Pool`、`internal/cache/redis.go:RedisManager`

---

## 决策 4：NATS JetStream 作为统一消息队列

**是什么**：异步任务（Feed 写扩散 / 订单支付超时 / 拼团成团 deadline / AI 总结触发 / 库存回滚）走 NATS JetStream（NATS 的持久化扩展：消息落盘 + ACK 确认 + 延迟投递）。

**为什么这么选**：
- 单二进制部署，Docker 一行起，运维 ~ 0
- 持久化 Stream + Consumer ACK：消息不丢
- 延迟投递通过 Header + 定时 re-deliver 实现，既能覆盖订单 30 分钟未支付自动取消，也能覆盖拼团按活动时限（示例 24 小时）未成团自动退款
- 订单支付超时与拼团成团超时必须拆成独立 topic / consumer；前者处理未支付关单，后者处理已支付未成团退款
- 当前业务**不需要**事务消息（消息和 DB 操作原子绑定），也不需要精确顺序消费

**不选什么 / 为什么**：
- **RocketMQ / Kafka**：日消息量 < 100 万 / 天的项目用它们是大材小用，运维 / 部署成本高
- **Redis Pub/Sub**：不持久化，消息一发即逝
- **数据库表轮询**：性能差、延迟高、消费者扩展困难

**现状**：[规划中] 客户端封装。NATS 容器在 docker-compose 已起，配置加载已含 `messaging.nats.url`。

**推翻条件**（来自 ADR-004）：
- 需要事务消息（消息和 DB 操作原子绑定） → 上 RocketMQ
- 日消息量 > 100 万 / 天且需要精确顺序 → 评估 Kafka

→ 权威：[ADR-004](../adr/004-nats-messaging.md)

---

## 决策 5：Feed 推拉结合（粉丝 2000 阈值）

**是什么**：作者发博文时按粉丝数分流：
- 粉丝 < 2000 → **推（Push）**：异步把帖子塞到所有粉丝的 Redis SortedSet inbox
- 粉丝 ≥ 2000 → **不推**：粉丝读 Feed 时实时去关注的大 V outbox 拉合并

**为什么这么选**：
- **纯推**：大 V 100 万粉丝 → 一次发文要写 100 万次 Redis，灾难
- **纯拉**：每次读 Feed 都要聚合关注的所有人 outbox，慢
- 推拉结合：99% 普通用户走推（读快），1% 大 V 走拉（写不爆炸）

**核心数据结构**：
- `feed:inbox:{userId}` SortedSet（推产物，TTL 7 天，Max 1000 条）
- `feed:outbox:{authorId}` SortedSet（所有人都有，TTL 30 天，Max 500 条）
- `feed:bigv:set` Set（大 V 名单）

**游标分页（不用 OFFSET）**：
传统 OFFSET LIMIT 在新数据插入时会"翻页看到重复"。改成"上一页最后一条 score 是 X，拉 X 之前的 N 条"，新数据不影响分页位置。

**热度排序（可选切换）**：
Wilson Score（赞踩比的置信下界）+ 时间衰减（半衰期 24 小时）。比"赞数 / 总数"公平，比"按时间倒序"有信息量。

**不选什么 / 为什么**：
- **纯算法推荐**：项目阶段还没到训练模型的位置；Wilson Score + 时间衰减是工程级算法，足够初期用
- **MongoDB 存 Feed**：Redis SortedSet 性能压它一头，且本就有 Redis
- **Cassandra 时间线**：未来用户量爆炸时的 fallback，今天上属于过度投资

**现状**：[规划中] 阶段 7-8 实现。

**推翻条件**（来自 ADR-005）：
- 用户量 < 1 万 → 纯推模式即可（简单直接）
- 算法推荐成为主要分发手段 → Feed 退化为推荐列表（不依赖关注关系）

→ 权威：[ADR-005](../adr/005-feed-push-pull-hybrid.md)

---

## 决策 6：自研 MCP Gateway

**是什么**：项目把所有 AI 能力（文章总结 / 智能搜索 / 推荐）按 [MCP 协议](00-glossary.md#mcp) 暴露——意味着用户可以拿 Claude Desktop / Cursor 这类标准 MCP 客户端直接接入 VibeShop。

**架构**：
```
外部客户端 ── HTTP+SSE+Bearer ──▶ MCP Gateway ── gRPC ──▶ MCP Server 池
                                  │
                                  ├ 认证（JWT/API Key）
                                  ├ 限流（全局 / per-user / per-tool 三层）
                                  └ Tool 路由（按 tool name 转发）
```

**为什么这么选**：
- 自研而非用开源 mcp-gateway：开源大多 Node.js / Python，与 Go 后端异构成本高；且需要深度集成业务限流（per-user token 预算）
- 自研而非完整引上游 Go SDK：`mcp-go` 等封装过重且更新与协议不同步；只要 transport + JSON-RPC + Tool 路由，自实现 < 1000 行
- HTTP+SSE 对外（兼容标准 MCP Client）+ gRPC 对内（编译期类型安全 + 高性能）：内外解耦的标准做法
- **多模型降级链**：Ollama（本地零成本）→ Claude（高质量长文）→ OpenAI（性价比通用）→ 全失败时返回博文摘要第一段。保证总有响应

**不选什么 / 为什么**：
- **直接调 OpenAI API**：单一供应商绑定 + 没有限流计费 / 多租户 / fallback
- **完整 mcp-go SDK**：协议本身简单，自实现可控
- **服务网格（Istio）做路由**：当前 MCP Server 数量 < 10，杀鸡用牛刀

**现状**：[规划中] 阶段 9-11。

**推翻条件**（来自 ADR-006）：
- AI 功能极简（只有文章总结） → 退回直接调 OpenAI API
- 需要对接 > 50 个 MCP Server → 考虑服务网格替代自研路由

→ 权威：[ADR-006](../adr/006-mcp-gateway.md)

---

## 决策大图（速查表）

| # | 决策 | 选 | 不选 | 主理由 | ADR | 现状 |
|---|---|---|---|---|---|---|
| 1 | 架构形态 | Modular Monolith | 微服务 / 纯单体 | 1-2 人团队 + 业务初期 + 模块 10 个 | [001](../adr/001-modular-monolith.md) | ✓ |
| 2 | 数据库 | MySQL + PG 双库 | 纯 MySQL / 纯 PG / TiDB | 交易 vs 内容数据特性差异大 | [002](../adr/002-dual-database.md) | ✓ |
| 3 | Redis 用途 | 统一缓存 + Pool 抽象 + 实例级 noeviction | 多 Redis / Memcached / per-DB 策略 | OSS 实例级限制 + 库存不能静默丢 | [003](../adr/003-redis-unified-cache.md) | ✓ |
| 4 | 消息队列 | NATS JetStream | RocketMQ / Kafka / DB 轮询 | 当前规模够用 + 单二进制运维零 | [004](../adr/004-nats-messaging.md) | [规划中] |
| 5 | Feed 流 | 推拉结合（粉丝 2000 阈值）+ Wilson 热度 | 纯推 / 纯拉 / 算法推荐 | 大 V 写爆炸 vs 普通读慢的平衡 | [005](../adr/005-feed-push-pull-hybrid.md) | [规划中] |
| 6 | AI 接入 | 自研 MCP Gateway + 多模型降级 | 直调 OpenAI / mcp-go 完整 SDK / Istio | 标准协议兼容 + 业务限流深度集成 | [006](../adr/006-mcp-gateway.md) | [规划中] |

---

## 接下来读什么

- 想看决策跑起来：[04 — 主链路走读](04-feature-tour.md)
- 想看技术栈选型表：[05 — 技术栈选型](05-tech-stack-rationale.md)
- 想看一次请求怎么走：[06 — 一次请求的生命周期](06-how-it-runs.md)

---

## 权威来源

每条决策的权威 ADR 已在条目内标注。其他相关源：
- 当前阶段进度 → [README.md](../../README.md) "项目状态" 段
- 模块边界 → [docs/architecture.md](../architecture.md)
- 阶段拆解 → [docs/plan.md](../plan.md)
- "改 X 动哪" → [docs/code-map.md](../code-map.md)
