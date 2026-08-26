# SubBridge 开发文档索引

本目录服务于开发、维护与验收。面向部署和日常使用，请从仓库根目录的 [README](../README.md) 开始。

## 推荐读取顺序

1. [当前状态与路线图](planning/current-status-and-roadmap.md)：确认已支持范围与剩余门禁。
2. [当前架构](reference/architecture.md)：了解已经实现或实测确认的边界。
3. [ADR 索引](decisions/adr/index.md)：了解长期且仍生效的技术决策。
4. 按任务类型进入以下分类；涉及真实环境时，再阅读对应验收记录。

## 分类

| 目录 | 内容 |
|---|---|
| [guides/](guides/) | 面向部署者的安装、升级回滚、排障和 OpenResty 公网入口说明 |
| [reference/](reference/) | 当前架构、风险分级验收矩阵、维护经验和 Knowledge Review 模板 |
| [decisions/adr/](decisions/adr/) | 已接受的架构决策、取舍与兼容约束 |
| [planning/](planning/) | 总体计划、阶段基线、功能契约、路线图和未完成工作 |
| [records/acceptance/](records/acceptance/) | 已发生的部署、Canary 与真实环境验收记录 |
| [records/reviews/](records/reviews/) | 实现评审、预检、审计、检查清单与交接记录 |

## 维护规则

- 当前状态只写入 `planning/current-status-and-roadmap.md`；历史记录保留发生当时的范围和证据。
- 当前事实写入 `reference/architecture.md`，长期取舍写入 ADR，高复用且有证据的经验写入 `reference/lessons-learned.md`。
- 本机拓扑、恢复步骤和敏感位置仅放在未跟踪的 `LOCAL_OPERATIONS.md`，不得记录明文凭据或临时状态。
- 改动文档后至少检查链接、UTF-8、尾随空白和 `git diff --check`。
