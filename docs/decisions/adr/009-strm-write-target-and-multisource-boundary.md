# ADR-009　STRM 写入锚点与多源写入边界

- 状态　accepted
- 日期　2026-08-25
- 相关组件　MediaContext、PathMapper、PathGuard、Inventory、D3 Add/Replace/Delete/Restore、HTTP API

## 背景

真实 STRM Item 的 `Item.Path` 是 Emby 媒体库中的本地 `.strm` 文件，选中的 `MediaSource.Path` 可能只是远程播放 URL。外挂字幕应与 `.strm` 文件相邻，文件名基于 `.strm` basename；远程 source path 不能作为本地目录或字幕文件名。此前 Core A/B 的写入 resolver 无条件使用 source path，导致真实 STRM 写入在文件操作前被错误拒绝。

同一个 STRM Item 的多个 MediaSource 还可能共享 Item 目录中的 sidecar。没有真实 Emby 证据证明该 sidecar 如何与每个 source 关联以前，按 source 生成多个可管理记录会允许一个 source 的操作修改另一个 source 的共享文件。

## 问题

如何在保留显式 source 绑定、PathMapper/PathGuard、原子写入和可恢复 history 的前提下，修复单源 STRM 写入，同时避免把多源共享 sidecar 当成 source-specific 文件管理？

## 最终选择

1. 所有 D3 写请求继续要求非空 `media_source_id`。服务端每次重新读取 Item，校验 Movie/Episode、source 结构、source 数量以及请求 ID 与当前 source 的精确匹配。
2. 单源 STRM 使用 `Item.Path` 作为写入锚点。该路径必须通过 PathMapper 和 PathGuard，映射后的 `.strm` 必须是现存普通文件；symlink、目录、设备文件、控制字符、未映射路径和缺失文件均拒绝。写入目录是 `.strm` 所在目录，字幕 basename 来自 `.strm` basename。source ID 只用于请求绑定、Artifact/Item 校验、Refresh 和 MediaStreams 核验。
3. 普通本地媒体使用当前显式 source 的 `MediaSource.Path`。该路径必须是本地路径、通过 PathMapper/PathGuard 并映射为现存普通文件。远程 source 不回退到 `Item.Path`。
4. 多源 STRM 的 Add、Replace、Delete、Restore 使用明确选择的 `media_source_id` 进行绑定、Refresh 和字幕流验证；本地写入锚点仍是 Item.Path。Emby 已实测将写入结果绑定到所选版本，不再因版本组拒绝写入。
5. 多源 STRM Inventory 只扫描 Item 目录，不扫描选中 source 的目录。共享 Item sidecar 可以只读展示，但 `manageable=false`，并使用不随 source 改变的共享 opaque 身份；Replace/Delete resolver 对其 fail closed。
6. 新 Replace/Delete history 保存 `OriginalLocation=item`（单源 STRM）或 `OriginalLocation=source`（普通本地媒体），类别必须直接来自最终 `WriteTarget.Location`，不能通过 Item/source 目录相等关系反推；只保存类别和安全 basename，不保存媒体绝对路径。当前 Item 重新识别为 STRM 时，Restore 先在认证、gate、Item 类型、source 唯一性和 source 绑定通过后判定 STRM 及 history 类别，再对旧 v2 `OriginalLocation=source` 返回 `409 strm_history_location_unsupported`，不受 Item.Path 未映射、缺失、目录或 symlink 影响，也不读取恢复副本或写入媒体。
7. 媒体公开投影只提供不含路径的 `write_capabilities`。多源 STRM 的 Add、Replace、Delete、Restore 控件与单源一致，但必须已有明确选中的 source；History 按当前选中 Item/source 返回安全的 Restore 能力。旧 history 的 `OriginalLocation=source` 仍返回 `strm_history_location_unsupported`，因为该旧记录无法证明原始目录。

本 ADR 修订 [ADR-008](008-core-ab-daily-source-bound-recovery.md) 中“Add 的目标 basename 只来自 source path”和多源 STRM 允许 source-specific sidecar 的部分规则；[ADR-004](004-item-and-source-path-separation.md) 关于 STRM 的 Item.Path 锚点和远程 source 仅作为播放定位符的决定继续有效。

## 选择原因

- Item.Path 是 STRM sidecar 的真实本地入口，能支持当前 C92 的单源部署形态；source path 仍然保留为 Emby source 绑定和结果核验事实。
- 普通本地多源媒体必须按 source path 命名，避免把不同版本写入同一个 basename。
- 多源 STRM 共享 sidecar 的关联关系尚未由真实 Emby 证据确认，读显示与写管理分离可以保留可见性而不扩大写入权限。
- history 只保存类别，Restore 必须重新解析当前 Item/source；对旧 STRM source history 直接拒绝比猜测目录更安全。

## 已知代价

- 多版本 STRM 必须明确选择 source；本地 `.strm` 文件仍必须存在并位于允许的映射目录，部署前需要单独完成权限和文件类型预检。
- 本地测试和 Fake Emby 只证明代码契约；它们不能替代 C92 Canary、Emby MediaStreams、官方字幕流或实际客户端读取。

## 验证依据

- `internal/media`：STRM 远程 source、普通本地 source、缺失/目录/symlink 锚点和多版本显式 source 绑定测试。
- `internal/inventory`：STRM 单一扫描范围与跨 source 身份测试。
- `internal/d3`：STRM Add/Replace/Delete/Restore、`OriginalLocation=item`、显式 source 绑定与旧 history 拒绝测试。
- `internal/httpapi`：普通本地多源 Fake Emby 正向流程、STRM Search/Fetch/Preview/Add/Upload/Replace/Delete/Restore 与多版本选中 source 写入测试。
- `internal/httpui` 与 `scripts/core-ab-ui-e2e.ps1`：公开能力投影、显式版本选择和 STRM 完整浏览器流程测试。
- `go test`、`go vet`、`go build` 与 `scripts/verify.ps1` 的结果以本次任务交付记录为准；C92、Docker、真实 Provider 和客户端范围仍需独立授权与实时验收。
