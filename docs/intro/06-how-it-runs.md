# 06 — 一次请求的生命周期

> 跟着 `curl http://localhost:8080/health` 这一条最简单的请求，从 TCP 到响应，把 VibeShop 后端跑一遍。
>
> 选 `/health` 是因为它是当前唯一已实现的 HTTP 端点（阶段 0 退出门），且它**真的访问了所有底层依赖**（MySQL / PG / Redis）——一次请求把整个基础设施栈都触达一遍，最适合走读。
>
> 不熟的词请查 [00-glossary.md](00-glossary.md)。
>
> 阶段 1+ 的业务请求（登录 / 下单 / Feed）流程大同小异，到时候按本文模板替换业务模块即可。

---

## 假设

- 你已经按 [README.md](../../README.md) "快速开始" 跑了 `make infra-up && go run .`
- 终端 A 应用运行中（端口 8080）
- 终端 B 你即将敲：

```bash
$ curl http://localhost:8080/health
```

---

## Step 1：进程是怎么起来的

`go run .` 触发 `main.go:main`，按顺序：

| # | 动作 | 文件锚点 | 失败行为 |
|---|---|---|---|
| 1 | 加载 `configs/dev.yaml` + 环境变量覆盖 | `internal/config/load.go:Load` | stderr 写错误 + 退出 1（logger 还没初始化） |
| 2 | 初始化 zap logger | `internal/logger.Init` | 同上 |
| 3 | 双数据库连接池：MySQL + PG，启动时 Ping，失败重试 3 次 | `internal/database.New` | `zap.Fatal` 退出 |
| 4 | Redis 连接：通过 `PoolGeneral` 做 Ping 验证，失败重试 3 次 | `internal/cache/redis.go:NewRedis` | `zap.Fatal` 退出 |
| 5 | 创建 Gin engine + 注册路由 | `internal/server/router.go:SetupRouter` | — |
| 6 | 启动 HTTP server，监听 `cfg.Gateway.Port` | `internal/server.NewServer` + `Run` | `zap.Fatal` 退出 |

```
[main] VibeShop starting  version=dev  env=dev  port=8080  debug=true
[database] connecting to MySQL... addr=localhost:3306 ...
[database] MySQL connected  ping_ok=true
[database] connecting to PostgreSQL... addr=localhost:5432 ...
[database] PostgreSQL connected  ping_ok=true
[cache] connecting to Redis...  addr=localhost:6379  pool_size=100
[cache] Redis connected  redis_version=7.x
[server] listening on :8080
```

注意第 4 步的细节：Redis 初始化通过 `PoolGeneral` 做 Ping，而不是另开一个 client。这是 ADR-003 修订时的细节——`Client()` 委托到 `Pool(PoolGeneral)`，DB0 不出双连接池。详见 [03 — 设计决策](03-design-decisions.md) 决策 3。

---

## Step 2：TCP / HTTP 接入

`curl` 发 HTTP/1.1 GET 到 8080：

```
GET /health HTTP/1.1
Host: localhost:8080
User-Agent: curl/...
Accept: */*
```

Go 标准库 `net/http` 接管 TCP 连接，把请求交给 Gin。

---

## Step 3：Gin 中间件链

请求按注册顺序流过：

```
TCP ──▶ http.Server ──▶ Gin engine
                          │
                          ▼
                   middleware.Recovery()    ◀── 兜底 panic
                          │                     internal/middleware/logging.go:Recovery
                          ▼
                   middleware.RequestLogger() ◀── 记日志（method/path/duration/status）
                          │                       敏感字段（password / token / api_key）脱敏
                          ▼                       internal/middleware/logging.go:RequestLogger
                   route handler: GET /health
                          │
```

- `Recovery`：handler panic 时捕获，记 stack trace + 返回 500，不让进程崩
- `RequestLogger`：每个请求一行结构化日志，慢请求（> 1s）自动升级到 warn 级别。query string 里的 `password=xxx` / `token=xxx` 会被替换成 `***`

---

## Step 4：handler 干活

`internal/server/router.go:SetupRouter` 注册的 `/health` handler 做 3 件事：

```go
// internal/server/router.go:36
r.GET("/health", func(c *gin.Context) {
    status := "ok"

    mysqlOK := true
    if err := db.PingMySQL(); err != nil { mysqlOK = false; status = "degraded" }

    postgresOK := true
    if err := db.PingPostgres(); err != nil { postgresOK = false; status = "degraded" }

    redisOK := true
    if err := rds.Ping(); err != nil { redisOK = false; status = "degraded" }

    c.JSON(http.StatusOK, gin.H{ ... })
})
```

每次 ping：

### 4a. MySQL ping
- `db.PingMySQL()` → `*sql.DB.PingContext(2s timeout)`
- 从连接池借一个连接，发 `SELECT 1`，正常返回
- 如果连接死了，连接池自动 Reap + 重建

### 4b. PostgreSQL ping
- `db.PingPostgres()` → 同 MySQL 路径，但底层是 pgx driver

### 4c. Redis ping
- `rds.Ping()` → `m.Pool(PoolGeneral).Ping(ctx).Err()` （`internal/cache/redis.go:Ping`）
- 走 [Pool 抽象](00-glossary.md#pool)：`Pool(PoolGeneral)` 拿 DB 0 的 client（懒加载，第二次起命中缓存）
- Redis 服务端响应 `+PONG`

> Close 之后这条会怎么样？返回的是哨兵 client，命令上 go-redis 报 `redis: client is closed`，不会触发 nil-map panic。详见 [03 决策 3](03-design-decisions.md#decision-redis-pool)。

### 4d. 组装响应

```json
{
  "status": "ok",
  "service": "vibeshop",
  "version": "dev",
  "env": "dev",
  "mysql_ok": true,
  "postgres_ok": true,
  "redis_ok": true
}
```

---

## Step 5：响应回流

```
handler 返回 ──▶ c.JSON 写 ResponseWriter
                        │
                        ▼
                middleware.RequestLogger 后置段
                        │   计算耗时 / 记日志
                        ▼
                middleware.Recovery 后置段
                        │
                        ▼
                Gin 写 HTTP/1.1 200 OK + body
                        │
                        ▼
                       TCP ──▶ curl 收到响应
```

终端 A 看到：

```
[INFO] http_request method=GET path=/health status=200 duration=8.3ms
```

终端 B 看到：

```json
{"status":"ok","service":"vibeshop","mysql_ok":true,"postgres_ok":true,"redis_ok":true,...}
```

---

## Step 6：进程退出

`Ctrl+C` 触发 SIGINT。`main.go` 的 defer 链按 LIFO 执行：

1. `server.Shutdown` （Gin 拒新连接 + 等存量请求结束 + timeout）
2. `redisManager.Close()` → 关所有已加载的 Pool client，加哨兵防 nil-map panic
3. `dbManager.Close()` → 关 MySQL + PG 连接池
4. `logger.Sync()` → 把缓冲区日志刷盘

进程清场退出。

---

## 业务请求会比 health 多什么

阶段 1+ 的业务请求（如 `POST /api/v1/users/login`）路径长一点，但骨架不变：

```
TCP / HTTP
    │
    ▼
Gin 中间件链：Recovery → RequestLogger →【新增】RateLimit → JWT 校验
    │
    ▼
模块路由组（/api/v1/users/* 注册到 user 模块）
    │
    ▼
user.Handler.Login                      ← 处理参数 + 调 service
    │
    ▼
user.Service.Login                      ← 业务编排
    │
    ├─ 查 PG users 表（PG）
    │
    ├─ 校验密码（bcrypt）
    │
    ├─ 签发 JWT（access + refresh）
    │
    └─ Redis Pool session：把 JWT id 写入会话集合（内含 TTL）
                          internal/cache/redis.go:RedisManager.Pool(PoolSession)
    │
    ▼
返回 { access_token, refresh_token, user_info }
```

[规划中] 阶段 1。模板就是这样，接下来 12 个阶段都在这个骨架上加内容。

---

## 接下来读什么

- 想看每个决策的"为什么"：[03 — 设计决策摘要](03-design-decisions.md)
- 想看链路对比：[04 — 主链路走读](04-feature-tour.md)
- 想看技术栈选型：[05 — 技术栈选型](05-tech-stack-rationale.md)

---

## 权威来源

- 启动流程 → `main.go:main`、`internal/server.NewServer`
- 中间件 → `internal/middleware/logging.go:RequestLogger`、`internal/middleware/logging.go:Recovery`
- /health 实现 → `internal/server/router.go` line 36 起
- Redis Pool 行为 → `internal/cache/redis.go:RedisManager.Pool`、[ADR-003](../adr/003-redis-unified-cache.md)
- 数据库连接管理 → `internal/database/`
- 日志规范 → [docs/dev-workflow.md](../dev-workflow.md) "日志安全规范" 段
