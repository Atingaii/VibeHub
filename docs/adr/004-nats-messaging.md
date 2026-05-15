# ADR-004: NATS JetStream 统一消息队列

## 状态
已采纳（替代原双层 MQ 方案）

## 背景
单体架构下，异步任务场景包括：Feed 写扩散、AI 总结触发、订单超时、库存回滚等。不需要重量级的 RocketMQ/Kafka，但需要：持久化、消息确认、延迟投递。

## 决策
统一使用 NATS JetStream：
- 单二进制部署，Docker 一行搞定
- 支持持久化 Stream + Consumer ACK
- 延迟投递通过 Header + 定时 re-deliver 实现
- 初期不引入 RocketMQ，复杂度兜不住

## 推翻条件
- 需要事务消息（消息和 DB 操作原子绑定）时上 RocketMQ
- 日消息量 > 100 万/天且需要精确顺序消费时考虑 Kafka
