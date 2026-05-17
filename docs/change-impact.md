# 同步检查表详版（Change Impact）

本表列出所有"改 X 必须同时改 Y"的联动规则。`AGENTS.md` 中只放最高频 5 条精简版。

**新增规则只改这里，不要写回 AGENTS.md 的精简表。**

---

## 通用规则

| 改动 | 必须同时更新 |
|------|-------------|
| 加/改数据库表(MySQL) | `scripts/migration/mysql/` DDL → `internal/model/` struct → `internal/store/` DAO |
| 加/改数据库表(PG) | `scripts/migration/pg/` DDL → `internal/model/` struct → `internal/store/` DAO |
| 加/改 REST 端点 | `internal/server/router.go` + module handler + API 文档 |
| 加/改 Redis Key | `internal/cache/keys.go` pattern + 使用方 store/cache 文件 + 文档说明 |
| 加/改 Redis Pool 用途 | `internal/cache/keys.go:Pool` 加常量 + `pools` 表登记 DB/RequiresTTL/desc + `docs/adr/003-redis-unified-cache.md` Pool 表 |
| 加/改 MQ Topic | `internal/mq/topics.go` + producer + consumer + 幂等策略说明 |
| 加/改 MCP Tool | `internal/module/mcp/tools/` 实现 + `configs/mcp-tools.yaml` 路由 + `docs/api/mcp-tools.md` |
| 加/改环境变量 | `configs/*.yaml.example` 模板 + `deploy/docker/.env.example` + `deploy/docker/docker-compose*.yml` + `docs/dev-workflow.md` 环境变量参考表 + README |
| 新增模块 | `internal/module/<name>/` + `internal/server/router.go` 注册 + AGENTS.md 代码地图更新 |
| 加/改中间件 | `internal/middleware/` + `internal/server/router.go` 注册 + 文档 |
| 加/改配置字段 | `internal/config/` struct + `configs/*.yaml.example` 模板 + `docs/dev-workflow.md` 环境变量表 |

---

## 密钥安全规则

| 改动 | 必须同时更新 |
|------|-------------|
| 加/改中间件密码 | `deploy/docker/.env.example` 加变量名 + `docker-compose*.yml` 引用 `${VAR}` + `docs/dev-workflow.md` |
| 加/改配置中的密钥字段 | `configs/*.yaml.example` 模板中字段为空 `""` + 注释说明环境变量名 + `internal/config/load.go:bindEnvMappings` 绑定 |
| 加/改日志中的敏感字段 | `internal/middleware/logging.go:sensitiveQueryKeys` 添加脱敏字段名 + `docs/dev-workflow.md` 日志安全规范更新 |
| 加/改工单/安全整改记录 | `docs/private/` 本地文档更新 + 公开文档仅保留脱敏摘要；不要放入 `docs/features/` / README / AGENTS.md 的公开内容中 |

> **硬性要求**：
> - 仓库追踪文件（`git ls-files`）中**绝不允许**出现真实密码、token、API key
> - Docker Compose 中密码通过 `${VIBESHOP_*}` 变量引用，不写明文
> - `*.yaml.example` / `.env.example` 中密钥字段必须为空字符串 `""`
> - 数据库用户使用业务用户（如 `vibeshop`），不使用超级用户（`root`/`postgres`）
> - 工单号、扫描单号、内网规则名、整改过程记录默认视为私有信息，放 `docs/private/`，不进入 GitHub

---

## 模块级规则

### 用户模块

| 改动 | 必须同时更新 |
|------|-------------|
| 改鉴权逻辑 | `internal/middleware/auth.go` + 所有需要鉴权的 handler + 前端 token 处理 |
| 加用户标签类型 | `internal/model/user_tag.go` + store + 人群包圈选逻辑 |
| 改关注/粉丝 | `internal/module/user/` + Feed 写扩散阈值判断 |

### 商品与库存

| 改动 | 必须同时更新 |
|------|-------------|
| 改库存扣减逻辑 | `internal/cache/stock.lua` Lua 脚本 + store 落库 + MQ consumer + 幂等校验 |
| 加 SKU 属性 | model + store + 商品搜索索引 + 前端展示 |
| 改商品搜索 | PG 全文搜索配置 + store 查询 + API 返回 |

### 订单模块

| 改动 | 必须同时更新 |
|------|-------------|
| 改订单状态机 | `internal/model/order_status.go` + store + MQ consumer + 前端状态展示 |
| 改退款流程 | store + MQ consumer + 支付接口调用 + 通知推送 |
| 加订单类型 | model + store + 状态机分支 + 前端展示 |
| 改支付超时逻辑 | order timeout topic/consumer + 库存回滚 + 订单状态机 |

### 拼团系统

| 改动 | 必须同时更新 |
|------|-------------|
| 改成团条件 | `internal/module/groupbuy/state_machine.go` + Redis + 超时 consumer |
| 改拼团成团超时/失败逻辑 | groupbuy deadline topic/consumer + 退款策略 + 状态机 |
| 改拼团库存 | `internal/cache/stock.lua` + store + 幂等校验 |
| 加拼团类型 | model + activity store + 状态机分支 + 前端 |

### Feed 流

| 改动 | 必须同时更新 |
|------|-------------|
| 改推拉阈值 | `internal/module/feed/config.go:PushThreshold` + 文档 + 可能需要 rehash |
| 改排序算法 | `internal/module/feed/ranker.go` + 前端排序切换 UI + A/B 测试配置 |
| 改 inbox 结构 | `internal/module/feed/timeline.go` + NATS fanout consumer + 游标逻辑 |
| 改游标格式 | `internal/module/feed/cursor.go` + 前端分页参数 + API 文档 |

### MCP Gateway

| 改动 | 必须同时更新 |
|------|-------------|
| 加 MCP Tool | `internal/module/mcp/tools/` 实现 + `configs/mcp-tools.yaml` + 文档 |
| 改路由逻辑 | `internal/module/mcp/router.go` + 配置格式 + 文档 |
| 改限流策略 | `internal/module/mcp/ratelimit.go` + 配置 + Prometheus metrics |
| 改传输层 | SSE 实现 + 客户端兼容性 + 文档 |
| 改模型 fallback | `internal/module/mcp/model_router.go` + 配置 + 文档 |

### 优惠券

| 改动 | 必须同时更新 |
|------|-------------|
| 加券类型 | `internal/model/coupon_type.go` + store + 核销引擎分支 + 前端展示 |
| 改核销逻辑 | `internal/module/coupon/engine.go` + 幂等 key + 审计日志 |
| 改发放逻辑 | store + MQ(如异步发放) + 限领校验 |

### 抽奖

| 改动 | 必须同时更新 |
|------|-------------|
| 加抽奖玩法 | `internal/module/lottery/` 算法 + 前端动画组件 + 奖品管理接口 |
| 改概率算法 | `internal/module/lottery/algorithm.go` + 审计日志 + 测试覆盖 |
| 改防作弊 | middleware + 设备指纹(前端) + 风控规则 |

### 内容模块

| 改动 | 必须同时更新 |
|------|-------------|
| 改博文模型 | PG migration + model + store + 搜索索引 |
| 加互动类型 | model + store + 计数器缓存 + 前端 |
| 改评论结构 | model + store + 前端楼中楼展示 |

---

## 前端规则（后续阶段）

### Next.js (Web)

| 改动 | 必须同时更新 |
|------|-------------|
| 加页面路由 | `web/src/app/<route>/` + 导航菜单 + SEO meta |
| 改 API 调用 | `web/src/lib/api.ts` 类型定义 + 使用组件 |
| 加共享组件 | `web/src/components/` + stories(如有 Storybook) |

---

## 部署规则

| 改动 | 必须同时更新 |
|------|-------------|
| 改端口 | `configs/*.yaml.example` + Docker 端口映射 + 文档 |
| 改健康检查 | handler + Docker HEALTHCHECK + 文档 |
| 加环境依赖 | `deploy/docker/docker-compose.infra.yml` + `deploy/docker/.env.example` + `docs/dev-workflow.md` |
| 改 Dockerfile | `deploy/docker/Dockerfile` + `docker-compose.yml` 引用 |
| 加/改 Docker 密码变量 | `deploy/docker/.env.example` + `docker-compose*.yml` 中 `${VAR}` 引用 + 文档 |

---

## 文档规则

| 改动 | 必须同时更新 |
|------|-------------|
| 加 ADR | `docs/adr/NNN-<slug>.md` + AGENTS.md 核心架构决策列表（如果是顶级决策） + `docs/intro/03-design-decisions.md` 摘要段 |
| 改架构决策 | 对应 ADR + AGENTS.md 精简版 + `docs/intro/03-design-decisions.md` 状态/摘要 + 受影响的代码 |
| 加功能设计 | `docs/features/<slug>.md`（**必须含「面试官 Q&A」段，4–6 条 Q，trade-off 领型为主**——见 AGENTS.md R1）+ `docs/plan.md` 如需调整路线图 |
| 加/改主链路数据流 | `docs/architecture.md` + 对应 ADR + `docs/intro/04-feature-tour.md` |
| 加/改技术栈选型 | `docs/intro/05-tech-stack-rationale.md`（A/B 节判断） + 如需 ADR 论证则新增 ADR + `README.md` 技术栈段 |
| 加/改新概念术语 | `docs/intro/00-glossary.md` 加条目 + 首次出现处链接到术语表 |
| **完成一个 plan.md 子项（任意 X.Y 退出门达成）** | `README.md` 阶段进度勾掉 + `docs/plan.md` 行打勾 + `docs/intro/01-what-is-vibeshop.md` "当前状态" 段刷新（时间戳 + 子项状态） |
| **新业务端点端到端可用（不只是 stub）** | router 注册 + handler/service + `docs/intro/04-feature-tour.md` 增加链路条目（含可跑的 curl 示例与代码锚点） |
| 弃用设计 | 旧文档首行加 DEPRECATED 标记（R5） + 新文档链接 + `docs/intro/03-design-decisions.md` 状态更新 |

---

*最后更新：2026-05-17*
