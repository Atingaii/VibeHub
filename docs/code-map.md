# 代码地图（Code Map）

> 本文档是"我想改 X 应该动哪里"的唯一详细来源。AGENTS.md 只放精简版 + 链接到这里。
>
> 不熟悉项目的读者，建议先读 [docs/intro/](intro/) 拿到术语 / 决策 / 链路的导读，再回到本文查具体路径。

---

## 目录结构与定位

```
main.go                     唯一入口：加载配置、初始化各模块、启动 HTTP server
internal/
  config/                   配置加载（Viper），环境变量绑定
  server/
    server.go               HTTP server 启动 + 优雅关闭
    router.go               总路由注册：各模块 handler 组装到 Gin
  middleware/
    auth.go                 JWT 鉴权中间件
    ratelimit.go            限流中间件
    logger.go               请求日志中间件
    trace.go                OpenTelemetry trace 中间件
    cors.go                 CORS 中间件
  module/
    user/                   用户模块（注册/登录/资料/标签/关注）
      handler.go            HTTP handler
      service.go            业务逻辑（interface 定义）
      service_impl.go       业务逻辑实现
    product/                商品模块（SPU/SKU/分类/搜索）
    order/                  订单模块（下单/支付/退款/状态机）
    groupbuy/               拼团模块（开团/参团/成团/超时）
      state_machine.go      拼团状态机
    coupon/                 优惠券模块（模板/发放/核销）
      engine.go             核销引擎
    lottery/                抽奖模块（活动/算法/防刷）
      algorithm.go          概率算法（crypto/rand）
    content/                内容模块（博文/评论/标签）
    feed/                   Feed 流模块
      config.go             推拉阈值 PushThreshold
      ranker.go             排序算法（Wilson Score）
      timeline.go           时间线操作
      cursor.go             游标分页
    ai/                     AI 模块（总结/关键词）
    mcp/                    MCP Gateway 模块
      router.go             tool 路由
      ratelimit.go          per-user 限流
      model_router.go       多模型切换 + fallback
      tools/                各 tool 实现
  model/                    数据模型定义（各表 struct）
  store/                    数据访问层（各模块 DAO）
  cache/
    keys.go                 Redis key 定义（唯一来源）
    stock.lua               库存预扣 Lua 脚本
  mq/
    topics.go               NATS topic/subject 定义（唯一来源）
    producer.go             消息发布
    consumer.go             消息消费基类
scripts/
  migration/
    mysql/                  MySQL DDL 迁移脚本
    pg/                     PostgreSQL DDL 迁移脚本
  check-docs.sh            文档一致性检查脚本
configs/
  dev.yaml                  开发配置
  prod.yaml                 生产配置（不含密钥）
  mcp-tools.yaml            MCP Tool 路由配置
deploy/
  docker/
    Dockerfile              多阶段构建
    docker-compose.yml      全栈启动（应用+中间件）
    docker-compose.infra.yml 仅中间件（开发用）
docs/
  adr/                      架构决策记录
  features/                 功能设计文档
  architecture.md           系统全景图
  change-impact.md          同步检查表详版
  code-map.md               本文件
  dev-workflow.md           开发工作流 + 环境变量
  plan.md                   功能路线图
web/                        前端 Next.js（后续阶段）
```

---

## 常见任务入口

### 我想加一个新的 REST API

1. `internal/module/<name>/handler.go` — 新增 handler 方法
2. `internal/server/router.go` — 注册路由
3. 如需鉴权：确认中间件已挂载到该路由组
4. 更新 API 文档

### 我想加一个新的数据库表

1. `scripts/migration/mysql/` 或 `scripts/migration/pg/` — 新增 DDL
2. `internal/model/<name>.go` — 定义 struct
3. `internal/store/<name>_store.go` — 实现 DAO
4. 如有缓存需求：`internal/cache/keys.go` 定义 key

### 我想加一个新的消息消费

1. `internal/mq/topics.go` — 定义 topic
2. 发布方模块 — 调用 `mq.Publish`
3. `internal/module/<name>/consumer.go` — 实现消费逻辑
4. `main.go` — 注册 consumer

### 我想加一个新的业务模块

1. 创建 `internal/module/<name>/` 目录
2. 实现 handler / service / service_impl
3. `internal/server/router.go` — 注册路由组
4. 如有独立表：走"加数据库表"流程
5. 更新 `AGENTS.md` 代码地图精简版 + 本文件

### 我想加一个新的 MCP Tool

1. `internal/module/mcp/tools/<name>.go` — 实现 Tool interface
2. `configs/mcp-tools.yaml` — 注册路由配置
3. 更新 MCP Tool 文档

### 我想改 Feed 排序算法

1. `internal/module/feed/ranker.go` — 修改排序逻辑
2. 考虑是否影响 A/B 测试配置
3. 更新 `docs/adr/005-feed-push-pull-hybrid.md` 如有方案变更

### 我想加一个新的 Redis 缓存

1. 确认归属哪个 Pool（`internal/cache/keys.go:Pool` — general/feed/stock/session/notify/rank）；如果都不匹配，先在 keys.go 加 Pool 常量并更新 ADR-003 表
2. `internal/cache/keys.go` — 定义 key pattern + TTL（按 Pool.RequiresTTL() 自检）
3. `internal/cache/<name>.go` — 实现缓存操作，通过 `RedisManager.Pool(p)` 取 client
4. 使用方 store/service 调用缓存层

### 我想改配置项

1. `internal/config/` — 添加/修改 struct 字段
2. `configs/dev.yaml` + `configs/prod.yaml` — 添加配置值
3. `docs/dev-workflow.md` 环境变量表 — 更新（唯一权威来源）
4. `deploy/docker/docker-compose*.yml` — 更新 environment 段

---

## 关键文件速查

| 关键概念 | 唯一权威文件 |
|----------|-------------|
| 所有 Redis Pool 定义 | `internal/cache/keys.go:Pool` |
| 所有 Redis key 定义 | `internal/cache/keys.go` |
| 所有 NATS topic 定义 | `internal/mq/topics.go` |
| 所有环境变量说明 | `docs/dev-workflow.md` |
| Feed 推拉阈值 | `internal/module/feed/config.go:PushThreshold` |
| 库存预扣 Lua 脚本 | `internal/cache/stock.lua` |
| 订单状态枚举 | `internal/model/order_status.go` |
| 拼团状态机 | `internal/module/groupbuy/state_machine.go` |
| MCP Tool 路由配置 | `configs/mcp-tools.yaml` |

---

*最后更新：2026-05-11*
*注：本文档中的路径为规划路径，实际文件在功能实现后对照创建。*
