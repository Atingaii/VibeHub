# ADR-002: MySQL 8.0+ 主库 + PostgreSQL 辅库

## 状态
已采纳 — 2025-05-09

## 背景

项目包含两类截然不同的数据特征：
- **交易型数据**（用户/订单/库存/支付）：高并发写、强一致性、行级锁
- **内容型数据**（博文/评论/标签/推荐）：复杂查询、全文搜索、JSON 嵌套结构

单一数据库难以同时在两个方向做到最优。

## 决策

**双库架构**：

### MySQL 8.0+（交易主库）
- 用户账户表（users / user_profiles）
- 订单系统（orders / order_items / refunds）
- 库存系统（inventory / stock_locks）
- 支付流水（transactions / payment_records）
- 拼团系统（group_activities / group_orders / group_participants）
- 优惠券系统（coupons / coupon_records）
- 抽奖系统（lottery_activities / lottery_records）

**选择理由**：
- InnoDB 在高并发 TPS 场景下比 PG 高约 50%
- InnoDB Cluster 高可用方案最成熟
- 复制延迟比 PG 低 40%
- 运维生态完善（Percona Toolkit / ProxySQL）

### PostgreSQL 15+（内容辅库）
- 博文内容（posts / post_versions / drafts）
- 评论系统（comments / comment_likes）
- 标签/话题（tags / topics / post_tags）
- 关注关系（follows）
- Feed 流元数据（feed_items / recommendations）
- AI 总结缓存（ai_summaries）
- 搜索索引辅助表

**选择理由**：
- JSONB 类型存储博文元数据（标签、配图、外链等）支持索引
- GIN/GiST 全文搜索索引对中文友好（配合 zhparser）
- 窗口函数用于推荐排序算法
- 数组类型存储多值属性（标签列表等）

### 跨库一致性
- **不使用分布式事务（XA）** — 复杂度高、性能差
- 使用 **Saga 模式**：业务操作拆成本地事务 + 补偿事务，通过 RocketMQ 事务消息编排
- 最终一致性：电商下单 → MySQL 扣库存（本地事务）→ MQ → PG 写积分/Feed（最终一致）

### ORM
- 统一使用 GORM 2.x
- DAO 层按数据库分离：`internal/store/mysql/` 和 `internal/store/pg/`
- 连接池独立配置

## 权衡

| 方案 | 优势 | 劣势 |
|------|------|------|
| 纯 MySQL | 运维简单、一致性好 | 全文搜索弱、JSON 查询慢 |
| 纯 PostgreSQL | 功能最全面 | 高并发写场景 TPS 偏低 |
| **双库（本方案）** | 各取所长、性能最优 | 运维两套、跨库一致性复杂 |

**接受劣势的理由**：Docker Compose 让本地开发两套数据库零成本；生产环境都是托管服务（RDS），运维差异不大。Saga 模式虽然增加了代码量，但 go-zero 的 DTM 集成成熟。

## 推翻条件

- 项目规模始终 < 5 万用户 → 退回纯 PG（简化架构）
- 全文搜索需求超出 PG 能力 → 引入 Elasticsearch/Meilisearch
- 需要跨库强一致性的场景增多 → 评估 TiDB（HTAP 混合）

## 代码锚点

- `internal/store/mysql/` — MySQL DAO 层
- `internal/store/pg/` — PostgreSQL DAO 层
- `configs/dev.yaml:database` — 双库连接配置
- `scripts/migration/mysql/` — MySQL 迁移脚本
- `scripts/migration/pg/` — PostgreSQL 迁移脚本
