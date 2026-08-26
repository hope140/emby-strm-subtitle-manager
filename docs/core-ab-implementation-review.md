# Core A/B 实现评审

- 状态　本地实现、自动化与最小浏览器 E2E 已完成；单源 STRM 的真实 C92 服务端闭环已通过，其他真实综合范围仍未执行
- 日期　2026-08-26
- 公共基线　`main` 的 `947d847bb8ee620fc0362081fdff981069472081`
- Core A/B 计划起点　`b675758d6e876e1e79e5bfcb74d5d7bbb226f830`（仅指实施计划文档起点，不是当前 `main` 基线）
- 决策　[ADR-008](adr/008-core-ab-daily-source-bound-recovery.md)、[ADR-009](adr/009-strm-write-target-and-multisource-boundary.md)

## 交付范围

Core A 已完成管理员受控日常模式和显式 Item/source 绑定的 Search→Fetch→Preview→Add。仅 Search 在单 source Item 时可省略 source；Upload 和所有 D3 写入始终必须明确选择 source。写入锚点按媒体类别分流：单源 STRM 使用 Item.Path 映射后的 `.strm` basename，普通本地媒体使用所选 source 的安全映射路径，因而不依赖标题、默认 source 或 source 排序；多源 STRM 写入安全拒绝。

Core B 已完成 Upload→PreviewArtifact→Add/Replace、Replace、可恢复 Delete、History 和 Restore。所有写操作共用 Item 锁、Validator、Artifact 绑定、非覆盖原子写入、Hash、Refresh/可见性核验、history 与 quarantine。没有批量、自动下载或永久删除接口。

## 实现要点

- `preview.ItemGate` 统一 Canary allowlist 与日常模式 generation；Candidate/Artifact 在 Item、source、认证上下文与 generation 上绑定。
- D2 在 Search、Fetch、Preview 和 Upload 前重新读取 Item/source；Search 仅对单 source 自动选择，Fetch/Preview 只使用 Token 绑定，Upload 和 D3 写入缺失 source 一律拒绝，错误 source 返回 `media_source_mismatch`。
- `media.ResolveWriteTarget` 对单源 STRM 使用 Item.Path，对普通本地媒体使用当前 source 的安全映射路径，并要求映射锚点是现存普通文件。Inventory 对普通本地媒体保留受控 Item/source 范围；多源 STRM 只扫描共享 Item 目录并把 sidecar 标记为不可管理。服务端 resolver 仅把 opaque `subtitle_id` 在事务内映射为私有路径。
- Replace 先写入、Hash 回读、Refresh 并核验新字幕，再归档旧文件；后续核验或 history 失败时恢复旧文件并隔离新文件。每个补偿步骤都重新核验 Hash 与 Emby MediaStreams；补偿失败返回 `subtitle_rollback_failed`，保留 archive/trash/quarantine 并要求人工恢复。
- Delete 复制、fsync、Hash 核对并非覆盖提交到私有 trash 后才移除媒体副本；Restore 重新读取 Item/source、检查 Hash 与同名冲突，再不覆盖恢复。history 只保存 `item`/`source` 的恢复目录类别，不保存媒体路径，并以默认与最大 `limit` 限制查询结果。
- Upload 仅接收 `file`、`media_source_id`、`language`，限制 multipart 体积，忽略原文件名与 MIME 类型；Validator 只产出短期 PreviewArtifact，随后由 Add/Replace 消费，不记录持久 history。
- UI 只补 source 状态、Upload、Add、Replace、Delete、操作历史和 Restore。媒体详情公开不含路径的 `write_capabilities`；多源 STRM 的 D3 控件按 `strm_multisource_write_unsupported` 隐藏/禁用，旧 source history 按当前 Item/source 标注 `strm_history_location_unsupported`。令牌/CSRF 仅留在页面内存，DOM、日志和响应不包含路径、原文件名、原始候选 ID、Token 或凭据。

## 自动化证据

- 单元测试覆盖日常 gate、Candidate/Artifact 的 Item/source 重绑定、多 source D2 正向流、普通本地 source-specific Add、单源 STRM 远程 source 写入、STRM/普通媒体锚点文件类型、共享 sidecar 只读、多源 STRM 四类写操作 409、Replace 的归档/回滚、Delete/trash、Restore 冲突、旧 STRM source history 拒绝、操作 ID 与 Artifact Hash 冲突、恢复目录类别、history limit，以及 restore/remove/quarantine/rollback Refresh/history 的失败注入。成功补偿后同一 `operation_id` 的 Replace/Delete 重试仅复用 Hash 一致的恢复材料；Hash 不一致保留冲突。
- `TestCoreABHTTPFakeEmbyDailyMultiSourceFlow` 覆盖普通本地 Fake Emby 的多 source Search→Fetch→Preview→Add、Upload 预览、Delete/Restore、Replace/Restore、Refresh 次数和日志/响应脱敏；`TestCoreABHTTPSingleSourceSTRMFullFlow` 覆盖真实形态远程 source 的单源 STRM Search→Fetch→Preview→Add、Upload→Preview、Delete/Restore、Replace/Restore、history capability、日志脱敏和无路径输出；`TestCoreABHTTPRejectsAllMultiSourceSTRMWrites` 覆盖多源 STRM Add/Replace/Delete/Restore 路由级 409 与无媒体变更。
- `scripts/core-ab-ui-e2e.ps1` 已由真实浏览器实际运行通过：多源 STRM 仍可 Search→Fetch→Preview，但 Add、Replace、Delete、Restore 控件均未提供；随后单源 STRM fixture（唯一远程 source、Item.Path 为本地 `.strm`）覆盖 Search→Fetch→Preview→Add、两次 Upload→Preview、Delete→Restore、Replace→Restore，并断言浏览器存储为空及 DOM 不含媒体路径或上传原文件名。
- 本次实际验证：`go test -count=1 ./...`、`go test -race ./...`、`go vet ./...`、`go build -trimpath ./cmd/server`、`scripts/verify.ps1`、`node --check`、`git diff --check` 和真实 `scripts/core-ab-ui-e2e.ps1` 均通过。Markdown 45 份文件的 UTF-8、代码围栏、尾随空白和相对链接检查通过；其中仓库既有的 `docs/d2-c-c92-canary-preflight.md` → `../LOCAL_OPERATIONS.md` 私有本机文档链接按约定跳过，未新增或修复该本机文件。新增差异已做凭据、Token、Cookie、认证 URL 和路径输出审查。未把本地 Fake Emby 或浏览器流程表述为真实 C92/客户端验收。

## Knowledge Review

任务或阶段　SubBridge STRM 写入目标独立审查后的 P1/P2/P3 修复与本地验收。

验证范围　Item gate、D2/D3、MediaContext、Inventory resolver、history location、Restore preflight、HTTP API、公开写能力投影、最小 UI、单元测试、Fake Emby 集成、Playwright 浏览器 E2E、全包构建与文档检查。

### Knowledge Findings

- 新增约束　可恢复操作必须保存已验证的目录类别而不是媒体路径；类别直接来自最终 `WriteTarget.Location`，不能通过相同目录反推。Restore 必须在认证、gate、Item 类型、source 唯一性和绑定通过后重读当前 Item/source，再恢复至 `item` 或 `source` 的安全目录；旧 STRM `source` history 在 Item.Path 坏锚点检查前稳定返回 `strm_history_location_unsupported`。
- 新增约束　按 source 查询 history 必须先按显式 `MediaSourceID` 过滤，再排序并应用请求的 `limit`；不能先截断同一 Item 的跨 source history，否则其他 source 的新记录会遮蔽当前 source 的有效 Replace/Delete 恢复记录。
- 隐蔽坑　STRM Item.Path 与选中 MediaSource.Path 可以有不同 basename 或目录，且 source path 可能是远程播放 URL；旧 UI 只看全局写开关会把多源 STRM Add/Restore 暴露到最后一步。单源 STRM 写入和恢复必须坚持 Item.Path；多源 STRM 的共享 Item sidecar 不能按 source 生成可管理对象，公开详情必须提供不含路径的写能力和历史恢复原因。
- 被证明错误的假设　把本地 `Version-A/B.mkv` source 与一个实际只创建的 `.strm` 文件放在同一浏览器 fixture 中，不能证明多源 STRM 正向写入；把 Item/source 目录相等当作 `item` history 类别也会错误记录普通本地媒体。
- 建议沉淀项　交互式文件上传 E2E 应使用 Playwright 的 `setInputFiles` 绑定本地测试字幕，并把每个确认动作拆成一次性 dialog handler；多源 STRM E2E 只验证 D2 保持可用和 D3 控件/稳定错误边界。

### 证据

- 代码　`internal/preview`、`internal/d2`、`internal/d3`、`internal/media`、`internal/inventory`、`internal/httpapi`、`internal/httpui`、`cmd/server` 和 `deploy`。
- 测试　`internal/media`、`internal/d3`（含 `TestHistoryListForSourceFiltersBeforeLimit`）、`internal/httpapi`、`internal/httpui` 定向回归、Fake Emby HTTP 集成、全包 Go、race、vet、build、`scripts/verify.ps1` 和真实 `scripts/core-ab-ui-e2e.ps1` 均通过；Node 语法、Markdown/敏感信息和差异检查也已完成。
- 实际运行、日志或可复现结果　本地 loopback Fake Emby 与真实 Playwright 浏览器 E2E 已运行。本评审完成后，候选提交另经授权在 C92 完成单源 STRM 的 Upload/Add/Replace/Delete/Restore、MediaStreams、官方字幕流、真实管理 UI Provider 链路和实际播放器读取，并恢复 closed；详见 [C92 单源 STRM 正式验收](core-ab-c92-acceptance-20260826.md) 与 [ASS/UI 修复 C92 验收](core-ab-ass-ui-fix-c92-acceptance-20260826.md)。该运行不包含普通本地媒体或多源 STRM 正向验收。

### 去重检查

- 已搜索的文档和关键词　`AGENTS.md`、`architecture.md`、`lessons-learned.md`、`adr/`、`Core A`、`Core B`、`MediaSource`、`STRM`、`ResolveWriteTarget`、`strm_multisource_write_unsupported`、`OriginalLocation`、`Restore`、`history`、`quarantine`、`Upload`、`subtitle_rollback_failed`、`limit`、`operation_id`、`OpenResty`、`route`。
- 是否更新已有结论　是；同步 STRM/普通媒体写入锚点、多源 STRM 只读 Inventory、稳定 409、旧 history 兼容边界、Upload/history 边界、History limit、可核验补偿和补偿后 operation ID 重试到 ADR、架构、实施计划、实现评审和维护经验。

### 分流判断

- `docs/lessons-learned.md`　更新
- `docs/architecture.md`　更新
- `docs/adr/`　更新 ADR-008
- `LOCAL_OPERATIONS.md`　不需要；本任务没有新的长期本机拓扑或恢复步骤
- `ADR-009`　新增；记录 STRM 写入锚点、多源共享 sidecar 和旧 history 恢复边界。

### 未验证范围与残余风险

- 单源 STRM 的 C92 服务端、真实管理 UI Provider 链路和实际播放器读取已完成；普通本地 Movie/Episode 的真实正向流程与多源 STRM 的 C92 D3 409 仍需独立授权后完成。
- 本机 MSYS2 GCC 位于 `C:\msys64\ucrt64\bin`；仅在测试进程中把该目录临时加入 `PATH` 后，`go test -race ./...` 已通过，未修改系统环境变量。OpenResty/nginx 二进制未安装，因此本轮只完成模板的标签与脱敏静态检查；真实入口配置解析仍需在独立部署授权中执行。
- archive/trash 的保留期清理、批量、自动下载、定时扫描、评分和永久删除均未实现。
