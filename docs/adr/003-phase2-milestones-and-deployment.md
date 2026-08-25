# ADR-003　Phase 2 里程碑与默认部署边界

- 状态　accepted
- 日期　2026-08-24
- 相关组件　Phase 2 只读模型、Emby Client、PathMapper、字幕清单、部署配置

## 背景

ADR-002 已确定采用新建轻量 Go 后端的方案 B。进入应用实现前，需要把“能部署进行实际测试”的路径拆成可回退、可验收的里程碑，并固定首轮部署的安全默认值。

## 决策

Phase 2 采用以下顺序，前一阶段的真实验收通过后才能进入下一阶段：

1. **D1 只读 Canary**：浏览 Emby 媒体，生成 `MediaContext`，完成路径映射和字幕清单展示；所有写能力关闭。
2. **D2 搜索预览**：在 D1 数据和安全边界稳定后，增加远程字幕搜索、候选级失败隔离、Fetch、校验和预览；仍不写入媒体库。
3. **D3 专用样本 Add**：只对明确指定的专用样本开放 Add，完成新版本文件写入、Emby 刷新、直连核验和可恢复归档；通过真实验收后才讨论 Replace、Delete 和批量能力。

默认部署采用 Linux Docker Compose 的单应用容器。容器只通过私网或 SSH 隧道访问 Emby 和管理界面，不默认公开暴露管理端点。媒体目录以只读方式挂载，配置默认 `write_enabled=false`。API Key 通过容器 Secret 或等价的受保护文件注入，不进入镜像、前端、普通日志或响应。

## 选择原因

- D1 先验证 Item、MediaSource、STRM 和路径映射，避免在写入功能中发现领域模型错误。
- D2 把远程候选失败和字幕内容校验隔离在预览边界内，避免未经验证的内容落盘。
- D3 使用专用样本限制真实写入影响范围，并让文件 Hash、Emby MediaStreams 和客户端读取形成完整证据链。
- 单容器、只读挂载和 `write_enabled=false` 使首轮部署的权限和恢复边界清晰；后续扩展不改变默认安全状态。

## 已知代价

- D1 不能直接验证字幕写入、刷新和客户端缓存行为。
- D2 需要维护短期候选 Token 和 PreviewArtifact，不能把 Emby 原始候选 ID 当作长期公共 API。
- D3 首轮只能使用专用样本，不能作为批量处理或全库自动化的证明。
- 私网或 SSH 隧道依赖部署环境已有可用的网络路径；本 ADR 不授权修改远端服务或防火墙。

## 验证依据

- [当前架构](../architecture.md)
- [总体规划](../../Emby_STRM_Subtitle_Manager_Master_Plan_Revised.md)
- [Gate 0 实测报告](../../GATE0_REPORT.md)
- [Phase 2 只读 Canary 验收定义](../phase2-readonly-canary.md)

本 ADR 记录路线和默认边界；当前 D1 的代码、自动化、C92 部署、公网 HTTPS 和 Movie/Episode STRM Canary 已有验收证据，C92 真实 Movie 版本组样本已找到，客户端 `AlternateMediaSources` 字段修正、本地回归、两个真实 Item 的 API/source 对应以及 D2 多源 API 409 安全拒绝 Canary 已完成，但真实浏览器 UI source 点击及多源正向 Search/Fetch/Preview 仍待完成，详见 [D1 部署验收报告](../d1-deployment-acceptance.md)、[D2 多版本 MediaSources 实测记录](../d2-multisource-c92-sample.md) 和 [D2 多源真实 API Canary](../d2-multisource-c92-canary-acceptance-20260825.md)。[ADR-005](005-conditional-d2-entry-without-live-multisource.md) 将“进入单源 D2”和“宣称真实多源搜索支持”拆为两个门禁：允许 D2 契约、实现和单源 Canary 继续推进，多源请求在正向能力门禁前必须安全拒绝。D2 的详细契约和当前 D2-B1 后端状态见 [D2 搜索预览契约](../d2-search-preview-contract.md)；D3 及所有写能力的前置条件不变。

D3 代码已完成专用样本 Add 的本地契约、自动化验证和 C92 真实闭环，包含会话 CSRF、`subtitle:write`、allowlist、Artifact 绑定、原子非覆盖版本文件、Emby Refresh/轮询、history/quarantine、字幕流和客户端读取。真实验收结束后已恢复默认 `write_enabled=false`；Replace、Delete、Upload 或批量写入仍未开放。完整请求/恢复契约和证据见 [D3 专用样本 Add 契约](../d3-dedicated-add-contract.md) 与 [D3 C92 Canary 验收](../d3-c92-canary-acceptance-20260825.md)。
