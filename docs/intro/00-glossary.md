# 00 — 前置概念汇总（小白词典）

> 这个文件是 VibeShop 各篇 intro 用到的术语索引。每条 2-4 行白话 + 类比，不展开实现细节。
> 实现细节的权威源在每条末尾的 → 锚点。

---

## A. 后端基本概念

### 单体架构（Monolith）
所有业务模块编译进**同一个可执行文件**，单进程运行。优点：本地一个 `go run .` 全部启动，调试 / 重构方便；缺点：模块太多时部署一次得整体重启。
→ 详见 [ADR-001](../adr/001-modular-monolith.md)

### 微服务架构（Microservices）
每个业务模块是独立服务、独立进程、独立数据库，靠网络（HTTP/gRPC）互相调用。和单体相对。优点：独立扩缩容、团队解耦；缺点：进程间通信、配置、运维复杂度都翻倍。

### 模块化单体（Modular Monolith）
<a id="modular-monolith"></a>
**外形是单体**，但内部按业务严格分包（VibeShop 是 `internal/module/{user,product,order,...}/`），模块之间只通过接口通信，不直接 import 对方的内部 struct。等于**今天单进程开发简单，未来要拆服务时按模块切就行**。
→ 详见 [ADR-001](../adr/001-modular-monolith.md)

### Modular Monolith 与微服务的关系
模块化单体是"做好准备的单体"——纪律性的接口边界让单体可以**渐进式**拆服务，而不是一上来就被分布式系统的复杂度绑架。

---

## B. 数据库与存储

### 双数据库（Dual Database）
项目同时用两种数据库，按数据特性分工：MySQL 管交易（订单 / 库存 / 用户），PostgreSQL 管内容（博文 / 评论 / Feed 元数据 / AI 总结）。
→ 详见 [ADR-002](../adr/002-dual-database.md)

### MySQL InnoDB
MySQL 默认的存储引擎，主打**行级锁 + 事务 + 高并发写**。订单系统、库存扣减这类"大量并发改同一张表"的场景适合。

### PostgreSQL JSONB / GIN
PG 的两个杀手锏：JSONB 让你把"博文带的标签 / 配图 / 外链"存成一个字段还能查询；GIN 索引让全文搜索（中文配 zhparser）跑得动。所以内容侧选 PG。

### ORM（Object-Relational Mapper）
在 Go 代码里写 `userRepo.Save(user)` 这种面向对象的写法，ORM 帮你翻译成 SQL `INSERT INTO users ...`。VibeShop 用 GORM 2.x。
→ 详见 [ADR-002](../adr/002-dual-database.md) 与 `internal/database/`

### Saga 模式（分布式事务的替代）
<a id="saga"></a>
"扣库存（MySQL）+ 写积分（PG）"这种跨库操作不能走 XA 分布式事务（贵且慢）。Saga 把它拆成两个本地事务 + 失败时跑补偿事务，靠消息队列编排。最终一致性。

### 连接池（Connection Pool）
每次连数据库都要 TCP 三次握手、认证、TLS——慢。连接池预先建好 N 个连接放着复用，业务用完归还。`pool_size` 和 `min_idle_conns` 就是池的上限和保底空闲数。

---

## C. Redis 与缓存

### Redis
内存里的键值存储。读写延迟亚毫秒级，是后端"贴身近的高速存储"。VibeShop 用它做：缓存、Feed 时间线、库存预扣、分布式锁、会话、排行榜。
→ 详见 [ADR-003](../adr/003-redis-unified-cache.md)

### Redis 逻辑 DB（DB 0..15）
单个 Redis 实例内部有 16 个独立"命名空间"（DB 0、DB 1 ... DB 15），互相隔离 key。注意：**OSS 单实例的 16 个 DB 共享同一份内存和淘汰策略**——不能让 DB1 用 noeviction、DB2 用 LRU 共存（这是个常见误解，详见 ADR-003 的修订背景）。

### Pool 抽象（VibeShop 自定义）
<a id="pool"></a>
为了不让业务代码硬编码 `client.SELECT 2` 这种 DB 数字，VibeShop 在 `internal/cache/keys.go` 定义了 6 个语义 Pool：`general / feed / stock / session / notify / rank`，每个 Pool 标注 DB 编号 + 是否必须 TTL。业务用 `RedisManager.Pool(PoolStock)` 取 client，未来真要拆物理实例时业务代码不动。
→ 详见 [ADR-003](../adr/003-redis-unified-cache.md) 与 `internal/cache/keys.go:Pool`

### maxmemory-policy（淘汰策略）
Redis 内存满时怎么腾地方：
- `allkeys-lru`：最少使用的 key 被淘汰
- `volatile-ttl`：带 TTL 的 key 中最快过期的先淘汰
- `noeviction`：不淘汰，写入直接报错

VibeShop 实例级用 `noeviction`，避免库存 / 锁这些不能丢的数据被静默 LRU 掉。代价是缓存类 Pool 必须主动用 TTL 收缩。

### TTL（Time To Live）
key 的"保质期"。设了 TTL=2h，2 小时后这个 key 自动消失。会话、缓存这类"过期就该没的"数据必须带 TTL。

### SortedSet（有序集合）
Redis 的一种数据结构：每个成员带一个分数，按分数排序。Feed 流的每条帖子的发布时间戳作分数 → 拉最新 N 条只是 `ZREVRANGEBYSCORE`，毫秒级。

### Lua 脚本
<a id="lua"></a>
Redis 内部能跑 Lua。把"读库存 → 判断 > 0 → 扣减 → 返回"这一连串操作打包成一个脚本，Redis 保证原子执行（中间不会被打断）。库存预扣防超卖的关键。

### 分布式锁
多个进程争抢同一资源时（防重复下单 / 防并发扣库存）需要锁。Redis 的 SETNX + 过期时间能实现一个简单分布式锁；项目用 `go-redsync/redsync`（Redisson 模式：可重入 + watchdog 自动续期）。

### AOF（Append-Only File）
Redis 持久化的一种方式：每条写命令追加到磁盘文件，重启时回放重建内存数据。VibeShop 的 Redis 容器开了 AOF。

---

## D. 消息队列

### 消息队列（Message Queue）
"先把任务写到队列，慢慢消费"的中间件。生产者扔消息进去就返回，消费者按自己速度处理。解耦 + 异步 + 削峰。

### NATS / JetStream
NATS 是轻量级消息中间件；JetStream 是 NATS 的持久化扩展（消息落盘 + ACK 确认 + 延迟投递）。VibeShop 用它跑 Feed 写扩散、订单支付超时、拼团成团 deadline、AI 总结触发。
→ 详见 [ADR-004](../adr/004-nats-messaging.md)

### Topic（主题）
消息按 topic 分类。"feed.push.fanout"是写扩散的 topic，"`order.payment.timeout`"处理未支付订单取消，"`groupbuy.deadline.reached`"处理拼团未成团退款。生产者发到 topic，消费者订阅 topic。

### Payment Timeout（支付超时）
订单创建后，在约定支付窗口内（当前设计为 30 分钟）一直没付款，就自动关闭。它处理的是**还没付钱**的订单，不处理已支付后的退款。

### Deadline（截止时间）
业务上的"最后期限"。在 VibeShop 里，拼团成团 deadline 指的是"这个团最晚什么时候必须凑满人"。到了这个时间还没成团，就走失败退款。

### 写扩散 vs 读扩散（Push vs Pull）
**写扩散（Push）**：作者发博文时立刻把帖子塞到所有粉丝的"收件箱"。读时只查自己收件箱。读快、写慢（大 V 100 万粉丝就要写 100 万次）。
**读扩散（Pull）**：作者发博文只写自己的"发件箱"。粉丝读 Feed 时实时去拉所有关注作者的发件箱合并。写快、读慢。
VibeShop 按粉丝数 2000 阈值切换，详见 [ADR-005](../adr/005-feed-push-pull-hybrid.md)。

### 延迟投递
"30 分钟后再处理"或"24 小时后再处理"。VibeShop 用它分别处理订单支付超时取消与拼团成团 deadline 检查，到点了消费者才收到。

### 幂等（Idempotent）
同一条消息处理多次，结果一样。消息队列经常重试，消费者必须能处理"重复收到同一条"——常见做法是用消息 ID 做唯一约束、或先查库再操作。

### 幂等键（Idempotency Key）
让消费者识别"这件事我已经处理过了"的唯一键。订单支付超时更适合用 `order_id`；拼团 deadline 更适合用 `group_id`。键选错了，就可能一条团失败被按多张订单重复退款。

---

## E. Feed 流与排序

### Feed 流
首页/关注页里源源不断的博文流。VibeShop Feed 像知乎首页：按时间或热度排，支持下拉刷新 + 上滑加载更多。

### 游标分页（Cursor-based pagination / keyset pagination）
传统 `OFFSET 100 LIMIT 20` 翻页有"插了新数据导致看到重复"的问题。游标分页改成"上一页最后一条的时间戳是 X，下一页拉 X 之前的 20 条"，新数据不会破坏分页位置。

### Wilson Score
统计学方法：给一个"点赞 - 点踩"比例算个**置信下界**。比直接 `upvotes/(upvotes+downvotes)` 公平——3 赞 0 踩比 999 赞 1 踩排得低，因为后者样本足够大。

### 时间衰减（Time Decay）
新内容打分加成，老内容打分衰减。半衰期 24 小时 = 一天后分数砍半。和 Wilson Score 相乘做最终热度排序。

---

## F. 认证与安全

### JWT（JSON Web Token）
用户登录后服务端发一个签名 token，客户端每次带在 `Authorization: Bearer <token>` 头里。服务端验签就知道是谁。无状态——服务端不用记会话。
配套问题：怎么"踢下线"？把 token ID 加到黑名单（Redis Session Pool）。

### Refresh Token
Access Token 寿命短（2 小时）防泄露；Refresh Token 寿命长（7 天）用来换新 Access Token，避免每两小时让用户重登。

### .env / 配置脱敏
真实密码、API key 不入仓库。仓库里只有 `.env.example`（字段名 + 空值）。开发者本地 `cp .env.example .env` 填真值。`.env` 在 `.gitignore`。
→ 详见 [docs/dev-workflow.md](../dev-workflow.md)

### 环境变量优先级
配置三个来源：`configs/dev.yaml` 默认值 → 环境变量覆盖（如 `REDIS_PASSWORD`）→ 容器 `.env` 注入。环境变量优先级最高。
→ 详见 [docs/dev-workflow.md](../dev-workflow.md)

---

## G. AI 与 MCP

### LLM（Large Language Model）
GPT-4o / Claude / Llama 这类大模型。VibeShop 调它们做：博文总结、关键词提取、智能客服。

### MCP（Model Context Protocol）
<a id="mcp"></a>
Anthropic 推动的标准协议，让 LLM 客户端（Claude Desktop / Cursor / 本项目自研 Gateway）以一致的方式调用"工具"（搜索、查 DB、调 API）。基于 JSON-RPC 2.0。
→ 详见 [ADR-006](../adr/006-mcp-gateway.md)

### MCP Tool
一个具名能力。例如 `summarize_article(postId)` 是 VibeShop 的总结 tool。Gateway 收到 `tools/call` → 路由到对应后端服务。

### Tool 路由 / Gateway
所有 Tool 调用先到 Gateway，Gateway 负责认证、限流、按 tool name 转发到具体后端。一道入口管多家服务。

### SSE（Server-Sent Events）
HTTP 长连接的一种，服务端往客户端持续推数据。MCP 用 SSE 让 LLM 流式返回 token（不用等整段生成完）。

### Token 预算
每个用户每天能消耗多少 LLM token（输入+输出）。超出限流，避免薅羊毛 / 失控成本。

### 模型降级链
首选 Ollama（本地、零成本）→ 超时 fallback OpenAI → 超时 fallback Claude → 都失败就返回博文摘要第一段。保证总有响应。

---

## H. 部署与运维

### Docker / Docker Compose
Docker 把"应用 + 依赖"打包成镜像，Compose 用一个 yaml 编排多个容器（应用 + MySQL + PG + Redis + NATS）一键起。
→ 详见 [docs/dev-workflow.md](../dev-workflow.md) 与 `deploy/docker/`

### 多阶段 Dockerfile
Dockerfile 里有多个 FROM 阶段：先在 builder 阶段编译 Go 二进制，再到 runtime 阶段只拷贝二进制——最终镜像小（不带 Go 工具链）。

### 健康检查（Healthcheck）
容器自报"我活着没"。Docker Compose 用它判断"启动好了"再起依赖容器。VibeShop 应用对外是 `GET /health` 返回 mysql_ok / postgres_ok / redis_ok。

### 主机端口参数化
本机 3306（MySQL 默认端口）可能已被本机服务占用。`VIBESHOP_MYSQL_HOST_PORT=3307` 把容器映射到 3307，容器内仍然 3306。
→ 详见 [docs/features/0.9-host-port-parameterize-and-autostart.md](../features/0.9-host-port-parameterize-and-autostart.md)

---

## I. 项目协作（VibeShop 特有）

### Doc-Impact
每个改代码的 commit 末尾必须写 `Doc-Impact: none` 或 `Doc-Impact: docs/foo.md, docs/bar.md`。机器可 parse，`scripts/check-docs.sh` 检查格式。
→ 详见 [AGENTS.md R6](../../AGENTS.md)

### R1..R8 硬规则
AGENTS.md 里的 8 条文档同步规则。违反任何一条的 commit 应被回退。详见 [AGENTS.md](../../AGENTS.md) "文档同步规范" 段。

### CCB（Claude Code Bridge）
本项目用的多 AI 协作框架。`/ask codex <message>` 把任务交给 Codex，Codex 评分给 JSON 反馈。VibeShop 的设计先经 Codex review 再实现。

### Doc-Impact: none 的判定
"none" 必须真的 none——R2 同步检查表（`docs/change-impact.md`）任意一行命中你的改动，none 就是错的。

---

## 怎么用这个词典

读 intro 其他几篇时遇到不熟的词，回这里查一下。如果发现某条解释**和实际代码 / ADR 不一致**——按 R5 加 DEPRECATED 横幅，指向当前权威源；不要直接改这里的解释，因为这个词典是导读层，权威源是 ADR / 代码。

新增术语：先在首次出现的 intro 文档里链接 `[术语](00-glossary.md#stable-slug)` —— `stable-slug` 是本文件里给该条目加的 `<a id="stable-slug"></a>` 锚点（中文标题在 GitHub 渲染下生成的锚点不可移植，所以用显式英文 slug）。再在本文件加条目并放上锚点。`docs/change-impact.md` 已把这条规则写入"加新概念术语"行。
