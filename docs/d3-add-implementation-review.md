# D3 Add 实现评审

Knowledge Review
任务或阶段　D3 专用样本 Add 本地实现与 C92 真实验收评审
验证范围　D3 配置、认证门禁、Artifact 绑定、单源 MediaContext、原子非覆盖写入、Emby Refresh/轮询、history/quarantine、HTTP/UI 接线、Fake Emby 测试及 C92 真实闭环

## Knowledge Findings

- 新增约束　D3 只能作用于服务端 allowlist 中的单一 Movie/Episode；基础 Compose 和默认配置继续关闭写能力，D3 需要独立可写 overlay。
- 隐蔽坑　写入文件存在、Refresh 返回成功或应用响应成功都不足以完成验收，必须继续核对 Emby MediaStreams、字幕流和客户端读取；历史记录只用于幂等恢复，不替代内容 Hash 校验。
- 隐蔽坑　Compose overlay 的 `/media:rw` 不会改变媒体目录本身的 POSIX 权限；目标目录必须临时允许 UID `10001:10001` 写入，且验收后要恢复原属主和只读挂载。
- 被证明错误的假设　不能把 D2 预览 Artifact 当作任意文件读取接口，也不能从多 MediaSource 中猜测写入目标。
- 建议沉淀项　D3 的真实验收固定采用“专用 allowlist → 单源绑定 → 原子版本文件 → Refresh/轮询 → Hash/字幕流/客户端读取 → 关闭写能力”的证据链。

## 证据

- 代码　`internal/d3`、`internal/auth`、`internal/config`、`internal/preview`、`internal/embyclient`、`internal/httpapi`、`internal/httpui` 和 `cmd/server`。
- 测试　D3 Add 原子写入、幂等重放、Refresh 失败隔离、配置门禁、HTTP CSRF/scope、Emby Refresh 请求及全包测试。
- 实际运行、日志或可复现结果　真实 C92 Add、Emby 字幕流、应用字幕清单、Emby Web 客户端读取，以及环境负责人补充的手机端实际播放确认，已在 [D3 C92 Canary 验收](d3-c92-canary-acceptance-20260825.md) 中单独记录；D3 写入窗口随后已恢复为 closed。

## 去重检查

- 已搜索的文档和关键词　`AGENTS.md`、`docs/architecture.md`、`docs/lessons-learned.md`、`docs/adr/`、`D3`、`CSRF`、`allowlist`、`Refresh`、`quarantine`、`subtitle:write`。
- 是否更新已有结论　是；架构、ADR-003、ADR-006、README、文档索引和 lessons learned 已补充 D3 当前边界。

## 分流判断

- `docs/lessons-learned.md`　更新，记录 D3 证据链和恢复边界。
- `docs/architecture.md`　更新，记录当前实现和默认关闭状态。
- `docs/adr/`　更新 ADR-003、ADR-006；本轮无需新增 ADR。
- `LOCAL_OPERATIONS.md`　已更新 C92 版本化回滚点、D3 专用目录和恢复步骤，不记录凭据、媒体名称或 Item ID。

## 未验证范围与残余风险

- Replace、Delete、Upload、批量写入和多源正向写入不属于 D3。
- C92 Docker、宿主权限、网络路径和关闭后的只读边界已经在验收报告中核对；后续部署仍不能由本地 Go 测试替代。
