# Core A/B ASS 与 UI 状态修复 C92 验收（2026-08-26）

## 范围

本次只修复并复测两项已在真实管理 UI 中发现的问题：

1. 合法 ASS 的 `Dialogue` 记录不按开始时间排列时，Upload 错误拒绝。
2. 搜索请求在详情重载时失效后，搜索按钮可能遗留 disabled 状态。

部署仅替换 C92 的 SubBridge 应用容器。未修改 Emby、SH、FRP、OpenResty、媒体目录全局权限或既有字幕文件。

## 实现

- ASS 仍校验编码、大小、字段、单条时间范围、控制字符和内容结构；不再要求 `Dialogue` 文件顺序递增。
- 原始 canonical ASS 内容不重写；仅供预览的 Cue 以稳定的开始时间顺序排列。
- 详情切换会使未完成的搜索请求失效，并显式复位搜索按钮；旧请求的 `finally` 不会影响新详情。

## 本地验证

- `go test -count=1 ./...` 通过。
- `go test -race -count=1 ./...` 通过（仅本进程临时加入已有 GCC 路径）。
- `go vet ./...`、`go build -trimpath ./cmd/server`、`scripts/verify.ps1`、`scripts/core-ab-ui-e2e.ps1`、`node --check internal/httpui/assets/app.js` 与 `git diff --check` 通过。

## C92 临时修复窗口

从候选工作树的已提交基线和四个已验证文件制作最小源码归档，构建带明确临时修复标识的应用镜像；该标识不是新的 Git 提交或不可变发布候选。

短时 daily 窗口中，应用容器 healthy、UID 10001、只读 rootfs、`/readyz` 均通过，远程搜索与写入按验收需要开启。用户完成以下真实 UI 复测：

- 原先被拒绝的非顺序 ASS 可 Upload 并生成预览。
- 搜索进行中重选当前媒体并重载详情后，搜索按钮恢复可点击。

此前同一受控窗口已由用户完成远程搜索、候选 Fetch、Preview、Add、Upload、Replace、可恢复 Delete、Restore 和实际播放器字幕显示；本报告不把这些既有操作扩展为未单独测试的媒体类型或多源 STRM 支持。

## 收尾

验收后以同一临时修复镜像的 closed Compose 重建应用。已复核：

- `remote_search_enabled=false`、`write_enabled=false`。
- `/media` 恢复只读，rootfs 只读，进程 UID 为 10001。
- 容器 healthy，`/readyz` 返回 200，未认证 `/v1/health` 返回 401。

测试产生的字幕按用户明确指示无需清理；未对既有字幕做清理操作。

## Knowledge Review

新的可复用结论：ASS 的事件行顺序不是语义正确性的可靠判据。解析器可以保留原始内容用于写入，同时对仅供预览的 Cue 做稳定排序；每一条 Cue 本身仍必须通过时间、文本和大小安全校验。

新的 UI 约束：任何以 request ID 作废旧异步请求的控件，都必须在新上下文初始化时复位其 busy/disabled 状态；不能依赖被作废请求的 `finally`。

当前证据只覆盖单源受控 UI 样本和用户实际操作，不构成普通本地媒体、多源 STRM 或新的正式不可变发布验收。
