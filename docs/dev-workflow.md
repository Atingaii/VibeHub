# 开发工作流

## 环境要求

- Go 1.22+
- Node.js 20+（前端开发时需要）
- Docker + Docker Compose
- air（热重载，`go install github.com/air-verse/air@latest`）
- golangci-lint（静态检查）

## 环境变量参考（唯一权威来源）

> 其他文档（README / docker-compose / configs）引用本表时只列名称 + "详见 docs/dev-workflow.md"。

| 变量 | 说明 | 默认值 | 备注 |
|------|------|--------|------|
| `APP_ENV` | 运行环境 | `dev` | dev / staging / prod |
| `APP_PORT` | HTTP 监听端口 | `8080` | 单体唯一端口 |
| `MYSQL_DSN` | MySQL 连接串 | （见 configs/dev.yaml.example） | 交易数据；格式 `user:pass@tcp(host:port)/db?...` |
| `PG_DSN` | PostgreSQL 连接串 | （见 configs/dev.yaml.example） | 内容数据；格式 `postgres://user:pass@host:port/db?sslmode=...` |
| `REDIS_ADDR` | Redis 地址 | `localhost:6379` | |
| `REDIS_PASSWORD` | Redis 密码 | （必须设置，见 configs/dev.yaml.example） | 开发/生产均需设置 |
| `NATS_URL` | NATS 连接 URL | `nats://localhost:4222` | JetStream 模式 |
| `OSS_PROVIDER` | 对象存储类型 | `local` | local / aliyun / aws |
| `OSS_LOCAL_PATH` | 本地存储路径 | `./uploads` | 仅 local 模式 |
| `OSS_ENDPOINT` | OSS 端点 | (需配置) | aliyun / aws 模式 |
| `OSS_ACCESS_KEY` | OSS Access Key | (需配置) | |
| `OSS_SECRET_KEY` | OSS Secret Key | (需配置) | |
| `OSS_BUCKET` | OSS Bucket 名 | `vibeshop` | |
| `JWT_SECRET` | JWT 签名密钥 | (需配置，≥32 字符) | 生产必须强随机 |
| `JWT_ACCESS_TTL` | Access Token 有效期 | `2h` | |
| `JWT_REFRESH_TTL` | Refresh Token 有效期 | `7d` | |
| `AI_DEFAULT_MODEL` | 默认 AI 模型 | `ollama-qwen2` | 开发用本地模型 |
| `OLLAMA_URL` | Ollama 地址 | `http://localhost:11434` | |
| `OPENAI_API_KEY` | OpenAI API Key | (可选) | |
| `CLAUDE_API_KEY` | Claude API Key | (可选) | |
| `AI_DAILY_TOKEN_BUDGET` | AI 日 token 预算 | `100000` | 超限降级 |
| `LOG_LEVEL` | 日志级别 | `debug` | debug / info / warn / error |
| `LOG_FORMAT` | 日志格式 | `console` | console / json |
| `OTEL_EXPORTER_ENDPOINT` | OpenTelemetry 导出地址 | `localhost:4317` | |
| `METRICS_PORT` | Prometheus metrics 端口 | `9100` | |

### Docker 专用环境变量（`deploy/docker/.env`）

> 以下变量仅在 Docker Compose 中使用，通过 `deploy/docker/.env` 文件注入。
> 应用代码中不直接读取这些变量（应用通过 `MYSQL_DSN`/`PG_DSN`/`REDIS_PASSWORD` 等标准变量获取）。

| 变量 | 说明 | 备注 |
|------|------|------|
| `VIBESHOP_MYSQL_ROOT_PASSWORD` | MySQL root 密码 | 仅容器初始化使用，应用不使用 root |
| `VIBESHOP_MYSQL_USER` | MySQL 业务用户名 | 默认 `vibeshop`，应用连接使用此用户 |
| `VIBESHOP_MYSQL_PASSWORD` | MySQL 业务用户密码 | 应用 DSN 中的密码 |
| `VIBESHOP_PG_USER` | PG 用户名 | 默认 `vibeshop`，非超级用户 |
| `VIBESHOP_PG_PASSWORD` | PG 密码 | |
| `VIBESHOP_REDIS_PASSWORD` | Redis requirepass 密码 | |
| `VIBESHOP_JWT_SECRET` | JWT 签名密钥 | ≥32 字符 |

> **⚠ 踩坑**：`.env` 文件中密码**不要使用 `$` 字符**，docker compose 会将其解释为变量引用。
> 如密码含 `$`，实际传入的值会被截断。建议用 `openssl rand -base64 24` 生成无特殊字符的密码。

## 配置加载机制

- 配置文件选择：`APP_ENV` 环境变量决定加载哪个文件（`configs/{APP_ENV}.yaml`），默认 `dev`
- 环境变量覆盖：**优先级高于配置文件**，所有配置项均可通过环境变量覆盖，映射规则见 `internal/config/load.go:bindEnvMappings`
- 代码入口：`internal/config/load.go:Load`

> **⚠ 安全说明**：`configs/dev.yaml` 和 `configs/docker.yaml` 已加入 `.gitignore`，不提交仓库。
> 真实密钥通过**环境变量**注入，或本地填写 `configs/dev.yaml`（不会被 git 追踪）。

> **⚠ 注意**：Go `time.Duration` 只支持 `h`/`m`/`s`/`ms`/`us`/`ns` 单位，不支持 `d`（天）。
> 7 天请写 `"168h"`，不要写 `"7d"`。

## 本地开发 SOP

### 0. 首次初始化配置（clone 后必做）

> 密钥文件（`.env`、`dev.yaml`）已加入 `.gitignore`，clone 后本地不存在，需手动创建。

**第一步：创建 Docker 密钥文件（必做）**

```bash
cp deploy/docker/.env.example deploy/docker/.env
```

编辑 `deploy/docker/.env`，填入真实密码：

```bash
# 生成强密码：openssl rand -base64 24
VIBESHOP_MYSQL_ROOT_PASSWORD=<你的MySQL root密码>
VIBESHOP_MYSQL_USER=vibeshop
VIBESHOP_MYSQL_PASSWORD=<你的MySQL业务用户密码>
VIBESHOP_PG_USER=vibeshop
VIBESHOP_PG_PASSWORD=<你的PG密码>
VIBESHOP_REDIS_PASSWORD=<你的Redis密码>
VIBESHOP_JWT_SECRET=<至少32字符，openssl rand -base64 48>
```

> Docker Compose 中所有中间件密码通过此 `.env` 文件注入（`${VIBESHOP_*}` 变量），compose 文件本身不含任何明文密码。

**第二步：创建应用配置文件（可选，环境变量可完全替代）**

```bash
cp configs/dev.yaml.example configs/dev.yaml
```

编辑 `configs/dev.yaml`，填入连接信息：

| 字段 | 说明 | 示例 |
|------|------|------|
| `database.mysql.dsn` | MySQL 连接串 | `vibeshop:你的密码@tcp(localhost:3306)/vibeshop?charset=utf8mb4&parseTime=True&loc=Local` |
| `database.postgres.dsn` | PostgreSQL 连接串 | `postgres://vibeshop:你的密码@localhost:5432/vibeshop_content?sslmode=disable` |
| `redis.password` | Redis 密码 | 与 `.env` 中 `VIBESHOP_REDIS_PASSWORD` 一致 |
| `auth.jwt_secret` | JWT 签名密钥 | 与 `.env` 中 `VIBESHOP_JWT_SECRET` 一致 |

**或者纯环境变量方式（适合 CI/容器/不想写 yaml）：**

```bash
export MYSQL_DSN="vibeshop:your_pass@tcp(localhost:3306)/vibeshop?charset=utf8mb4&parseTime=True&loc=Local"
export PG_DSN="postgres://vibeshop:your_pass@localhost:5432/vibeshop_content?sslmode=disable"
export REDIS_PASSWORD="your_redis_pass"
export JWT_SECRET="$(openssl rand -base64 48)"
```

> 环境变量优先级高于配置文件，无需修改 yaml 文件即可覆盖任意配置项。

### 1. 启动基础设施

```bash
make infra-up
# 等价于: docker compose --env-file deploy/docker/.env -f deploy/docker/docker-compose.infra.yml up -d
# 启动: MySQL + PostgreSQL + Redis + NATS，全部 healthy 后返回
```

> 基础设施密码通过 `deploy/docker/.env` 注入，
> 确保应用配置（`configs/dev.yaml` 或环境变量）中的 DSN 密码与之一致。

### 2. 数据库迁移

```bash
make migrate
# 执行 go run . migrate
# 读取 scripts/migration/mysql/*.sql 和 scripts/migration/pg/*.sql
```

### 3. 填充测试数据

```bash
make seed
# 执行 go run . seed
```

### 4. 启动应用（单体）

```bash
make dev
# 使用 air 热重载启动单体应用
# 单一进程，所有模块在同一端口: http://localhost:8080
# 模块通过配置开关控制是否加载
```

或不带热重载：

```bash
make run
# go run .
```

验证启动成功：

```bash
curl http://localhost:8080/health
# => {"status":"ok","mysql_ok":true,"postgres_ok":true,"redis_ok":true,...}
```

### 5. 启动前端（后续阶段）

```bash
cd web && pnpm dev          # Next.js http://localhost:3000
```

## 代码提交规范

### Commit Message 格式

```
<type>(<scope>): <标题>

改动摘要：
- [文件]: [做了什么]

退出门：
- [x] 验证项

Doc-Impact: none 或 文件列表
```

### Type 枚举

| type | 含义 |
|------|------|
| feat | 新功能 |
| fix | Bug 修复 |
| docs | 文档变更 |
| refactor | 重构（不改功能） |
| perf | 性能优化 |
| test | 测试 |
| chore | 构建/工具/依赖 |

### Scope 枚举

`user` / `product` / `order` / `groupbuy` / `coupon` / `lottery` / `content` / `feed` / `ai` / `mcp` / `config` / `middleware` / `infra` / `web`

## Make 命令速查

```bash
make dev            # 热重载启动（单体应用）
make run            # 直接运行（不带热重载）
make build          # 编译生产二进制
make test           # 运行单元测试
make test-race      # 带竞态检测测试
make lint           # 代码静态检查
make migrate        # 执行数据库迁移
make seed           # 填充测试数据
make infra-up       # 启动基础设施（Docker）+ 等待健康就绪
make infra-down     # 停止基础设施
make infra-status   # 查看基础设施容器状态
make infra-clean    # 停止并清除数据卷（慎用！）
make docker-build   # 构建应用 Docker 镜像
make docker-up      # 一键启动全部（应用 + 中间件）
make docker-down    # 停止全部容器
make docker-logs    # 查看应用日志
make quick-start    # 快捷启动（infra + run）
make full-start     # 全 Docker 启动
make doc-lint       # 文档-代码一致性检查
make help           # 显示帮助（分类命令列表）
```

## 日志规范

结构化 JSON 格式（生产），console 格式（开发）。统一字段：

```json
{
  "level": "info",
  "ts": "2026-05-09T12:00:00Z",
  "caller": "module/order/handler.go:42",
  "msg": "order created",
  "module": "order",
  "traceId": "abc123",
  "userId": "u_001",
  "orderId": "o_001",
  "latencyMs": 12
}
```

### 日志模块前缀

| 前缀 | 模块 |
|------|------|
| `[user]` | 用户模块 |
| `[product]` | 商品模块 |
| `[order]` | 订单模块 |
| `[groupbuy]` | 拼团模块 |
| `[coupon]` | 优惠券模块 |
| `[lottery]` | 抽奖模块 |
| `[content]` | 内容模块 |
| `[feed]` | Feed 流模块 |
| `[ai]` | AI 模块 |
| `[mcp]` | MCP Gateway |
| `[cache]` | Redis 缓存层 |
| `[mq]` | 消息队列 |
| `[http]` | HTTP 请求日志（RequestLogger 中间件） |
| `[database]` | 数据库连接层 |
| `[logger]` | 日志系统自身（Sync 失败等） |

### 日志安全规范

**query 参数脱敏**（`internal/middleware/logging.go:maskQuery`）：

HTTP 请求日志中，以下 query 参数名会自动替换为 `[REDACTED]`：
`token` / `access_token` / `refresh_token` / `password` / `passwd` / `secret` /
`api_key` / `apikey` / `key` / `auth` / `authorization` / `code` / `sign` / `signature`

如需新增脱敏字段，在 `internal/middleware/logging.go:sensitiveQueryKeys` map 中添加。

**Sync 错误处理**（`internal/logger/logger.go:Sync`）：

程序退出时调用 `logger.Sync()`。错误分两类：
- `EBADF` / `EINVAL`（进程退出时 /dev/stderr 已关闭）：预期行为，静默忽略
- 其他错误（磁盘满/文件被删导致 flush 失败）：写到 `os.Stderr`，格式 `[logger] Sync failed, buffered logs may be lost: <err>`

---

*最后更新：2026-05-15*
