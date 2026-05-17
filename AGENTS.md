# AGENTS.md — VibeShop 会话简报

这是 AI 编程会话的入口简报。读完这份文档你应该能直接上手设计方案和改代码。详细信息在 `docs/` 下按需加载。

## 一句话定位

**社交电商 + AI 内容平台**：集拼团购物（商品/优惠券/人群标签/抽奖）、博文 Feed 流（类知乎发文/关注/推荐 + AI 总结）、AI MCP Gateway（统一 HTTP/RPC 调度多模型/多工具）于一体的综合平台。Go 单体后端，模块化设计，未来按需拆服务。

## 核心架构决策（记住这 7 条）

1. **单体优先 + 模块化内部（Modular Monolith）**：单个 `main.go` 入口，内部按业务域拆包（`internal/module/`）。`go run .` 一把起全栈，后续需要时再按模块拆微服务。不过早引入服务间通信的复杂度。见 [ADR-001](docs/adr/001-modular-monolith.md)。

2. **Gin Web 框架**：统一使用 Gin 做 HTTP 层（生态好、上手快、45K+ req/s）。路由按模块分组注册。不混用多框架。见 [ADR-001](docs/adr/001-modular-monolith.md)。

3. **MySQL 8.0+ 主库 + PostgreSQL 辅库**：交易型数据（用户/订单/库存/支付）走 MySQL；内容型数据（博文/评论/推荐）走 PostgreSQL（JSONB + 全文搜索）。ORM 用 GORM。见 [ADR-002](docs/adr/002-dual-database.md)。

4. **Redis 统一缓存 + Feed 流存储**：Redis 7.x 承担缓存 + Feed 时间线（SortedSet）+ 库存预扣（Lua 原子脚本）+ 分布式锁。见 [ADR-003](docs/adr/003-redis-unified-cache.md)。

5. **NATS 轻量消息**：实时推送/Feed 更新/AI 触发/异步任务统一走 NATS JetStream（单二进制、毫秒延迟）。初期不上 RocketMQ/Kafka，用 NATS 的持久化 + 消息确认满足需求。见 [ADR-004](docs/adr/004-nats-messaging.md)。

6. **推拉结合 Feed 流**：粉丝 < 2000 的作者用推模式（写扩散到 Redis SortedSet），大 V 用拉模式（读时聚合）。热度排序用 Wilson Score + 时间衰减。见 [ADR-005](docs/adr/005-feed-push-pull-hybrid.md)。

7. **MCP Gateway 统一 AI 调度**：自研 MCP Gateway（HTTP+SSE 对外），统一管理多个 AI 能力（总结/搜索/推荐）。支持工具路由、限流、多模型切换。见 [ADR-006](docs/adr/006-mcp-gateway.md)。

## 领域黑话速查

| 词 | 含义 |
|---|---|
| **拼团活动** | 定义拼团价/成团人数/时限的活动配置 |
| **开团** | 用户发起拼团，创建拼团订单 |
| **参团** | 其他用户加入已存在的拼团 |
| **成团** | 参团人数达标，触发订单履行 |
| **Feed** | 用户首页内容流（关注 + 推荐） |
| **写扩散** | Push：发布时写到所有粉丝 inbox |
| **读扩散** | Pull：阅读时实时聚合 |
| **MCP** | Model Context Protocol（AI 工具/资源调用协议） |
| **Tool** | MCP 中 AI 可调用的函数 |
| **SKU** | 最小库存单元 |
| **券** | 优惠券（满减/折扣/免邮） |
| **人群包** | 标签圈选后的用户集合 |

## 目录结构规划（尚未实现，仅供参考）

```
main.go                     唯一入口：加载配置、初始化各模块、启动 HTTP server
internal/
  config/                   配置加载（Viper + 环境变量覆盖）
  server/                   HTTP server + 路由注册
  middleware/               中间件（JWT鉴权/限流/日志脱敏/Trace）
  module/
    user/                   用户模块（注册/登录/资料/标签）
    product/                商品模块（SPU/SKU/分类/搜索）
    order/                  订单模块（下单/支付/退款）
    groupbuy/               拼团模块（开团/参团/成团状态机）
    coupon/                 优惠券模块（发放/领取/核销）
    lottery/                抽奖模块（活动/算法/奖品/防刷）
    content/                内容模块（博文CRUD/Markdown/标签）
    feed/                   Feed流模块（推拉/排序/分页）
    ai/                     AI模块（总结/关键词/标题生成）
    mcp/                    MCP Gateway模块（路由/协议/SSE）
  model/                    数据模型定义（各表 struct）
  store/                    数据访问层（各模块 DAO）
  cache/                    Redis 缓存层
  mq/                       消息队列抽象
configs/
  dev.yaml.example          开发配置模板（可提交，不含密钥）
  docker.yaml.example       Docker 配置模板（可提交，不含密钥）
  dev.yaml                  开发配置（.gitignore，密钥通过环境变量注入）
  docker.yaml               Docker 配置（.gitignore，同上）
  prod.yaml                 生产配置（.gitignore）
docs/
  adr/                      架构决策记录
  features/                 功能设计文档（做之前先写这里）
  private/                  本地私有文档（工单/安全扫描/整改记录，.gitignore，不提交 GitHub）
  plan.md                   功能实现计划书
deploy/
  docker/
    Dockerfile              应用多阶段构建（编译→最小Alpine镜像）
    docker-compose.yml      全栈启动（应用+中间件，make docker-up）
    docker-compose.infra.yml 仅中间件（开发用，make infra-up）
    .env.example            Docker 环境变量模板（可提交，密钥全空）
    .env                    Docker 环境变量（.gitignore，含真实密钥）
web/                        前端（Next.js，后做）
```

## 启动方式速查

> **首次 clone 必做**：`cp deploy/docker/.env.example deploy/docker/.env` + 填入密码。详见 [docs/dev-workflow.md](docs/dev-workflow.md) "首次初始化配置"节。

| 方式 | 命令 | 场景 |
|------|------|------|
| 开发（热重载） | `make infra-up && make dev` | 日常写代码 |
| 快捷启动 | `make quick-start` | 快速验证 |
| 全 Docker | `make docker-up` | 演示/部署，零本地依赖 |
| 构建二进制 | `make build` → `./bin/vibeshop` | 轻量部署 |

## 同步检查表（Top-5 精简版）

| 改动 | 必须同时更新 |
|---|---|
| 加/改数据库表 | migration SQL + `internal/model/` struct + `internal/store/` DAO |
| 加/改 REST 端点 | `internal/server/router.go` 路由 + 对应 module handler + API 文档 |
| 加/改环境变量 | `configs/*.yaml.example` + `deploy/docker/.env.example` + `deploy/docker/docker-compose*.yml` + `docs/dev-workflow.md` + README |
| 加/改 Redis 键 | `internal/cache/keys.go` 定义 + 使用处 store 文件 + 文档说明 |
| 加/改消息 Topic | `internal/mq/topics.go` + producer + consumer + 幂等策略说明 |

**完整同步检查表（30+ 条）见 [docs/change-impact.md](docs/change-impact.md)**。加新规则只改那里。

## 禁忌与陷阱

- **不要把密钥提交到仓库**。配置文件（`configs/dev.yaml`、`deploy/docker/.env`）已加入 `.gitignore`；模板文件（`*.example`）中密钥字段必须为空字符串 `""`。Docker Compose 中密码通过 `${VIBESHOP_*}` 变量引用，不写明文。→ *违反后果：HW 代码安全扫描告警，密钥泄露风险，工单需紧急修复。*
- **不要把工单/安全扫描整改文档提交到 GitHub**。此类内容统一放 `docs/private/`，目录已加入 `.gitignore`；公开仓库只保留脱敏后的通用设计或结论。→ *违反后果：泄露工单号、内网规则、整改上下文等隐私信息。*
- **数据库用户使用业务用户，不用超级用户**。开发/生产均使用 `vibeshop` 用户（非 `root`/`postgres`），遵循最小权限原则。→ *违反后果：CodeCC inner-mdb-normal-client 规则告警；超级用户被攻破后攻击者可操作任意表。*
- **日志中不要打印敏感信息**。query 参数脱敏见 `internal/middleware/logging.go:sensitiveQueryKeys`；GORM SQL 日志不输出完整参数值（Warn 级别）。→ *违反后果：日志泄露用户密码/token，违反数据安全合规要求。*

- **不要多 main 入口**。一个 `main.go` 启动所有模块。需要只起部分模块时用配置开关，不开新进程。→ *违反后果：部署复杂度指数增长，配置漂移难以追踪，违背 ADR-001 单体优先决策。*
- **不要绕过 Redis 直接扣库存**。必须 Redis Lua 预扣 → MQ 异步落库。→ *违反后果：并发超卖，数据不一致，极难回滚。*
- **不要把订单支付超时和拼团成团超时混成同一条延迟消息**。前者是 30 分钟未支付关单；后者是按活动时限处理未成团退款。→ *违反后果：未支付订单取消与已支付团退款互串，导致错误关单、漏退款或错退款。*
- **Feed 大 V 必须走拉模式**。阈值在 `internal/module/feed/config.go:PushThreshold`。→ *违反后果：大 V 发文时写扩散到百万粉丝 inbox，Redis 内存暴涨 + 写入延迟飙升。*
- **AI 调用必须经 MCP Gateway 模块**。不要直连模型 API。→ *违反后果：绕过限流/计费/fallback/审计，失去统一管控能力，模型费用失控。*
- **跨模块调用走接口而非直接 import**。模块间通过 interface 解耦，为将来拆服务留后路。→ *违反后果：循环依赖、模块耦合，未来拆微服务时需要大规模重构。*
- **抽奖概率用 `crypto/rand`**。不用 `math/rand`。→ *违反后果：`math/rand` 可预测，被黑产利用刷奖品，造成资损。*
- **优惠券核销必须幂等**。用 order_id 做幂等键。→ *违反后果：网络重试/MQ 重投导致重复核销，资损 + 用户投诉。*
- **Feed 分页用 cursor，不用 OFFSET**。→ *违反后果：OFFSET 在大数据量下性能急剧下降（全表扫描），且并发写入时出现重复/遗漏。*

## 文档同步规范（HARD RULES，严格遵守）

这个项目的文档和代码同步是第一优先级。**所有涉及代码或行为变更的工作必须遵守以下 8 条**。违反任何一条的 commit 应被回退。

### R1. 设计先行（Design before code）
非 trivial 改动（≥ 2 个文件、或影响外部可观察行为、或新增跨模块接口）**必须在动代码前**留下设计说明：
- **架构级**（模块边界、协议、数据模型、跨模块接口）→ 新增 `docs/adr/NNN-<slug>.md`，沿用现有 ADR 模板（背景 / 决策 / 权衡 / 推翻条件 / 代码锚点）
- **功能级**（新 endpoint、新消息类型、新页面）→ `docs/features/<slug>.md` 设计文档
- **trivial**（改 bug、调文案、重命名变量、补 log）→ 可跳过；但 commit 信息里要说清楚

没有设计文档的架构级改动，review 阶段要打回。

### R2. 同步检查表（Change-impact table）
下表列出**最高频**的 5 条"改 X 必须**同时**改 Y"。动手前自查，提交前再过一遍：

| 改动 | 必须同时更新 |
|---|---|
| 加/改数据库表 | migration SQL + `internal/model/` struct + `internal/store/` DAO |
| 加/改 REST 端点 | `internal/server/router.go` 路由 + 对应 module handler + API 文档 |
| 加/改环境变量 | `configs/*.yaml` + `deploy/docker/docker-compose*.yml` + `docs/dev-workflow.md` 环境变量参考表 + README |
| 加/改 Redis 键 | `internal/cache/keys.go` 定义 + 使用处 store 文件 + 文档说明 |
| 加/改消息 Topic | `internal/mq/topics.go` + producer + consumer + 幂等策略说明 |

**其余耦合规则（MCP Tool / 新模块 / 前端联动等）全部见 [docs/change-impact.md](docs/change-impact.md)**。加新规则**只改那里**，不要写回本表。

### R3. 代码锚点（Anchored references）
文档里引用代码必须用 `path:line` 或 `path:function` 形式，例如 `internal/module/feed/config.go:PushThreshold`、`internal/server/router.go:42`。不要写"在某个模块里"这种模糊描述——锚点脱节 grep 就能发现，模糊描述只能靠人肉读。

### R4. 单一来源（Single source of truth）
每个事实只在一处写详细版，其他文档链过去：
- 环境变量明细 → `docs/dev-workflow.md` 的"环境变量参考"表是唯一详细版；README / docker-compose 只列名称 + "详见 dev-workflow.md"
- 架构决策 → 各 `docs/adr/NNN-*.md` 唯一详细版；AGENTS.md 只放一句话总结 + 链接
- 代码地图 → `docs/code-map.md` 唯一详细版（代码实现后创建），AGENTS.md 只放精简版 + "详见 code-map.md"
- 同步检查表 → `docs/change-impact.md` 唯一详细版；AGENTS.md 只放 Top-5
- 工单/安全整改隐私材料 → `docs/private/` 本地保留；公开文档只写脱敏摘要

修改时只动一处，避免"一处改另一处忘"。

### R5. 弃用不删（Deprecate, don't delete）
推翻旧设计时旧文档**保留**，首行加：

```markdown
> **⚠ DEPRECATED — <一句话说明>**
>
> 当前设计见：[链接]。本文保留作历史档案，不要据此改代码。
```

绝不 `git rm` 过期文档（保留决策轨迹）；也绝不让过期文档无标注地继续出现在索引里。

### R6. Commit 中声明文档影响（Doc-Impact tag）
每个改代码的 commit 必须在 message 末尾加一行，格式是**机器可 parse 的稳定格式**：

```
Doc-Impact: none
```
或（逗号+空格分隔，不写动词）：
```
Doc-Impact: docs/architecture.md, AGENTS.md
```

- key 固定 `Doc-Impact`（Title-Case、连字符），value 要么是 `none`，要么是逗号+空格分隔的相对路径列表
- `none` 必须真的 none——如果 R2 / `docs/change-impact.md` 有一行命中了当前改动，`none` 就是错的
- `scripts/check-docs.sh` 会 parse 这行对账；声明错会炸

### R7. 编码后 sweep（Post-code review sweep）
写完代码、准备提交前，**必须**按以下顺序人工扫：

1. `AGENTS.md`（核心架构决策、禁忌、代码地图精简版）
2. `docs/architecture.md`（数据流、模块边界）— 实现后创建
3. `docs/code-map.md`（"改 X 动哪"对照）— 实现后创建
4. `docs/dev-workflow.md`（SOP、env var、日志前缀）
5. 相关 ADR
6. `docs/change-impact.md`（同步检查表详版）
7. `README.md`（对外速查）

不改也要扫。看到"虽然不是我这次要动的，但已经和代码对不上"的地方，顺手修掉或记 issue；不要装没看见。

可以运行 `scripts/check-docs.sh` 做机械化校验（env var 对账 / 代码锚点存在性 / Doc-Impact 格式检查）。

### R8. Release 前全量校验（Release gate）
`make build` 之前建议跑一遍：
- `scripts/check-docs.sh` 退出 0
- 文档里提到的所有环境变量名 grep 一遍 `configs/*.yaml` + 代码，确认都存在
- 文档里提到的所有 `path:function` 锚点抽样检查，确认没位移
- 有任何一条对不上，**不允许发版**，先修文档或代码

---

## 功能开发循环（Feature Dev Loop）

当用户提出功能需求时，严格按以下循环执行：

### Phase 0: 需求确认
1. 复述需求 + 定边界（做什么/不做什么）
2. 识别依赖和前置条件

### Phase 1: 设计输出（必做）
先写 `docs/features/<slug>.md`，包含目标、数据模型、接口设计、实现步骤、退出门。

### Phase 2: 实现
按设计逐步实现。每改一个文件先读它。遵守禁忌和检查表。

### Phase 3: 自验
编译通过 + 退出门验证 + 检查表过一遍 + 禁忌扫描。

### Phase 4: 收尾
更新 AGENTS.md + 生成 commit message + 报告改动。

## 常见任务入口（代码地图精简版）

| 想做什么 | 动哪里 |
|---|---|
| 加 REST 端点 | `internal/server/router.go` 挂路由 → `internal/module/<name>/handler.go` → `internal/store/` |
| 加数据库表 | `scripts/migration/{mysql,pg}/NNNNN_name.sql`（goose 格式） → `internal/model/` struct → `internal/store/` DAO；用 `make migrate` / `make migrate TARGET=mysql` 执行 |
| 加 Redis 缓存 | `internal/cache/keys.go` 定义 key → `internal/cache/<name>.go` 实现 |
| 加消息 Topic | `internal/mq/topics.go` 定义 → producer 模块 → consumer 模块 |
| 加 MCP Tool | `internal/module/mcp/tools/` 实现 → `configs/mcp-tools.yaml` 路由 → 文档 |
| 加新业务模块 | `internal/module/<name>/` 全套（module.go / handler.go / service.go / dto.go / errors.go） → `internal/server/router.go` 注册 → AGENTS.md 更新 |
| 改配置项 | `configs/*.yaml` + `internal/config/` 加载逻辑 + `docs/dev-workflow.md` 环境变量表 |
| 改中间件 | `internal/middleware/` + `internal/server/router.go` 注册顺序 |

详版见 [docs/code-map.md](docs/code-map.md)（代码实现后创建）。

---

## 延伸阅读

- [docs/intro/](docs/intro/) — **项目导读（小白友好，含术语表 + 决策摘要 + 链路走读）**
- [docs/adr/](docs/adr/) — 架构决策记录（为什么这么做）
- [docs/features/](docs/features/) — 功能设计文档（做之前先写这里）
- `docs/private/` — 本地私有文档（工单/安全扫描/整改记录，不提交 GitHub）
- [docs/change-impact.md](docs/change-impact.md) — R2 同步检查表详版（加耦合规则只改这里）
- [docs/architecture.md](docs/architecture.md) — 系统架构全景图（实现后创建）
- [docs/code-map.md](docs/code-map.md) — "我想改 X 应该动哪"详版（实现后创建）
- [docs/dev-workflow.md](docs/dev-workflow.md) — 开发工作流（环境变量唯一权威来源）
- [docs/plan.md](docs/plan.md) — 功能实现计划书（分阶段路线图）
- [README.md](README.md) — 项目介绍
