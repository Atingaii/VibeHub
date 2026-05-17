# VibeShop

社交电商 + AI 内容平台。集拼团购物、博文 Feed 流、AI MCP Gateway 于一体。

## 项目状态

**当前阶段：阶段 1 — 用户体系（进行中）**

- [x] 阶段 0 — 项目骨架（0.1 ~ 0.8 + 0.9 端口参数化）
- [x] 1.1 用户注册（密码注册 + goose 迁移基础设施）
- [ ] 1.2 用户登录 / JWT 签发
- [ ] 1.3 JWT 鉴权中间件
- [ ] 1.4 用户资料 CRUD
- [ ] 1.5 用户标签体系
- [ ] 1.6 关注 / 粉丝
- [ ] 阶段 2-12：商品 / 订单 / 拼团 / 券 / 抽奖 / 内容 / Feed / AI / 人群 / 前端 / 移动端

## 快速开始

```bash
# 步骤 0：初始化密钥配置（首次 clone 必做）

# Docker 密钥（MySQL/PG/Redis 密码）：
cp deploy/docker/.env.example deploy/docker/.env
# 编辑 deploy/docker/.env，填入真实密码
# - MySQL/Redis/JWT 用 openssl rand -base64 24
# - PG 必须 URL-safe，用 openssl rand -hex 24（详见 .env.example 内联说明）

# 应用配置（可选，环境变量优先级更高）：
cp configs/dev.yaml.example configs/dev.yaml
# 编辑 configs/dev.yaml，填入 DSN 等配置（或通过环境变量覆盖）

# 方式一：中间件 Docker + 本地编译运行（适合开发，推荐）
make infra-up    # 启动 MySQL/PG/Redis/NATS（全部 healthy 后返回）
go run .         # 本地编译运行应用
curl http://localhost:8080/health

# 方式二：全 Docker 一键启动（零本地 Go 环境依赖）
make docker-up
# 如果 8080 端口被占用：APP_HOST_PORT=8088 make docker-up
curl http://localhost:8080/health

# 验证输出
# => {"status":"ok","service":"vibeshop","mysql_ok":true,"postgres_ok":true,"redis_ok":true,...}
```

> **安全说明**：所有密钥通过 `deploy/docker/.env` 注入（已加入 `.gitignore`，不提交仓库）。
> 模板文件 `.env.example` / `*.yaml.example` 可提交，**不含真实密钥**。
> 详见 [docs/dev-workflow.md](docs/dev-workflow.md) — "首次初始化配置"节。

> **端口冲突**：默认占用 3306/5432/6379/4222/8222；若已被本机或 Windows 服务占用，
> 在 `deploy/docker/.env` 中设 `VIBESHOP_*_HOST_PORT` 错开主机端口（容器内端口不变），
> 同步把 `configs/dev.yaml` 的 DSN/`addr` 改成对应端口。详见 dev-workflow.md。

> **PG 密码必须 URL-safe**：pgx 按 URL 解析 DSN，密码含 `/`、`+`、`@`、`:` 等会导致连接失败。
> 用 `openssl rand -hex 24` 生成 48 hex 字符的密码（其他字段仍可用 `rand -base64`）。

## 文档索引

| 文档 | 说明 |
|------|------|
| [docs/intro/](docs/intro/) | **项目导读（小白友好，含术语表 + 决策摘要 + 链路走读）** |
| [AGENTS.md](AGENTS.md) | AI 会话入口（规则宪法） |
| [docs/plan.md](docs/plan.md) | **功能实现计划书（必读）** |
| [docs/adr/](docs/adr/) | 架构决策记录 |
| [docs/dev-workflow.md](docs/dev-workflow.md) | 开发工作流 |

## 技术栈

Go + Gin + GORM + MySQL + PostgreSQL + Redis + NATS + MCP Protocol

前端：Next.js（Web）+ Uni-app（移动端）

## 开发方式

本项目使用 Vibe Coding 模式开发。对 AI 说出功能名称（如"做阶段 0：项目初始化"），它会自动按 Feature Dev Loop 执行完整的设计→实现→验证→文档 闭环。
