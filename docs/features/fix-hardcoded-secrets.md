# fix-hardcoded-secrets — 硬编码密钥安全修复

## 背景

代码安全扫描（HW 代码安全漏洞）发现以下问题：
- `configs/dev.yaml` / `configs/docker.yaml` 包含明文密码（MySQL/PG/Redis）和 JWT Secret
- 上述配置文件未被 `.gitignore` 排除，已随代码提交，密钥存在仓库泄露风险
- `internal/middleware/logging.go` 将完整 query 参数写入日志（可能含 token/password 等敏感字段）
- `internal/database/database.go` Debug 模式下 GORM 输出完整 SQL（含参数值）
- `internal/logger/logger.go:Sync()` 错误被静默丢弃

## 目标

- 配置文件中所有密钥清空为占位符，真实值只通过环境变量注入（项目已有 `bindEnvMappings` 机制）
- `configs/dev.yaml` / `configs/docker.yaml` 加入 `.gitignore`，不再提交真实密钥
- 提供 `*.example` 模板文件供团队成员参考配置项
- 日志中对敏感 query 参数（token/key/password/secret/code/auth 等 14 类）进行脱敏
- GORM Debug SQL 日志统一使用 Warn 级别（不输出完整参数值）
- `Sync()` 真实 flush 失败时写到 `os.Stderr` 兜底（EBADF/EINVAL 属预期行为静默忽略）

## 数据模型变化

无

## 接口变化

无（纯配置 + 内部实现变更）

## 实现步骤

### 第一轮修复（2026-05-14）
1. 修改 `configs/dev.yaml`：密钥字段清空，加注释说明通过环境变量注入
2. 修改 `configs/docker.yaml`：同上
3. 创建 `configs/dev.yaml.example`：完整模板（含所有字段，密钥为空字符串）
4. 创建 `configs/docker.yaml.example`：同上
5. 修改 `.gitignore`：加入 `configs/dev.yaml` 和 `configs/docker.yaml`
6. `git rm --cached configs/dev.yaml configs/docker.yaml`：从 git 追踪历史移除
7. 修改 `internal/middleware/logging.go`：新增 `maskQuery()` 脱敏 14 类敏感参数
8. 修改 `internal/database/database.go`：移除 Debug→Info 级别 SQL 日志，统一 Warn 级别
9. 修改 `internal/logger/logger.go`：`Sync()` 用 `fmt.Fprintf(os.Stderr)` 兜底真实 flush 失败；
   新增 `isExpectedSyncError()` 过滤 EBADF/EINVAL
10. 修改 `docs/dev-workflow.md`：新增"首次初始化配置"节 + 日志安全规范 + 移除明文密码默认值
11. 修改 `README.md`：快速开始节新增步骤 0（初始化配置）

### 第二轮修复（2026-05-15，工单 W202605151028121387139）
CodeCC 规则 `inner-mdb-normal-client` 仍扫到 `docker.yaml.example:27` 含 DSN 模板字符串。

12. 修改 `configs/dev.yaml.example` / `docker.yaml.example`：DSN 字段彻底改为空字符串 `""`，不再含任何用户名/密码/CHANGE_ME 占位符
13. 修改 `deploy/docker/docker-compose.yml` / `docker-compose.infra.yml`：所有密码改为 `${VIBESHOP_*}` 环境变量引用
14. 创建 `deploy/docker/.env.example`：Docker 环境变量模板（密钥全空）
15. 创建 `deploy/docker/.env`（.gitignore）：本地真实密钥
16. 修改 `.gitignore`：加入 `deploy/docker/.env`
17. 修改 `Makefile`：所有 `docker compose` 命令加 `--env-file deploy/docker/.env`
18. **最小权限**：MySQL 新增业务用户 `vibeshop`（通过 MYSQL_USER/MYSQL_PASSWORD），PG 用户改为 `vibeshop`；应用 DSN 不再使用 root/postgres 超级用户
19. 同步更新所有文档：AGENTS.md（目录结构/启动/禁忌/同步检查表）、change-impact.md（密钥安全规则）、README.md、dev-workflow.md

### 踩坑记录

- **docker compose `.env` 中 `$` 字符会被解释为变量引用**：密码 `Vb$hop_mysql_2026!` 中的 `$hop_mysql_2026!` 被解释为空变量，导致实际密码变为 `Vb`。**解决**：`.env` 中密码不使用 `$` 字符，或用 `$$` 转义（仅在 YAML 中生效，`.env` 中 `$$` 无效）

## 豁免说明（误报/设计合理项）

| 问题 | 处理方式 |
|------|---------|
| `sslmode=disable`（本地开发） | 设计合理，本地容器内部通信无需 TLS；加注释 + `#nosec` 标注 |
| HTTP 无 TLS（`server.go`） | 设计合理，生产环境通过 Nginx 反向代理终止 TLS；代码层不加 TLS |
| `_ = v.BindEnv(...)` | Viper 当前版本此函数不返回有效 error，属低风险；加注释说明 |

## 退出门

### 第一轮
- [x] `go build ./...` 编译通过
- [x] `configs/dev.yaml` 不含明文密码（grep 验证）
- [x] `configs/dev.yaml` 在 `.gitignore` 中，`git ls-files configs/dev.yaml` 为空
- [x] 日志中敏感 query 参数被替换为 `[REDACTED]`（`internal/middleware/logging.go:maskQuery`）
- [x] `logger.Sync()` 真实 flush 失败时输出到 `os.Stderr`，EBADF/EINVAL 静默忽略
- [x] `docs/dev-workflow.md` 包含完整的首次初始化配置说明
- [x] `README.md` 快速开始含步骤 0（初始化配置）

### 第二轮（工单 W202605151028121387139）
- [x] `git ls-files | xargs grep` 无明文密码 / CHANGE_ME / root:@tcp / postgres:@
- [x] Docker Compose 中所有密码通过 `${VIBESHOP_*}` 变量引用
- [x] `deploy/docker/.env.example` 存在且密钥全空
- [x] `deploy/docker/.env` 在 `.gitignore` 中
- [x] `make infra-up` 4/4 healthy（无变量未定义警告）
- [x] MySQL 使用 `vibeshop` 用户（`/health` 验证 `mysql_ok: true`）
- [x] PG 使用 `vibeshop` 用户（`/health` 验证 `postgres_ok: true`）
- [x] AGENTS.md 目录结构 / 启动方式 / 禁忌 / 同步检查表已更新
- [x] docs/change-impact.md 密钥安全规则已添加
