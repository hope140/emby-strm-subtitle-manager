# ADR-008　Core A/B 日常模式、显式 source 绑定与可恢复字幕操作

- 状态　accepted
- 日期　2026-08-25
- 相关组件　D2、D3、PreviewArtifact、MediaContext、Inventory、HTTP API、最小管理 UI、Compose 写入 overlay

## 背景

ADR-003 的 D3.1 只覆盖一个受 allowlist 约束的单源 Add Canary。Core A/B 需要在不扩大默认部署权限的前提下，把本地实现扩展为管理员可受控启用的日常流程，并正向支持一个 Item 的多个 MediaSource、独立 Item 版本结构，以及 Replace、Upload、可恢复 Delete 和 Restore。

这个扩展不能把 Item 标题、默认 source、客户端路径或历史媒体路径变成写入依据，也不能把本地 Fake Emby 证据写成真实 C92 验收。

## 问题

如何在保留默认关闭、认证、PathMapper、PathGuard、Artifact、锁和恢复边界的同时，让每一个读写操作始终精确绑定当前 Item 与选中的 source？

## 可选方案

1. 继续只支持 D3.1 的专用单源 Canary，把其余能力留给后续重构。
2. 按标题、默认 source 或一次性 Item 路径推断多版本目标。
3. 以统一 Item gate、每次重新解析的 Item/source、受限服务端 resolver 和私有恢复状态完成 Core A/B。

## 最终选择

选择方案 3，并固定以下规则。

1. `canary.enabled=true` 时保留 allowlist 与 generation 绑定；关闭 Canary 时使用允许有效 Movie/Episode 的日常 Item gate，并提供稳定 generation。模式变化和服务重启均使内存 Candidate/Artifact 失效。
2. Search 可在单 source Item 时省略 `media_source_id`；Fetch/Preview 只从已绑定 Token 取得 source。Upload、Add、Replace、Delete 与 Restore 在单源和多源时都要求明确且精确的 `media_source_id`。History 的最小查询按 Item 读取，但每条返回记录均携带不可变的 Item/source 绑定，UI 仅显示当前已选 source 的记录。每一步重新读取 Item，重新选择 source；缺失、错误、重复或变化的 source 一律安全拒绝。
3. Add 的目标 basename 只来自当前选中 source 已安全映射的媒体路径。Inventory 对 STRM 同时有界扫描 Item.Path 和当前 source 的安全路径；重复位置或不完整清单不允许修改。
4. Replace、Delete 与 Restore 只接收服务端 Inventory 签发的 opaque `subtitle_id`。它在 Item 锁内重新解析为私有路径事实，绝不从 HTTP 请求、history 公共字段或日志接收路径或文件名。
5. Replace 先核验新版本，再归档旧文件；Delete 只移动到私有 trash；Restore 从 archive/trash 重新核验 Hash，并以不覆盖方式恢复。恢复 history 只保存 `item`/`source` 目录类别，不保存媒体绝对路径，因而可在重新解析后回到原有安全目录。没有永久删除或批量写入接口。
6. Upload 只通过现有 Validator 生成已绑定、短期的 PreviewArtifact，忽略客户端文件名和 MIME 类型；Add/Replace 复用该 Artifact，不另建写入链，也不写持久 history。
7. 默认 `remote_search_enabled=false`、`write_enabled=false` 不变。日常写入使用单独的最小 Compose overlay，仍要求认证 scope、CSRF、PathMapper、PathGuard、私有目录与精确可写挂载；本 ADR 不授权部署、重启或改变 C92/SH/Emby。

## 选择原因

- 统一 gate 使 Canary 与日常模式共享 Token/Artifact 的失效语义，而不是以 `nil` 或特殊 generation 绕过绑定。
- 强制 source 重解析可覆盖一个 Item 多 source 与多个独立 Item 两种版本组织方式，不依赖 UI 排序或标题猜测。
- Item 锁、原子不覆盖写入、Hash、Refresh、history 与 quarantine 继续被所有写操作复用，减少独立写链产生的恢复差异。
- 恢复位置用私有类别表达，避免把媒体路径写入持久化状态，同时不把 source-specific STRM sidecar 错误恢复到另一目录。

## 已知代价

- 管理员必须先选择版本；多 source 不再可通过默认 source 或省略参数便利调用。
- 私有 archive、trash、history 与 quarantine 目录成为写能力的必要运行前置条件。
- 本地单元、Fake Emby 与浏览器 E2E 只证明实现和协作，不能替代 Movie、Episode、单源、多源、字幕流和实际客户端的真实 C92 综合验收。

## 后续影响

- D2/D3 的旧“单源/专用 Add”文档保留其历史 Canary 证据，但当前实现契约以本 ADR 和 [Core A/B 实现评审](../core-ab-implementation-review.md) 为准。
- 后续部署必须单独取得授权，并在真实环境重新验证目录权限、Refresh、MediaStreams、字幕流和客户端读取；完成后恢复默认 closed/只读边界。
- 批量、自动下载、定时扫描和永久清理仍不在本决策范围内。

## 验证依据

- `internal/d2`、`internal/d3`、`internal/inventory`、`internal/media`、`internal/preview`、`internal/httpapi` 与 `internal/httpui` 的单元和 Fake Emby 集成测试。
- 本地 `scripts/core-ab-ui-e2e.ps1`：真实浏览器的多源选择、Search→Fetch→Preview→Add、Upload→Preview、Delete/Restore 与 Replace/Restore。
- [Core A/B 连续实施计划](../core-ab-implementation-plan.md) 与 [Core A/B 实现评审](../core-ab-implementation-review.md)。
