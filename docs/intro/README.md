# docs/intro/ — VibeShop 项目导读

面向**第一次读这个仓库的人**：招聘方、新加入团队成员、产品/非技术合作方、AI agent 接手时的语境引导。

---

## 这层文档的定位

VibeShop 已经有几层结构化文档：

| 层 | 文档 | 受众 | 风格 |
|----|------|------|------|
| 入口 | `README.md` | 所有人 | 项目门面 + 快速开始 |
| 规则 | `AGENTS.md` | AI agent / 协作者 | 硬规则（违反就回退） |
| 决策档 | `docs/adr/` | 工程师 | 为什么选 X 不选 Y |
| 全景 | `docs/architecture.md` | 工程师 | 模块边界 / 数据流 / 锚点 |
| 路线 | `docs/plan.md` | 工程师 / 项目方 | 阶段与退出门 |
| 工作流 | `docs/dev-workflow.md` | 工程师 | SOP / 环境变量 |
| 代码地图 | `docs/code-map.md` | 工程师 | "改 X 动哪" |

但上面这套都假定读者**已经熟悉电商 / 后端 / MCP 等概念**。`docs/intro/` 补的是**导读层**：把同一套事实按阅读顺序、配前置概念词典、用白话讲一遍，让看简历的产品经理也能跟上"这个项目在做什么 + 为什么是这套设计"。

**导读 ≠ 权威**。intro 不引入新事实，只复述并指向 ADR / architecture / code-map 的权威源。任何一段被推翻时，按 R5（弃用不删）加 DEPRECATED 横幅。

---

## 推荐阅读顺序

**非技术读者 / 简历评审**：
1. [01 — VibeShop 是什么](01-what-is-vibeshop.md) — 一句话定位 + 三个核心场景
2. [00 — 前置概念汇总](00-glossary.md) — 不熟的词在这里查
3. [04 — 主链路走读](04-feature-tour.md) — 拼团 / Feed / AI 是怎么跑的
4. [03 — 设计决策摘要](03-design-decisions.md) — 6 个核心选择 + 不选什么

**新加入工程师**：
1. [01 — VibeShop 是什么](01-what-is-vibeshop.md)
2. [02 — 架构走读](02-architecture-walkthrough.md) — 三层架构白话版
3. [03 — 设计决策摘要](03-design-decisions.md)
4. [05 — 技术栈选型](05-tech-stack-rationale.md) — 当前已在 ADR 中有依据的部分摘要
5. [06 — 一次请求的生命周期](06-how-it-runs.md) — curl 到 DB 走一遍
6. 然后回到 `docs/architecture.md` / `docs/adr/` 看权威详版

**AI agent 接手**：先扫仓库根 [README](../../README.md) 拿当前阶段 + SHA，再按工程师路径读，最后跳到 `AGENTS.md` 学硬规则。

---

## 当前阶段标注

> **代码版本**：2026-05-16 / commit `5e71ee2`（refactor(cache): introduce Redis Pool abstraction）
> **阶段**：阶段 0 已完成（项目骨架 + 基础设施），阶段 1-12 [规划中]
> **最后审阅**：2026-05-16

阶段 0 退出门已全部 [✓]：
- 0.1 项目初始化 / 0.2 配置加载 / 0.3 日志 / 0.4 双数据库
- 0.5 Redis 连接 + Pool 抽象（ADR-003 修订版）
- 0.6 Docker 基础设施 / 0.7 应用镜像 / 0.8 快捷启动
- 0.9 中间件主机端口参数化 + 自启动

阶段 1-12 [规划中]——下面所有 intro 文档涉及业务模块（拼团 / Feed / AI）的实现细节都标 `[规划中]`，请不要把 intro 当成"已经做好了"的证据。

---

## 这层文档的维护规则

`docs/intro/` 是**派生层**，长期同步靠 `docs/change-impact.md` 中针对 intro 的几条规则（核心三条如下，权威全表见末尾链接）：

| 改动 | 必须同时更新 |
|---|---|
| 加 / 改 ADR | 对应 ADR + AGENTS.md 精简版 + `docs/intro/03-design-decisions.md` 摘要 |
| 加 / 改主链路数据流 | `docs/architecture.md` + 对应 ADR + `docs/intro/04-feature-tour.md` |
| 加 / 改新概念术语 | `docs/intro/00-glossary.md` 加条目 + 首次出现处链接术语表 |

详见 [docs/change-impact.md](../change-impact.md) "文档规则" 段（含弃用设计 / 加技术栈选型 / 改架构决策的同步规则）。

`scripts/check-docs.sh` 已把 `docs/intro/README.md`（本文件）纳入文档完整性检查，并新增 `[5/5]` 步：扫所有形如"反引号包裹"或链接括号中的相对 markdown 引用（`xxx.md#yyy`），校文件存在 + 目标文件中存在 `<a id="yyy">` 显式声明。注意：当前实现是文本级正则，不区分行内代码示例和真实链接——所以当心**别在示例里随手贴一个不存在的 anchor**，那会被当真。具体内容是否仍与 ADR 一致，靠 R7 的 post-code sweep 人工扫。

被推翻的段落不删，按 R5 加 DEPRECATED 横幅指向新设计——参考 [docs/features/0.5-redis.md](../features/0.5-redis.md) 头部样式。

---

## 文件索引

| 文件 | 一句话说明 |
|---|---|
| [00-glossary.md](00-glossary.md) | 前置概念汇总（小白词典） |
| [01-what-is-vibeshop.md](01-what-is-vibeshop.md) | 一句话定位 + 三个核心场景 |
| [02-architecture-walkthrough.md](02-architecture-walkthrough.md) | 三层架构白话版 |
| [03-design-decisions.md](03-design-decisions.md) | 6 个核心决策的"是什么 / 为什么 / 不选什么" |
| [04-feature-tour.md](04-feature-tour.md) | 主链路走读：拼团 / Feed 推 / Feed 拉 / AI 总结 |
| [05-tech-stack-rationale.md](05-tech-stack-rationale.md) | 技术栈选型摘要（限于 ADR 已覆盖范围）+ 现状清单 |
| [06-how-it-runs.md](06-how-it-runs.md) | 一次 HTTP 请求的端到端生命周期 |
