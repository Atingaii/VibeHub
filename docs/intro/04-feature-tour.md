# 04 — 主链路走读

> 5 条核心链路的白话讲解。每条配上"小白也能看懂的步骤" + 关键技术点 + 现状标注。
>
> 不熟的词请查 [00-glossary.md](00-glossary.md)。
>
> 注意：链路 0（用户注册）已在 1.1 阶段实现，可以真的跑起来；链路 A-D 仍是**已完成的设计**，业务代码逐阶段补齐中。

---

## 链路 0 — 用户注册（已实现）<a id="链路-0--用户注册已实现"></a>

> 1.1 阶段已完成，是当前唯一**业务**端点（`/health` 是基础设施端点）。
> 走完这条链路，你能体感整个 ADR-001 单体 + ADR-002 双库 + 0.4 GORM + 1.1 goose 迁移的协同。

### 业务流程白话版

1. 调用方提交 `POST /api/v1/auth/register`，body 里 `username` / `phone` / `email` 三选一作为身份标识，外加 `password`
2. 服务端先**规范化**输入（`username` / `email` 转小写 + 去空白、`phone` 仅去空白），再校验三选一规则与各字段格式
3. 通过校验的 `password` 用 bcrypt（cost=10）哈希
4. 数据落 MySQL `users` 表；唯一索引保证标识不重复，CHECK 约束 `chk_users_identifier_present` 兜底"至少一个标识非空"
5. 成功返回 201 + 用户基本信息；标识冲突 409；入参不合法 400

### 后端怎么跑

```
前端 ─POST /api/v1/auth/register──┐
                                   │
                                   ▼
                              router.go
                                   │
                                   ▼
                       user.handler.Register
                          解析 JSON / 错误码映射
                                   │
                                   ▼
                       user.service.Register
                                   │
              ┌────────────────────┼────────────────────┐
              │                    │                    │
              ▼                    ▼                    ▼
        normalizeAndValidate   bcrypt.GenerateFrom   store.UserStore.Create
        TrimSpace+ToLower      Password (cost=10)    GORM INSERT users
        三选一 + 正则 + 严格邮箱校验                       │
                                                          ▼
                                                MySQL users 表
                                                ├─ 唯一索引（username/phone/email）
                                                └─ CHECK chk_users_identifier_present
                                                          │
                                          ┌───────────────┼───────────────┐
                                          │               │               │
                                          ▼               ▼               ▼
                                    1062 冲突        3819 CHECK         成功
                                  ErrIdentifier   ErrIdentifier    填回 ID/CreatedAt
                                       Taken           Missing
                                          │               │               │
                                          ▼               ▼               ▼
                                       409              400 (兜底)       201 + JSON
```

### 关键技术点

- **三列均可 NULL + 各自唯一索引**——MySQL 唯一索引允许多个 NULL，恰好支撑"任一标识即可注册"语义。
- **service 与 DB 双层防线**：service `normalizeAndValidate` 是主防线；DB 层 `chk_users_identifier_present` CHECK 约束兜底未来跳过 service 的写入路径（admin / 批量导入 / 其他模块直插）。两层都通过 sentinel error 一致映射到 HTTP 400/409。
- **唯一性的真实性来源是 DB 唯一索引**，service 不做先查后写（避免 TOCTOU 竞态 + 多余查询）。
- **邮箱严格校验**：正则锁定 `local@domain.tld` 形态 + `net/mail.ParseAddress` 后强制 `addr.Name == ""` 且 `addr.Address == 输入`，拒绝 `Foo <a@b.com>` 这种 display-name 形式与无 TLD 形式。
- **密码安全**：bcrypt 60 字节 hash 入库；GORM logger 启用 `ParameterizedQueries=true`，SQL 错误日志只渲染占位符，确保 hash / token 等绑定值永远不进日志。
- **数据库迁移是 goose 库模式**：SQL 文件通过 `//go:embed` 编译进二进制，`migrate up/down/status [mysql|pg|all]` 子命令按 target 懒加载单库——单库命令不会因为另一库不可达而失败。

### 你能跑起来的版本

```bash
make infra-up && make migrate && make run
# 另一个终端：
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"username":"alice","password":"Strong#1"}'
# => 201 {"user_id":1,"username":"alice","phone":null,"email":null,"created_at":"..."}
```

代码锚点：`internal/module/user/{handler,service,errors,dto}.go` + `internal/store/user_store.go` + `scripts/migration/mysql/00001_create_users.sql` + `00002_users_require_identifier.sql`。

设计文档详版：[docs/features/1.1-user-register.md](../features/1.1-user-register.md)。

---

## 链路 A — 拼团下单（开团 / 跟团 / 自动成团）

### 业务流程白话版

1. 用户 A 在某商品页点"开团"——先创建一张拼团订单，支付成功后把团号 `T123` 分享给朋友
2. 朋友 B 拿到链接进来"跟团"——同样下单、付款
3. 每张待支付订单都只有 **30 分钟支付窗口**；超时未支付就自动取消，不进入成团判断
4. 团真正开始倒计时后，若 24 小时内凑够人数（比如 3 人）→ 团号 `T123` **自动成团**
5. 24 小时没凑够 → **自动退款**给所有已支付成员

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
        查 SKU/价格      Redis 原子扣减  写待支付订单 (MySQL)
              │               │               │
              └───────┬───────┴───────────────┘
                      │
                      ▼
              发 NATS 消息 "order.payment.timeout"
                      │
            （30 分钟后由 order Consumer 收到）
                      │
          订单仍未支付？
          ├─ 是 → 关单 + 回滚库存
          └─ 否 → 支付回调成功，团状态=进行中
                      │
                      ▼
        发 NATS 消息 "groupbuy.deadline.reached"
                      │
            （示例：24 小时后由 groupbuy Consumer 收到）
                      │
                      ▼
                查团状态：满员？
                ├─ 是 → 幂等退出 / 全员订单标"待发货"
                └─ 否 → 退款已支付成员（调 payment.Service）
```

### 关键技术点

- **库存预扣防超卖**：用 [Lua 脚本](00-glossary.md#lua)在 Redis 端原子完成"读 → 判 > 0 → 扣"，整段不会被打断。Pool 走 `PoolStock`（DB 2，noeviction，不能静默丢）。
- **订单写 MySQL**：因为是交易数据（强一致 + 行级锁），按 [ADR-002](../adr/002-dual-database.md) 选 MySQL。
- **两类 deadline 必须拆开**：`order.payment.timeout` 只处理 30 分钟未支付关单；`groupbuy.deadline.reached` 只处理活动时限（本文示例 24 小时）未成团退款。两者 topic、consumer、幂等键都不能混用。
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
| 拼团下单 | 用户开团 / 跟团 | Redis Lua + NATS 双 deadline | MySQL orders + Redis stock | **同步**扣库存写订单 + **异步**支付超时/成团超时处理 | [规划中，阶段 4] |
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
