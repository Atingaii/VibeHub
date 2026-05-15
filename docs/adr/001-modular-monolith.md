# ADR-001: 单体优先 + 模块化内部架构（Modular Monolith）

## 状态
已采纳

## 背景
项目初期团队小（1-2人），功能多（电商+内容+AI），如果一开始就拆微服务会带来：
- 10+ 个进程需要启动和维护
- 服务间通信、服务发现、配置管理的额外复杂度
- 本地开发体验差（要开多个终端、管理多个数据库连接）
- 部署/调试困难

## 决策
采用 **Modular Monolith** 架构：
- 单个 `main.go` 入口，`go run .` 一键启动
- 内部按业务域拆模块：`internal/module/{user,product,order,groupbuy,coupon,lottery,content,feed,ai,mcp}/`
- 每个模块有独立的 handler/service/repository 三层
- 模块间通过 Go interface 通信，不直接 import 另一个模块的内部实现
- HTTP 框架统一用 Gin
- 未来需要拆服务时，把一个 module 连同其 interface 实现一起搬到独立服务，接口不变

## 权衡
| 优势 | 劣势 |
|---|---|
| 开发体验极好，本地一个进程全部启动 | 模块间强依赖风险（需纪律保证 interface 解耦） |
| 部署简单，单二进制 | 单进程性能上限（足够到日活百万级） |
| 代码搜索/重构方便 | 不适合超大团队并行开发 |
| 不需要服务发现/RPC框架 | 拆服务时有一定重构成本 |

## 推翻条件
当以下任一出现时，启动拆服务：
- 单一模块需要独立扩缩容（如秒杀时库存服务需要 10x 实例）
- 团队 > 5 人需要独立交付节奏
- 单进程内存/CPU 触顶

## 代码锚点
- 入口：`main.go`
- 模块注册：`internal/server/router.go`
- 模块间接口：`internal/module/<name>/interface.go`
