# 文档索引

这套文档把当前事实、长期决策、维护经验和本机信息分开保存。开发前按需要读取，任务结束后通过 Knowledge Review 决定是否更新。

当前状态：Phase 1 已完成路线决策和文档收口，ADR-002 和 ADR-003 已接受并选择方案 B 及其 D1→D2→D3 里程碑。D1 代码切片、Linux 全包自动化验证、C92 Docker Compose 部署、公网 HTTPS 和 Movie/Episode STRM 真实 Canary 已验收。真实多媒体源样本仍未验收；在多源样本补齐前不进入 D2。上游构建基线的失败和未验证项仍以 [Phase 1 基线报告](../BASELINE.md) 为准；它们不等同于新 Go 服务的验证结果。

## 正式文档

| 文档 | 保存内容 | 不保存的内容 |
|---|---|---|
| [当前架构](architecture.md) | 已实现或实测确认的组件、数据流和边界 | 未来愿景和未验证设计 |
| [维护经验](lessons-learned.md) | 隐蔽、高复用、有证据的规则 | 一次性排错流水账 |
| [ADR](adr/README.md) | 长期架构选择、原因和代价 | 局部实现细节 |
| [Phase 1 基线检查表](phase1-baseline-checklist.md) | Phase 1 当前收口状态、基线任务和下一阶段门禁 | Phase 2 实现 |
| [Phase 2 只读 Canary](phase2-readonly-canary.md) | D1 已实现切片的 API、配置、安全边界和自动/真实验收门禁 | D2 搜索预览和 D3 写入实现 |
| [D1 部署验收报告](d1-deployment-acceptance.md) | 已验证的部署、真实 STRM Canary 和剩余门禁 | 私有部署细节、凭据和一次性操作记录 |
| [Phase 1 基线报告](../BASELINE.md) | 上游快照、构建结果、失败清单和未验证范围 | 后续阶段实现 |
| [OpenResty 公网入口基线](../deploy/openresty/README.md) | 面向所有部署者的安全反代日志、FRP 和公网端口边界 | 自动修改现有服务器配置 |
| [ChineseSubFinder 复用矩阵](../CSF_REUSE_MATRIX.md) | 上游模块耦合证据和方案 B 的复用边界 | 新运行时实现 |
| [Knowledge Review 模板](knowledge-review-template.md) | 任务结束后的知识检查格式 | 实际任务结论 |

总体规划与 Gate 0 报告暂时保留在仓库根目录。总体规划描述目标状态，Gate 报告保存真实验证过程。架构文档只吸收其中已经成立且仍然有效的事实。

## 本地文档

`LOCAL_OPERATIONS.md` 位于仓库根目录，由版本化 `.gitignore` 排除。

适合写入的内容包括服务器别名、连接方式、容器或服务名称、持久路径、PathMapper 对照、备份位置和恢复步骤。

不适合写入的内容包括明文凭据、候选 ID、带认证参数的 URL、当前进程状态和每天的操作记录。

## 维护方式

1. 任务开始前搜索现有文档，避免重复结论。
2. 任务结束时填写 Knowledge Review。
3. 先核验证据，再更新对应正式文档。
4. 架构选择写 ADR，已被替代的 ADR 保留历史并修改状态。
5. 当前状态依靠实时命令检查，文档只保存长期规则。
