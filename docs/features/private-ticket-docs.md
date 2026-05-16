# private-ticket-docs — 工单与整改私有文档收口

## 背景

当前仓库中的部分整改文档包含以下不适合进入 GitHub 的信息：
- 工单号、扫描平台规则名、内网整改上下文
- 安全扫描原始问题描述和处置细节
- 仅供内部排障/复盘使用的记录

这些内容应保留为本地私有材料，不进入公开仓库。

## 目标

- 为工单/安全扫描/整改记录建立固定的本地私有目录
- 让私有文档默认被 Git 忽略，不再误提交
- 将已存在的私有文档从仓库追踪中移出，但保留本地文件
- 明确公开文档与私有文档的边界

## 数据模型变化

无

## 接口变化

无

## 实现步骤

1. 新增本地私有目录约定：`docs/private/`
2. 在 `.gitignore` 中忽略 `docs/private/`
3. 将现有工单/安全整改文档移入 `docs/private/`
4. 更新 `AGENTS.md`、`docs/features/README.md`、`docs/change-impact.md`、`docs/dev-workflow.md`
5. 调整 `scripts/check-docs.sh`，默认跳过 `docs/private/`

## 公开/私有边界

### 公开仓库允许保留

- 通用设计说明
- 不含工单号的整改结论
- 不含内网平台名称、扫描明细、账号口令、人员信息的摘要

### 必须放入私有目录

- 工单号、扫描单号、内网平台链接
- 安全扫描原文、整改过程记录
- 含敏感上下文的复盘、截图、日志片段

## 退出门

- [ ] `docs/private/` 已加入 `.gitignore`
- [ ] 现有私有文档已移入 `docs/private/`
- [ ] `scripts/check-docs.sh` 不扫描 `docs/private/`
- [ ] 公开文档已补充私有文档使用规则
