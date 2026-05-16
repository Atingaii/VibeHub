# 05 — 技术栈选型

> 这个表分两块：
>
> **A. 已在 ADR / AGENTS.md 有依据的选型**——直接复述权威源 + 链接。
> **B. 仅作为现状清单**——目前选了，但权威依据还未沉到 ADR；本节只列事实，不当新事实源。
>
> 这样划分是为了避免 intro 层自创"为什么不选 X"的论据（违反 [R4 单一来源](../../AGENTS.md)）。Codex review 提醒：本文要么链回权威源，要么标"现状清单"，不能两边都做不全。
>
> 不熟的词请查 [00-glossary.md](00-glossary.md)。

---

## A. ADR / AGENTS.md 已覆盖的选型

| 类别 | 选 | 不选 / 备选 | 主理由（一句话） | 权威来源 |
|---|---|---|---|---|
| 架构形态 | Modular Monolith | 微服务 / 纯单体 | 1-2 人团队 + 业务初期 + 模块 10 个，靠模块化纪律保留拆服务可能 | [ADR-001](../adr/001-modular-monolith.md) |
| 主交易库 | MySQL 8.0+（InnoDB） | PostgreSQL / TiDB | 高并发写 TPS 比 PG 高 ~50%、复制延迟低 40%、运维生态成熟 | [ADR-002](../adr/002-dual-database.md) |
| 内容 / 元数据库 | PostgreSQL 15+ | MySQL / 混合 | JSONB + GIN 全文索引（中文配 zhparser）+ 窗口函数 + 数组类型 | [ADR-002](../adr/002-dual-database.md) |
| 缓存 / 近程存储 | Redis 7.x + Pool 抽象 | Memcached / Dragonfly / KeyDB | 多用途（缓存 + SortedSet + Lua + Lock + Stream）+ 生态稳定 | [ADR-003](../adr/003-redis-unified-cache.md) |
| Redis 实例策略 | 单实例 + 实例级 noeviction | per-DB 策略 / 多实例 | OSS 单实例 maxmemory-policy 是实例级，noeviction 防库存 / 锁静默丢 | [ADR-003](../adr/003-redis-unified-cache.md) v2 修订 |
| 消息队列 | NATS JetStream | RocketMQ / Kafka / DB 轮询 | 单二进制、持久化 + ACK + 延迟投递、当前规模够用 | [ADR-004](../adr/004-nats-messaging.md) |
| Feed 流策略 | 推拉结合（粉丝 2000 阈值） | 纯推 / 纯拉 / 算法推荐 | 大 V 写爆炸 vs 普通读慢的平衡 | [ADR-005](../adr/005-feed-push-pull-hybrid.md) |
| 排序算法 | Wilson Score + 时间衰减 | 纯赞数 / 纯时间倒序 | Wilson 置信下界对样本量公平，时间衰减保新鲜度 | [ADR-005](../adr/005-feed-push-pull-hybrid.md) |
| AI 接入协议 | MCP（Model Context Protocol） | 自定义 HTTP / OpenAI 私有协议 | 兼容 Claude Desktop / Cursor 等标准客户端 | [ADR-006](../adr/006-mcp-gateway.md) |
| MCP Gateway | 自研 + HTTP+SSE 对外 / gRPC 对内 | 开源 mcp-gateway / mcp-go 完整 SDK | Go 一致 + 业务限流深度集成 + 协议简单（< 1000 行） | [ADR-006](../adr/006-mcp-gateway.md) |
| LLM 路由 | 多模型降级链（Ollama → Claude → OpenAI） | 单供应商绑定 | 零成本优先 + 失败兜底 + 总有响应 | [ADR-006](../adr/006-mcp-gateway.md) |

---

## B. 现状清单（暂未沉到 ADR）

下面这些是**当前实际在用**的具体技术，但"为什么不是 X"的依据还没沉到 ADR。如果你要写论据级讨论，请等到对应 ADR / 架构附录补上后再展开——本节只列事实。

### B.1 后端语言与 HTTP 框架

| 项 | 当前选择 | 状态 |
|---|---|---|
| 语言 | Go 1.21+ | 已用（`go.mod`） |
| HTTP 框架 | Gin | 已用（`internal/server/router.go`，AGENTS.md 提及但未单独 ADR） |

> "为什么 Go 不是 Java / Node / Rust"、"为什么 Gin 不是 Echo / Fiber / Chi"——目前没有 ADR 论证。新加成员问起，参照行业常识答：Go 的 goroutine 模型适合后端 IO 密集 + 编译产物单二进制方便部署 + 团队熟悉度。但**不要把这条写进任何架构文档当事实**，那需要新 ADR。

### B.2 ORM 与数据访问

| 项 | 当前选择 | 状态 |
|---|---|---|
| ORM | GORM 2.x | 已用（`internal/database/`） |
| MySQL driver | `gorm.io/driver/mysql` | 已用 |
| PG driver | `gorm.io/driver/postgres`（pgx 底层） | 已用 |
| Redis client | `github.com/redis/go-redis/v9` | 已用（`internal/cache/redis.go`） |

> ADR-002 提到"统一使用 GORM 2.x"，但"为什么 GORM 不是 sqlx / ent / sqlc"未单独论证。

### B.3 配置 / 日志 / 中间件

| 项 | 当前选择 | 状态 |
|---|---|---|
| 配置加载 | Viper | 已用（`internal/config/`） |
| 日志库 | zap（uber-go/zap） | 已用（`internal/logger/`） |
| JWT 库 | [规划中] golang-jwt/jwt | 未实现 |
| 限流库 | [规划中] uber-go/ratelimit 或自建 | 未实现 |
| 分布式锁 | [规划中] go-redsync/redsync | 未实现 |

### B.4 消息 / 协议

| 项 | 当前选择 | 状态 |
|---|---|---|
| NATS client | `github.com/nats-io/nats.go` | [规划中] 已选型，未封装 |
| MCP transport | HTTP + SSE 自实现 | [规划中] |
| MCP 内部 RPC | gRPC（google.golang.org/grpc） | [规划中] |

### B.5 前端

| 项 | 当前选择 | 状态 |
|---|---|---|
| Web | Next.js | [规划中]，仓库内未实现 |
| 移动端 | Uni-app | [规划中]，仓库内未实现 |

> "为什么 Next.js 不是纯 React / SvelteKit / Nuxt"、"为什么 Uni-app 不是 React Native / Flutter"——目前没有 ADR。

### B.6 部署 / 运维

| 项 | 当前选择 | 状态 |
|---|---|---|
| 容器化 | Docker + Docker Compose | 已用（`deploy/docker/`） |
| 镜像构建 | 多阶段 Dockerfile（builder + runtime） | 已用（0.7） |
| 健康检查 | 应用 `GET /health` + Compose healthcheck | 已用（0.6） |
| 端口冲突处理 | `VIBESHOP_*_HOST_PORT` 环境变量参数化 | 已用（0.9） |
| 自启动 | `restart: unless-stopped` | 已用（0.9） |
| 可观测性（Tracing） | OpenTelemetry → Jaeger | [规划中] |
| 可观测性（Metrics） | Prometheus | [规划中] |

---

## 关于补 ADR

如果某个 B 节中的选项被反复挑战（典型：为什么 Go / 为什么 Gin / 为什么 GORM），就该新增 ADR——这是**沉到权威层**的方式，比在 intro 里写论据靠谱。流程见 AGENTS.md R1：

> **架构级**（模块边界、协议、数据模型、跨模块接口）→ 新增 `docs/adr/NNN-<slug>.md`

加 ADR 后回到本文 B 节，把对应行迁移到 A 节并补 ADR 链接。`docs/change-impact.md` 中的 "加 ADR" 同步规则会兜底提醒。

---

## 接下来读什么

- 想看一次请求怎么走：[06 — 一次请求的生命周期](06-how-it-runs.md)
- 想看决策的"为什么"：[03 — 设计决策摘要](03-design-decisions.md)

---

## 权威来源

A 节每行已链接到对应 ADR。其他相关源：
- 已实现技术的代码锚点 → [docs/code-map.md](../code-map.md)
- 当前阶段进度 → [README.md](../../README.md) "项目状态" + [docs/plan.md](../plan.md)
- ADR 写作模板 → [AGENTS.md R1](../../AGENTS.md)
