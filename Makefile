# Makefile — VibeShop 构建系统
# 单体架构：一个二进制，多种启动方式

.PHONY: all dev run build build-linux build-linux-arm \
	test test-race lint doc-lint \
	infra-up infra-down infra-clean infra-status infra-wait \
	docker-build docker-up docker-down docker-logs \
	first-run quick-start full-start \
	migrate migrate-status migrate-down seed clean help

# ============= 变量 =============

APP_NAME   := vibeshop
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)
INFRA_COMPOSE := deploy/docker/docker-compose.infra.yml
FULL_COMPOSE  := deploy/docker/docker-compose.yml
ENV_FILE      := deploy/docker/.env

# ============= 开发（本地）=============

dev: ## 热重载启动后端（需要先 go install github.com/air-verse/air@latest）
	air

run: ## 直接启动后端（不带热重载）
	go run .

# ============= 构建 =============

build: ## 编译生产二进制
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(APP_NAME) .

build-linux: ## 交叉编译 Linux amd64（适合 Docker/服务器）
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/$(APP_NAME)-linux-amd64 .

build-linux-arm: ## 交叉编译 Linux arm64
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/$(APP_NAME)-linux-arm64 .

# ============= 测试 =============

test: ## 运行单元测试
	go test ./... -cover -count=1

test-race: ## 带竞态检测测试
	go test ./... -race -count=1

lint: ## 代码静态检查
	golangci-lint run ./...

doc-lint: ## 文档-代码一致性检查
	./scripts/check-docs.sh

# ============= 本地基础设施（仅中间件）=============

infra-up: ## 启动本地基础设施（MySQL/PG/Redis/NATS）
	@echo "🚀 启动基础设施..."
	@docker compose --env-file $(ENV_FILE) -f $(INFRA_COMPOSE) up -d
	@$(MAKE) infra-wait

infra-wait: ## 等待基础设施健康就绪
	@echo "⏳ 等待服务健康检查通过..."
	@for i in 1 2 3 4 5 6 7 8 9 10 11 12; do \
		HEALTHY=$$(docker ps --filter "name=vibeshop" --filter "health=healthy" --format "{{.Names}}" 2>/dev/null | wc -l); \
		if [ "$$HEALTHY" -ge 4 ]; then \
			echo "✅ 全部 4 个服务已健康就绪"; \
			break; \
		fi; \
		if [ "$$i" -eq 12 ]; then \
			echo "⚠️  超时：部分服务可能未就绪，请运行 make infra-status 检查"; \
			break; \
		fi; \
		echo "   等待中... ($$HEALTHY/4 healthy, 第 $$i 次检查)"; \
		sleep 5; \
	done

infra-down: ## 停止本地基础设施
	@docker compose --env-file $(ENV_FILE) -f $(INFRA_COMPOSE) down

infra-clean: ## 停止并清除数据卷（慎用！会丢失所有数据）
	@echo "⚠️  即将停止并删除所有数据卷..."
	@docker compose --env-file $(ENV_FILE) -f $(INFRA_COMPOSE) down -v

infra-status: ## 查看基础设施容器状态
	@echo "=== 基础设施容器状态 ==="
	@docker compose --env-file $(ENV_FILE) -f $(INFRA_COMPOSE) ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || \
		docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" --filter "name=vibeshop"

# ============= Docker 一键部署（应用 + 中间件）=============

docker-build: ## 构建应用 Docker 镜像
	docker build --build-arg VERSION=$(VERSION) --build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(APP_NAME):$(VERSION) -t $(APP_NAME):latest -f deploy/docker/Dockerfile .

docker-up: ## 一键启动全部（应用 + 中间件）
	@echo "🚀 构建并启动全栈..."
	@VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME) \
		docker compose --env-file $(ENV_FILE) -f $(FULL_COMPOSE) up -d --build
	@echo ""
	@echo "========================================="
	@echo "  ✅ VibeShop 已启动！"
	@echo "  应用: http://localhost:$${APP_HOST_PORT:-8080}"
	@echo "  健康检查: http://localhost:$${APP_HOST_PORT:-8080}/health"
	@echo "  提示: APP_HOST_PORT=8088 make docker-up 可指定端口"
	@echo "========================================="

docker-down: ## 停止全部容器
	@docker compose --env-file $(ENV_FILE) -f $(FULL_COMPOSE) down

docker-logs: ## 查看应用日志
	@docker compose --env-file $(ENV_FILE) -f $(FULL_COMPOSE) logs -f app

# ============= 快捷启动组合 =============

first-run: ## 一键启动（首次 clone 推荐）：自动生成密钥 → 起中间件 → 跑迁移 → 前台启动应用（Ctrl+C 退出）
	@./scripts/bootstrap.sh

quick-start: ## 快捷启动：中间件 Docker + 本地运行应用（适合开发）
	@echo ""
	@echo "========================================="
	@echo "  VibeShop 快捷启动"
	@echo "  模式: 中间件 Docker + 本地编译运行"
	@echo "========================================="
	@echo ""
	@$(MAKE) infra-up
	@echo ""
	@echo "🏃 启动应用..."
	@go run .

full-start: docker-up ## 全 Docker 启动：一条命令跑全部（适合演示/部署）

# ============= 数据库 =============

migrate: ## 执行数据库迁移（默认 all = mysql + pg；可加 TARGET=mysql|pg）
	go run . migrate up $(or $(TARGET),all)

migrate-status: ## 查看迁移状态（默认 all；可加 TARGET=mysql|pg）
	go run . migrate status $(or $(TARGET),all)

migrate-down: ## 回滚一步迁移（默认 all；可加 TARGET=mysql|pg）
	go run . migrate down $(or $(TARGET),all)

seed: ## 填充测试数据
	go run . seed

# ============= 清理 =============

clean: ## 清理构建产物
	rm -rf bin/
	rm -rf tmp/

# ============= 帮助 =============

help: ## 显示帮助
	@echo ""
	@echo "VibeShop 构建命令"
	@echo "================="
	@echo ""
	@echo "📦 快捷启动（推荐）:"
	@echo "  make first-run      🚀 一键启动（首次 clone：自动生成密钥 + 起中间件 + 迁移 + 启动应用）"
	@echo "  make quick-start    中间件 Docker + 本地运行（开发首选，需先有 .env / dev.yaml）"
	@echo "  make full-start     全 Docker 一键启动（零本地依赖）"
	@echo ""
	@echo "🔧 开发:"
	@echo "  make dev            热重载启动（需要 air）"
	@echo "  make run            直接运行"
	@echo "  make build          编译生产二进制"
	@echo "  make test           单元测试"
	@echo "  make lint           代码静态检查"
	@echo ""
	@echo "🐳 Docker:"
	@echo "  make infra-up       启动中间件（MySQL/PG/Redis/NATS）"
	@echo "  make infra-down     停止中间件"
	@echo "  make infra-status   查看容器状态"
	@echo "  make docker-up      全栈启动（应用 + 中间件）"
	@echo "  make docker-down    停止全部容器"
	@echo "  make docker-logs    查看应用日志"
	@echo ""
	@echo "📋 数据库:"
	@echo "  make migrate              执行迁移（默认 all；TARGET=mysql|pg 限定单库）"
	@echo "  make migrate-status       查看迁移状态"
	@echo "  make migrate-down         回滚一步"
	@echo "  make seed                 填充测试数据"
	@echo ""
	@echo "🧹 清理:"
	@echo "  make clean          清理构建产物"
	@echo "  make infra-clean    停止并删除数据卷（慎用！）"
	@echo ""
