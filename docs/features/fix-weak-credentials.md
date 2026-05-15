# 安全修复：中间件弱口令 + Redis 未授权访问

## 背景

安全扫描报告 3 个风险：
1. **PostgreSQL 弱口令** — 密码 `123456`，端口 5432 暴露到外网
2. **MySQL 弱口令** — 密码 `123456`，端口 3306 暴露到外网
3. **Redis 未授权** — 无密码，端口 6379 暴露到外网

## 修复方案

### 1. 密码加强

| 服务 | 修复前 | 修复后 |
|------|--------|--------|
| MySQL | `root:123456` | `root:Vb$hop_mysql_2026!` |
| PostgreSQL | `postgres:123456` | `postgres:Vb$hop_pg_2026!` |
| Redis | (无密码) | `requirepass Vb$hop_redis_2026!` |

### 2. 端口绑定限制

所有中间件端口从 `"PORT:PORT"` 改为 `"127.0.0.1:PORT:PORT"`，仅允许本机访问。

### 3. 影响范围（R2 同步检查表）

根据"加/改环境变量"规则，涉及文件：

| 文件 | 改动 |
|------|------|
| `configs/dev.yaml` | MySQL DSN / PG DSN / Redis password 更新 |
| `configs/docker.yaml` | MySQL DSN / PG DSN / Redis password 更新 |
| `deploy/docker/docker-compose.infra.yml` | MySQL/PG 密码 + Redis requirepass + 端口绑定 127.0.0.1 |
| `deploy/docker/docker-compose.yml` | 同 infra + app 环境变量 DSN/REDIS_PASSWORD |
| `docs/dev-workflow.md` | 环境变量默认值更新 |
| `internal/cache/redis.go` | Redis healthcheck 需要传密码 |

### 4. Redis healthcheck 改造

Redis 设置密码后，`redis-cli ping` 不再能无密码访问，healthcheck 需要改为：
```
redis-cli -a <password> ping
```

## 退出门

- [x] `make infra-up` 后 4 个容器全部 healthy
- [x] MySQL 无法用 `123456` 登录（返回 `Access denied`）
- [x] PostgreSQL 无法用 `123456` 登录（新密码已生效）
- [x] Redis 无法无密码访问（返回 `NOAUTH Authentication required`）
- [x] 端口仅绑定 `127.0.0.1`，外网不可达（`ss -tlnp` 已验证）
- [x] `/health` 返回 `mysql_ok` + `postgres_ok` + `redis_ok` 全部 true
