# VibeShop 生产 Dockerfile
# 多阶段构建：编译阶段 + 最小运行镜像
# 最终镜像约 15-20MB（Alpine + 静态编译二进制）

# ====== 第一阶段：编译 ======
FROM golang:1.23-alpine AS builder

# 安装 git（ldflags 版本注入需要 git describe）
RUN apk add --no-cache git

WORKDIR /src

# 允许 Go 自动下载所需的 toolchain（go.mod 声明 go 1.25）
ENV GOTOOLCHAIN=auto

# 先拷贝依赖文件，利用 Docker 缓存层
COPY go.mod go.sum ./
RUN go mod download

# 拷贝全部源码
COPY . .

# 静态编译（CGO_ENABLED=0 确保无 C 依赖）
# ldflags 注入版本信息和构建时间
ARG VERSION=dev
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}" \
    -o /app/vibeshop .

# ====== 第二阶段：运行 ======
FROM alpine:3.19

# 安装最小依赖（TLS 证书 + 时区数据）
RUN apk add --no-cache ca-certificates tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

WORKDIR /app

# 从编译阶段拷贝二进制
COPY --from=builder /app/vibeshop .

# 拷贝配置文件目录（运行时可通过环境变量覆盖）
COPY configs/ ./configs/

# 安全性：使用非 root 用户运行
RUN adduser -D -H appuser
USER appuser

# 暴露应用端口
EXPOSE 8080

# Docker 健康检查（与 /health 端点对齐）
HEALTHCHECK --interval=10s --timeout=3s --start-period=15s --retries=5 \
    CMD wget -qO- http://localhost:8080/health || exit 1

# 启动命令
ENTRYPOINT ["./vibeshop"]
