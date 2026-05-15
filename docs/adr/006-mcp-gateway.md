# ADR-006: AI MCP Gateway 统一调度

## 状态
已采纳 — 2025-05-09

## 背景

项目需要 AI 能力（文章总结、智能推荐、智能客服等），需要：
1. 统一管理多个 AI 服务（OpenAI / Claude / 本地 Ollama）
2. 对接 MCP 协议生态（Tools / Resources / Prompts）
3. 限流/计费/监控的统一入口
4. 多租户隔离（不同商户的 AI 配额独立）

## 决策

**自研 MCP Gateway**，基于 HTTP+SSE 对外 / gRPC 对内的架构：

### 架构分层

```
┌─────────────────────────────────────────────┐
│          外部客户端（前端/移动端/CLI）         │
│         HTTP + Bearer Token + SSE            │
└──────────────────┬──────────────────────────┘
                   │
┌──────────────────▼──────────────────────────┐
│              MCP Gateway（本服务）            │
│                                              │
│  ┌──────────┬──────────┬──────────────────┐  │
│  │ 认证层   │ 限流层   │ 路由层            │  │
│  │(JWT/API) │(滑窗/令) │(tool→backend)    │  │
│  └──────────┴──────────┴──────────────────┘  │
│                                              │
│  ┌──────────────────────────────────────────┐ │
│  │          Session Manager                 │ │
│  │   (会话状态 / 多轮对话 / 上下文窗口)     │ │
│  └──────────────────────────────────────────┘ │
└──────────────────┬──────────────────────────┘
                   │ gRPC / stdio
┌──────────────────▼──────────────────────────┐
│           MCP Server 池                      │
│                                              │
│  ┌────────────┐  ┌────────────┐  ┌────────┐ │
│  │ AI Summary │  │ Search     │  │ Custom │ │
│  │ Server     │  │ Server     │  │ Tools  │ │
│  └────────────┘  └────────────┘  └────────┘ │
└──────────────────────────────────────────────┘
```

### 传输层

**对外**：HTTP + SSE（Streamable HTTP，MCP 2025 规范）
- `POST /mcp` — 客户端发送 JSON-RPC 请求
- `GET /mcp/sse` — 服务器推送响应流
- 兼容标准 MCP Client（Claude Desktop / Cursor 等）

**对内**：gRPC
- Gateway ↔ 各 MCP Server 之间用 gRPC
- 编译期类型安全 + 高性能序列化
- 支持双向流（streaming tool results）

**本地开发**：stdio
- 本地调试时 MCP Server 走 stdio 启动

### Tool 路由

```yaml
# configs/mcp-tools.yaml
tools:
  - name: "summarize_article"
    backend: "ai-summary-svc"
    description: "Summarize a blog post"
    rateLimit: "10/min/user"
    
  - name: "search_products"
    backend: "search-svc"
    description: "Search products by keyword"
    rateLimit: "50/min/user"
    
  - name: "recommend_posts"
    backend: "feed-svc"  
    description: "Get personalized post recommendations"
    rateLimit: "20/min/user"
```

**路由逻辑**：Gateway 收到 `tools/call` 请求 → 按 tool name 查路由表 → gRPC 转发到对应后端 MCP Server。

### AI 总结服务（MCP Server 实现）

```go
// cmd/ai-summary-svc/tools/summarize.go
type SummarizeTool struct {
    llmClient  llm.Client       // 多模型切换
    cache      cache.Cache       // 总结缓存
    tokenBudget budget.Manager   // Token 预算
}

func (t *SummarizeTool) Call(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
    // 1. 解析参数（postId or content）
    // 2. 检查缓存
    // 3. 检查 token 预算
    // 4. 调用 LLM（OpenAI / Claude / Ollama）
    // 5. 写缓存
    // 6. 返回结构化总结
}
```

### 多模型切换策略

```
优先级（可配置）：
1. Ollama（本地部署，零成本，延迟低但质量中）
2. Claude（质量最高，适合长文总结）
3. OpenAI GPT-4o（通用，性价比高）

降级链：
Claude timeout → fallback OpenAI → fallback Ollama → fallback 返回摘要第一段
```

### 限流方案

```
三层限流：
1. Gateway 全局：令牌桶，1000 req/s
2. Per-user：滑动窗口，100 req/min
3. Per-tool：独立配额（configs/mcp-tools.yaml 定义）
```

### 可观测性

- **Tracing**：OpenTelemetry → Jaeger（每个 tool call 一条 trace）
- **Metrics**：Prometheus（请求量/延迟/错误率/token 消耗）
- **Logging**：结构化 JSON 日志（tool / user / latency / tokens）

## 权衡

**自研 vs 用开源 mcp-gateway 的理由**：
- 开源项目大多是 Node.js/Python，与 Go 后端异构增加复杂度
- 需要深度集成业务限流（per-user token 预算）
- 需要 gRPC 内部通信（开源方案多走 HTTP）
- MCP 协议本身简单（JSON-RPC 2.0），实现成本可控

**不引上游 Go SDK 完整包的理由**：
- `mcp-go` 等 SDK 封装过重，且更新频率与协议不同步
- 只需 transport 层 + JSON-RPC 解析，自己实现 < 1000 行

## 推翻条件

- AI 功能极简（只有文章总结） → 退回直接调 OpenAI API，不需要 Gateway
- 需要对接 > 50 个 MCP Server → 考虑服务网格（Istio）替代自研路由
- MCP 协议大版本变更 → 评估是否切换到官方 SDK

## 代码锚点

- `cmd/mcp-gateway/main.go` — Gateway 入口
- `cmd/mcp-gateway/transport/sse.go` — SSE 传输层
- `cmd/mcp-gateway/router/tool_router.go` — 工具路由
- `cmd/mcp-gateway/auth/` — 认证中间件
- `cmd/mcp-gateway/ratelimit/` — 限流器
- `cmd/ai-summary-svc/main.go` — AI 总结 MCP Server
- `cmd/ai-summary-svc/tools/` — Tool 实现
- `pkg/mcpsdk/` — 内部 MCP 协议封装
- `configs/mcp-tools.yaml` — Tool 路由配置
