# ADR-005　在缺少真实多源样本时有条件进入 D2

- 状态　accepted（真实样本已发现；客户端字段修正、真实 API/source 对应和 D2 安全拒绝 Canary 已完成，多源正向支持门禁仍待完整实现与 Canary）
- 日期　2026-08-24
- 相关组件　D1 真实验收、D2 搜索预览、多 MediaSource、功能开关

## 后续事实更新（2026-08-25）

C92 已提供一个真实 Movie 版本组。只读复核发现：列表或详情请求若不在 `Fields` 中包含 `AlternateMediaSources`，每个关联 Item 可能只返回默认 `MediaSource`；加入该字段后，每个详情 Item 返回两个完整 `MediaSources`。因此原 ADR 的“真实样本尚未找到”背景已结束，但“完成 API、UI 和 source 对应验收前保持多源 fail closed”的决策仍然有效。

本轮已在 `internal/embyclient` 详情请求中固定加入 `AlternateMediaSources`，并补充完整 source 列表的 DTO 合并、去重边界和 D2 409 回归测试。随后 C92 对两个真实多源 Movie Item 完成了 API/source 对应和 D2 Search、Fetch、Preview 的真实 409 安全拒绝验收，证据见 [D2 多源真实 API Canary](../../records/acceptance/d2-multisource-c92-canary-acceptance-20260825.md)。该事实更新不授权多源正向搜索、部署常开、重启或任何写入。

随后 Core A/B 已在本地源码和 Fake Emby/浏览器自动化中实现正向多 source Search、Fetch、Preview、Add 与可恢复写入，具体决策见 [ADR-008](008-core-ab-daily-source-bound-recovery.md)。本 ADR 后文的“多源继续 fail closed”描述保留其原始 Canary 门禁和真实 409 证据；真实 C92 正向多 source、文件系统、字幕流和客户端验收尚未执行，默认开关和部署授权仍不变。

## 背景

D1 的代码、自动化、C92 Docker Compose 部署、公网 HTTPS 和真实 Movie/Episode STRM Canary 已通过。两轮有界只读扫描覆盖 11 个媒体库，分别检查 1,026 个最新样本和 938 个分层样本，但没有找到包含多个 `MediaSource` 的 Movie/Episode。该结果只能证明取样范围未命中，不能证明全库不存在多源 Item，也不是多源代码失败。

原 D1 门禁要求真实 Movie、Episode 和多源 Item 全部验收后才进入 D2。继续等待未知样本会让已由独立证据支撑的单源搜索预览长期停滞；修改 Emby 或媒体来制造样本又超出本轮只读授权，并会把环境变更引入验收结果。

## 问题

在真实多源样本暂时不可得时，如何继续 D2，同时不把自动化测试冒充真实验收，也不让未经验证的多源搜索进入实际使用？

## 可选方案

1. 保持原门禁，直到发现真实多源 Item 后才开始任何 D2 工作。
2. 修改 Emby 或媒体，主动构造真实多源 Item。
3. 有条件进入 D2，将单源能力推进与真实多源支持声明拆成两个门禁。

## 最终选择

选择方案 3，并采用以下约束：

1. D1 的自动化门禁、真实 Movie/Episode STRM Canary、部署和安全边界已经足以开始 D2 的契约、实现和单源 Canary。
2. D2 初始范围只包括单 MediaSource 的 Movie/Episode。实现、部署和验收期间 `remote_search_enabled=false` 继续作为默认值；没有 D2 专项授权和验收，不得启用实际远程搜索。
3. D2 遇到多个 `MediaSource` 时必须安全拒绝，不得猜测第一个 source，也不得把 D1 的显式 source 浏览能力自动扩大为已验证的多源搜索能力。稳定错误码和响应契约在 D2 设计阶段确定，并由测试固定。
4. Fake Emby 的多源 409、显式 source 选择和候选隔离测试仍是自动化必需项，但不能替代真实多源验收。
5. 找到真实多源样本只是解除“样本缺失”这一前置条件；在完成 API、UI 和 source 对应验收前，不得宣称或启用真实多源搜索、Fetch 或预览支持。该项是多源能力的独立门禁，不再阻断单源 D2。
6. 不再为解除 D2 而继续无界扫描。后续只在环境负责人提供已知样本，或另行明确授权有界扩大扫描时复查。
7. D3 及任何 Add、Replace、Delete、Upload、Refresh 或批量写入门禁不变；本 ADR 不授权写入、部署、重启或修改 Emby/媒体。

## 选择原因

- 单源 Movie/Episode 已有真实只读证据，D2 的候选搜索、失败隔离和预览可以在不写媒体库的边界内独立实现和验收。
- 多源路径继续 fail closed，缺少真实样本不会变成隐含猜测或虚假的兼容性声明。
- 自动化证据和真实环境证据继续分开，后续拿到样本时可以补齐明确、可复核的多源门禁。
- 不修改 Emby 或媒体来制造样本，保持当前只读授权和生产环境边界。

## 已知代价

- D2 首轮不能服务多源 Item；用户遇到此类 Item 时会收到明确的安全拒绝。
- 真实多源正向搜索的 API/UI 对应关系仍需后续实现和样本验收；本次只证明真实 API/source 对应和安全拒绝，不能把它扩大成 D2 正向支持结论。
- D1 的“全部真实样本齐备”和“足以有条件进入 D2”成为两个不同状态，阶段报告必须明确区分。

## 后续影响

- 下一项工作是先完成 [D2 搜索预览契约](../../planning/d2-search-preview-contract.md) 中的 API、状态模型、Token/Artifact 生命周期、候选级失败、安全日志和资源上限设计，再开始代码实现。
- D2 实现必须默认关闭功能开关，并为单源成功、多源拒绝、候选局部失败、超时和无结果建立自动化测试。
- 真实多源样本出现后，只做有界、脱敏、只读验收；本次安全拒绝证据已记录，正向支持仍需新的实现、授权和 Canary 证据。

## 验证依据

- [D1 部署验收报告](../../records/acceptance/d1-deployment-acceptance.md)
- [Phase 2-D1 只读 Canary 验收定义](../../planning/phase2-readonly-canary.md)
- [维护经验](../../reference/lessons-learned.md)
- [ADR-003](003-phase2-milestones-and-deployment.md)
- [ADR-004](004-item-and-source-path-separation.md)
