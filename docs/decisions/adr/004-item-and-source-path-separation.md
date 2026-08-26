# ADR-004　Item 路径与 MediaSource 播放定位分离

- 状态　accepted
- 日期　2026-08-24
- 相关组件　MediaContext、PathMapper、PathGuard、字幕清单、HTTP 投影

## 背景

Emby Item 同时返回 `Path` 和一个或多个 `MediaSource.Path`。后者可能是本地文件路径，也可能是 HTTP 等播放定位符。STRM 的 MediaSource 定位符尤其不能被应用当作本地文件或目录使用。

## 决策

- STRM 以 `Item.Path` 的 `.strm` 扩展名判断。
- STRM 的 PathMapper、PathGuard、LocalDirectory 和 sidecar inventory 始终使用 `Item.Path`。
- 非 STRM 只有在选中的 `MediaSource.Path` 是本地路径时才使用它作为本地 inventory 路径；单源且 source path 缺失时，D1 只读 MediaContext 可以回退 `Item.Path` 并记录稳定 warning。D3 写入不得使用该只读 fallback，必须要求选中的 source path 本地、可映射且为现存普通文件。
- 远程 MediaSource.Path 只保留为内部上游事实，不映射、不访问、不输出、不写入日志；STRM 仍由 Item.Path 正常建立 inventory，只有缺少本地 inventory 锚点的非 STRM source 才返回降级状态。
- 多源可以共享同一个 Item.Path 目录，但 MediaStreams 必须保持选中 MediaSource 的 source-specific 语义，不能用 item-level streams 猜测补齐。

[ADR-009](009-strm-write-target-and-multisource-boundary.md) 进一步固定单源 STRM 的写入锚点和多源 STRM 的共享 sidecar 写入边界。

## 选择原因

这样可以避免把 STRM 内部播放定位符或远程 URL 当作本地文件系统目标，同时保留普通本地媒体的清单能力。路径选择、映射和字幕流选择分别有明确的输入，便于 Fake Emby 与真实 Canary 验证。

## 验证依据

- `internal/media` 对 STRM、普通本地媒体、远程 source、多源和空/非法 Item.Path 的单元测试
- `internal/httpapi` 的 Movie、Episode、多 MediaSource 只读测试
- `internal/inventory` 对外部字幕 URL、query、fragment 和安全 basename 的投影测试
- [当前架构](../../reference/architecture.md)
- [Phase 2 只读 Canary 验收定义](../../planning/phase2-readonly-canary.md)
