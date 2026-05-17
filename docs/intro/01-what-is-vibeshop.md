# 01 — VibeShop 是什么

> 一句话定位：**社交电商 + AI 内容平台**。集拼团购物、博文 Feed 流、AI MCP Gateway 于一体的 Go 单体后端。

不熟的词请查 [00-glossary.md](00-glossary.md)。

---

## 核心场景

VibeShop 把三种通常分开做的产品**合在同一个后端进程里**：

### 场景 A — 拼团电商（像拼多多的开团 / 跟团 / 自动成团）

用户能开团、邀请朋友凑人数、到点自动成团或退款。
- 涉及模块：商品（`internal/module/product/`）、订单（`order/`）、拼团（`groupbuy/`）、库存预扣（Redis Pool `stock`）
- 关键技术：Redis Lua 原子脚本扣库存防超卖、NATS JetStream 分别处理订单支付超时与拼团成团超时
- 当前状态：[规划中]

### 场景 B — 内容社区（像知乎首页的 Feed 流）

用户能发博文、关注作者、刷信息流（关注 + 推荐两种 tab）。
- 涉及模块：内容（`internal/module/content/`，PG）、关注关系（PG）、Feed（`feed/`，Redis Pool `feed`）
- 关键技术：**推拉结合** Feed（粉丝数 < 2000 走 push，>= 2000 走 pull）、Wilson Score + 时间衰减热度排序、游标分页
- 当前状态：[规划中]

### 场景 C — AI 能力 Gateway（MCP 协议）

平台内置文章总结、智能搜索、推荐这类 AI 工具，对外按 MCP 协议暴露——意味着用户可以拿 Claude Desktop / Cursor 这类标准 MCP 客户端直接接入。
- 涉及模块：AI（`internal/module/ai/`）、MCP（`mcp/`）
- 关键技术：自研 MCP Gateway（HTTP + SSE 对外，gRPC 对内），多模型降级链（Ollama → Claude → OpenAI），三层限流（全局 / per-user / per-tool）
- 当前状态：[规划中]

---

## 为什么是"这三个组合"

电商 + 内容 + AI 在国内不是新组合（小红书、抖音都在做），但通常每家做一个。VibeShop 把三个放一起的逻辑：

1. **业务上闭环**：内容（博文）是引流 → 拼团是变现 → AI 是辅助决策（自动总结博文 / 推荐商品 / 客服）。三者互相滋养。
2. **技术上验证完整后端能力**：交易侧（强一致 / 高并发写）+ 内容侧（复杂查询 / 全文搜索）+ AI 侧（流式 / 限流 / 多模型）三类完全不同的工程挑战放在同一个仓库，是给作为简历项目的最大价值。
3. **"做减法"的代价**：业务范围比单一拼团 app 大 2-3 倍，所以用 [Modular Monolith](00-glossary.md#modular-monolith) 控制复杂度——单进程开发、模块化代码、未来按需拆服务。

---

## 不做什么

避免简历层面被误解为"什么都做了"，明确边界：

- **不做支付通道**：项目接的是支付网关接口（沙箱），不实现银行级风控
- **不做 IM 即时通讯**：博文有评论但没有"私信"
- **不做直播 / 视频流**：内容只到博文 + 图片
- **不做推荐算法的深度学习模型**：Wilson Score + 时间衰减是工程级算法，不训练模型
- **MCP Gateway 自研但不与官方 SDK 完全对齐**：只支持 transport + JSON-RPC + Tool 路由，不实现完整 Resources / Prompts 子集

---

## 当前状态（截至 2026-05-17）

阶段 0 —— **基础设施已完成**：
- HTTP 服务（Gin + 健康检查）
- 双数据库连接管理（GORM + 连接池 + 启动验证 + 断连重试）
- Redis 连接 + Pool 抽象（6 个语义分组，Close 后哨兵 client 防 nil-map panic）
- NATS 客户端 [规划中，已选型]
- Docker Compose 编排（应用 + 中间件全栈一键启动）
- 配置加载（Viper + 环境变量优先级）
- 结构化日志（zap + 请求日志中间件 + 敏感字段脱敏）
- 多 AI 协作框架（CCB：Claude designer + Codex reviewer 模式，每个 commit 经 plan + code 双轮 review）

阶段 1 —— **用户体系（进行中）**：
- 1.1 用户注册 ✅ —— `POST /api/v1/auth/register`，username/phone/email 三选一，bcrypt cost=10，CHECK 约束 DB 兜底
- 1.2 用户登录 ✅ —— `POST /api/v1/auth/{login,refresh,logout}`，HS256 JWT（access 2h + refresh 7d），refresh 哈希存 Redis PoolSession + Lua 原子轮换防并发，logout 幂等
- 1.3 JWT 鉴权中间件 [下一步]
- 1.4 资料、1.5 标签、1.6 关注 [规划中]

同时 1.1 阶段引入了 **goose 数据库迁移**（库模式 + embed.FS + DBProvider 接口按需开库），作为后续所有阶段的 schema 演进基础设施。

阶段 2-12 —— **业务模块规划中**：
- 阶段 2：商品 + 订单基础
- 阶段 3-4：购物车 + 拼团核心
- 阶段 5-6：内容（博文 CRUD + 评论）+ 关注关系
- 阶段 7-8：Feed 推拉 + 排序
- 阶段 9：AI 总结（MCP Server）
- 阶段 10-11：MCP Gateway + 多模型路由
- 阶段 12：可观测性（Tracing / Metrics / Alert）

详细阶段拆解见 [docs/plan.md](../plan.md)。

---

## 接下来读什么

- 想懂"模块边界 / 数据流"：[02 — 架构走读](02-architecture-walkthrough.md)
- 想懂"为什么选这套技术 / 决策"：[03 — 设计决策摘要](03-design-decisions.md)
- 想看场景跑起来是什么样：[04 — 主链路走读](04-feature-tour.md)
- 想看一次请求怎么走：[06 — 一次请求的生命周期](06-how-it-runs.md)

---

## 权威来源

本文叙述性内容的权威源：

- 模块边界与数据流 → [docs/architecture.md](../architecture.md)
- 阶段路线图与退出门 → [docs/plan.md](../plan.md)
- 当前状态清单 → [README.md](../../README.md) "项目状态" 段
- 单体 vs 模块化 → [ADR-001](../adr/001-modular-monolith.md)
