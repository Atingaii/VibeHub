# 02 — 架构走读

> 三层架构白话版。权威详版见 [docs/architecture.md](../architecture.md)。
> 不熟的词请查 [00-glossary.md](00-glossary.md)。

---

## 一图概括

VibeShop 后端是个 **三层架构 + 模块化内部** 的 Go 单体：

```
┌────────────────────────────────────────────────────────────┐
│  客户端层                                                   │
│  ─────────                                                  │
│  - Web（Next.js）         [规划中]                           │
│  - 移动端（Uni-app）      [规划中]                           │
│  - 标准 MCP Client（Claude Desktop / Cursor 接 Gateway）     │
└──────────────────────────┬─────────────────────────────────┘
                           │ HTTP (REST + SSE)
┌──────────────────────────▼─────────────────────────────────┐
│  Gin HTTP Server（单二进制 / main.go）                       │
│  ────────────────────────────────────                        │
│  请求中间件：日志 / Recovery / 限流 / JWT                    │
│  路由：internal/server/router.go                             │
│                                                              │
│  业务模块（独立编译，接口解耦，模块间不直接 import）：         │
│  ┌──────────┬──────────┬──────────┬──────────┬───────────┐  │
│  │ user     │ product  │ order    │ groupbuy │ coupon    │  │
│  │ lottery  │ content  │ feed     │ ai       │ mcp       │  │
│  └──────────┴──────────┴──────────┴──────────┴───────────┘  │
│                                                              │
│  共享底座（cross-cutting）：                                 │
│  - internal/config（Viper 配置加载）                         │
│  - internal/logger（zap 结构化日志）                         │
│  - internal/middleware（HTTP 中间件）                        │
│  - internal/database（MySQL+PG 双连接池）                    │
│  - internal/cache（Redis Pool 抽象）                         │
│  - internal/mq    [规划中：NATS JetStream 封装]              │
└──────────────────────────┬─────────────────────────────────┘
                           │
┌──────────────────────────▼─────────────────────────────────┐
│  数据与基础设施层                                            │
│  ───────────────                                             │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐             │
│  │ MySQL 8.0+ │  │ PostgreSQL │  │ Redis 7.x  │             │
│  │ 交易主库   │  │ 15+ 内容库 │  │ 6 个语义   │             │
│  │            │  │            │  │ Pool       │             │
│  └────────────┘  └────────────┘  └────────────┘             │
│  ┌────────────┐  ┌────────────┐                              │
│  │ NATS       │  │ OSS        │                              │
│  │ JetStream  │  │ (local /   │                              │
│  │            │  │  aliyun)   │                              │
│  └────────────┘  └────────────┘                              │
└────────────────────────────────────────────────────────────┘
```

权威全景图见 [docs/architecture.md](../architecture.md) 和 `docs/pic/architecture/`。

---

## 三层各自负责什么

### 客户端层

不在仓库里。标准 HTTP REST + 一些 SSE（用于 MCP Tool 流式响应）。

注意 MCP Gateway 这一层故意支持**第三方标准 MCP 客户端**——用户可以拿 Claude Desktop 直接接入 VibeShop 提供的 AI Tools，这是项目的差异点之一。

### Gin HTTP Server（单进程）

#### 为什么是单体不是微服务

- 团队 1-2 人 / 模块 10 个 / 业务初期 → 微服务的"独立部署 / 服务发现 / RPC 框架"复杂度兜不住
- 单进程开发体验：一个 `go run .` 全部启动，断点 / log / 重构都直接
- "为拆服务做准备" 不等于"今天就拆"：靠模块化纪律保证未来可拆

详见 [ADR-001](../adr/001-modular-monolith.md)。

#### 模块边界纪律

```go
// ✅ 正确：通过接口解耦
type StockService interface {
    Deduct(ctx context.Context, skuID string, qty int) error
}

// ❌ 错误：直接 import 另一个模块的内部实现
import "internal/module/product/stock"
```

每个 `internal/module/<name>/` 暴露 `interface.go` 给别的模块用，内部实现永远不被外部 import。这条规则是 [ADR-001](../adr/001-modular-monolith.md) 的核心契约。

#### 共享底座（cross-cutting）

跨模块复用的能力放在 `internal/` 下的非 module 子包：

| 子包 | 干嘛 | 现状 |
|---|---|---|
| `internal/config/` | Viper 加载 yaml + 环境变量覆盖 | 已实现（0.2） |
| `internal/logger/` | zap 结构化日志 + 请求日志中间件 | 已实现（0.3） |
| `internal/middleware/` | Recovery / 限流 / JWT / 请求日志 | 部分（0.3） |
| `internal/database/` | MySQL+PG 双连接池 + 启动 Ping + 重试 | 已实现（0.4） |
| `internal/cache/` | Redis Pool 抽象 + Close 哨兵 | 已实现（0.5 + ADR-003 修订） |
| `internal/mq/` | NATS JetStream 封装 | [规划中] |
| `internal/server/` | Gin engine + router 注册 + Server 装配 | 已实现（0.1） |

### 数据与基础设施层

#### 为什么 MySQL + PostgreSQL 双库

| 数据特性 | 主库 | 选择理由 |
|---|---|---|
| 高并发写、强一致、行级锁（订单 / 库存 / 拼团） | **MySQL** | InnoDB 比 PG TPS 高 ~50%、复制延迟低 40%、运维生态成熟 |
| 复杂查询、全文搜索、JSON 嵌套（博文 / 评论 / Feed 元数据 / AI 总结） | **PostgreSQL** | JSONB + GIN 索引 + 中文分词 + 窗口函数 |

跨库不上 XA 分布式事务（贵且慢），用 **Saga 模式**：本地事务 + 失败补偿 + 消息编排。
详见 [ADR-002](../adr/002-dual-database.md)。

#### 为什么 Redis 不只做缓存

VibeShop 把 Redis 当 **统一近程存储**，覆盖六类用途：

| Pool | DB | 用途 | TTL 约束 |
|---|---|---|---|
| `general` | 0 | 通用缓存（商品 / 用户） | 必须 TTL |
| `feed` | 1 | Feed 时间线 SortedSet | 不带 TTL |
| `stock` | 2 | 库存 / 锁 / 计数 | 不带 TTL |
| `session` | 3 | 会话 / Token | 必须 TTL |
| `notify` | 4 | 消息 / 通知 | 必须 TTL |
| `rank` | 5 | 排行榜 | 不带 TTL |

业务代码不直接写 `client.SELECT 2`，而是 `RedisManager.Pool(PoolStock)`。这是 ADR-003 修订时新增的 Pool 抽象——OSS 单实例的 maxmemory-policy 是实例级，per-DB 配淘汰策略其实做不到，所以实例级统一 `noeviction`，pool 元数据是阶段一约束。

详见 [ADR-003](../adr/003-redis-unified-cache.md) 与 `internal/cache/keys.go:Pool`。

#### NATS JetStream 而不是 Kafka / RocketMQ

异步任务（Feed 写扩散 / 订单超时 / AI 总结触发 / 库存回滚）的中间件需求：
- 轻量（单二进制部署）
- 持久化 + ACK + 延迟投递
- 不需要事务消息或精确顺序消费（拼团暂时不需要）

JetStream 全部满足，且 Docker 一行起。
详见 [ADR-004](../adr/004-nats-messaging.md)。

---

## 模块间通信规则

VibeShop 的"内部纪律"——任何 PR 违反这条都该回退：

1. **同进程内**：模块间通过 Go interface 调用，不直接 import 具体实现
2. **异步解耦**：能容忍延迟的操作走 NATS 消息（典型例子：发博文 → NATS → Feed 写扩散）
3. **跨模块共享数据**：禁止直接读对方表，必须经对方接口（典型例子：订单模块要查商品库存，调 product 模块的 `StockService.Get(skuID)`，不直接 SELECT inventory）

详见 [docs/architecture.md](../architecture.md) "模块间通信规则" 段。

---

## 部署形态

```
Docker Compose
├─ vibeshop（应用容器，多阶段 Dockerfile 编译）
├─ mysql      （3306 → ${VIBESHOP_MYSQL_HOST_PORT:-3306}）
├─ postgres   （5432 → ${VIBESHOP_PG_HOST_PORT:-5432}）
├─ redis      （6379 → ${VIBESHOP_REDIS_HOST_PORT:-6379}）
└─ nats       （4222 / 8222 → ${VIBESHOP_NATS_*_HOST_PORT}）
```

主机端口全部参数化（解决本机已有 MySQL / Redis 占端口的常见痛点），密钥走 `deploy/docker/.env`（在 .gitignore，不入仓库）。
详见 [docs/dev-workflow.md](../dev-workflow.md) 与 [0.9 阶段说明](../features/0.9-host-port-parameterize-and-autostart.md)。

---

## 接下来读什么

- 想懂每个决策的"为什么不选 X"：[03 — 设计决策摘要](03-design-decisions.md)
- 想看链路跑起来：[04 — 主链路走读](04-feature-tour.md)
- 想跟着一次请求走一遍：[06 — 一次请求的生命周期](06-how-it-runs.md)

---

## 权威来源

- 模块边界 / 数据流 / 通信规则 → [docs/architecture.md](../architecture.md)
- 单体决策 → [ADR-001](../adr/001-modular-monolith.md)
- 双数据库决策 → [ADR-002](../adr/002-dual-database.md)
- Redis Pool 抽象 → [ADR-003](../adr/003-redis-unified-cache.md) 与 `internal/cache/keys.go:Pool`
- NATS 决策 → [ADR-004](../adr/004-nats-messaging.md)
- 端口参数化 → [docs/features/0.9-host-port-parameterize-and-autostart.md](../features/0.9-host-port-parameterize-and-autostart.md)
- "我想改 X 动哪里" → [docs/code-map.md](../code-map.md)
