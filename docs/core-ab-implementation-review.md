# Core A/B 实现评审

- 状态　本地实现、自动化与最小浏览器 E2E 已完成；真实 C92 综合验收未执行
- 日期　2026-08-25
- 基线　`main` 的 `b675758`
- 决策　[ADR-008](adr/008-core-ab-daily-source-bound-recovery.md)

## 交付范围

Core A 已完成管理员受控日常模式和显式 Item/source 绑定的 Search→Fetch→Preview→Add。仅 Search 在单 source Item 时可省略 source；Upload 和所有 D3 写入始终必须明确选择 source。Add 的目标 basename 来自所选 source 的安全映射路径，因而不依赖标题、默认 source 或 source 排序。

Core B 已完成 Upload→PreviewArtifact→Add/Replace、Replace、可恢复 Delete、History 和 Restore。所有写操作共用 Item 锁、Validator、Artifact 绑定、非覆盖原子写入、Hash、Refresh/可见性核验、history 与 quarantine。没有批量、自动下载或永久删除接口。

## 实现要点

- `preview.ItemGate` 统一 Canary allowlist 与日常模式 generation；Candidate/Artifact 在 Item、source、认证上下文与 generation 上绑定。
- D2 在 Search、Fetch、Preview 和 Upload 前重新读取 Item/source；Search 仅对单 source 自动选择，Fetch/Preview 只使用 Token 绑定，Upload 和 D3 写入缺失 source 一律拒绝，错误 source 返回 `media_source_mismatch`。
- `media.ResolveWriteTarget` 只接受当前 source 的安全映射路径。Inventory 对 STRM 同时有界检查 Item 与选中 source 的 sidecar 范围，服务端 resolver 仅把 opaque `subtitle_id` 在事务内映射为私有路径。
- Replace 先写入、Hash 回读、Refresh 并核验新字幕，再归档旧文件；后续核验或 history 失败时恢复旧文件并隔离新文件。每个补偿步骤都重新核验 Hash 与 Emby MediaStreams；补偿失败返回 `subtitle_rollback_failed`，保留 archive/trash/quarantine 并要求人工恢复。
- Delete 复制、fsync、Hash 核对并非覆盖提交到私有 trash 后才移除媒体副本；Restore 重新读取 Item/source、检查 Hash 与同名冲突，再不覆盖恢复。history 只保存 `item`/`source` 的恢复目录类别，不保存媒体路径，并以默认与最大 `limit` 限制查询结果。
- Upload 仅接收 `file`、`media_source_id`、`language`，限制 multipart 体积，忽略原文件名与 MIME 类型；Validator 只产出短期 PreviewArtifact，随后由 Add/Replace 消费，不记录持久 history。
- UI 只补 source 状态、Upload、Add、Replace、Delete、操作历史和 Restore。令牌/CSRF 仅留在页面内存，DOM、日志和响应不包含路径、原文件名、原始候选 ID、Token 或凭据。

## 自动化证据

- 单元测试覆盖日常 gate、Candidate/Artifact 的 Item/source 重绑定、多 source D2 正向流、source-specific Add、Replace 的归档/回滚、Delete/trash、Restore 冲突、操作 ID 与 Artifact Hash 冲突、恢复目录类别、history limit，以及 restore/remove/quarantine/rollback Refresh/history 的失败注入。
- `TestCoreABHTTPFakeEmbyDailyMultiSourceFlow` 覆盖 Fake Emby 的多 source Search→Fetch→Preview→Add、Upload 预览、Delete/Restore、Replace/Restore、Refresh 次数和日志/响应脱敏。
- `scripts/core-ab-ui-e2e.ps1` 在本地 loopback fixture 用真实浏览器覆盖 Version A 选择、Search→Fetch→Preview→Add、两次 Upload→Preview、Delete→Restore、Replace→Restore，并断言浏览器存储为空及 DOM 不含媒体路径或上传原文件名。
- 最终命令与结果见本次提交的验证记录；未把本地 Fake Emby 或浏览器流程表述为真实 C92/客户端验收。

## Knowledge Review

任务或阶段　Core A/B 连续本地实现与审核。

验证范围　Item gate、D2/D3、MediaContext、Inventory resolver、HTTP API、最小 UI、Compose 示例、单元测试、Fake Emby 集成、Playwright 浏览器 E2E、全包构建与文档检查。

### Knowledge Findings

- 新增约束　可恢复操作必须保存已验证的目录类别而不是媒体路径；Restore 只能在重新解析当前 Item/source 后恢复至 `item` 或 `source` 的安全目录。补偿不是 best-effort：恢复、移除、quarantine、Refresh 或 history 的补偿步骤均须重新核对 Hash 与 MediaStreams；任一步失败返回 `subtitle_rollback_failed`，且保留私有恢复副本供人工恢复。
- 隐蔽坑　STRM Item.Path 与选中 MediaSource.Path 可以有不同 basename 或目录。只按 Add 的 source 目录 Restore 会把原有 sidecar 放错位置；Inventory 必须保留两种安全扫描范围并在发现重复位置时拒绝修改。
- 被证明错误的假设　现有单源 Canary 的 `nil`/allowlist 特殊分支不能安全扩展到日常模式；统一 gate generation 才能持续约束 Candidate/Artifact 与写操作。
- 建议沉淀项　交互式文件上传 E2E 应使用 Playwright 的 `setInputFiles` 绑定本地测试字幕，并把每个确认动作拆成一次性 dialog handler，避免重复 handler 污染后续操作。

### 证据

- 代码　`internal/preview`、`internal/d2`、`internal/d3`、`internal/media`、`internal/inventory`、`internal/httpapi`、`internal/httpui`、`cmd/server` 和 `deploy`。
- 测试　相关包单元测试、Fake Emby HTTP 集成、`scripts/core-ab-ui-e2e.ps1`、全包测试、vet、build、Node 语法和差异检查。
- 实际运行、日志或可复现结果　本地 loopback Fake Emby 与浏览器 E2E 已运行；没有访问 C92/SH/Emby，没有部署、重启或真实客户端验收。

### 去重检查

- 已搜索的文档和关键词　`AGENTS.md`、`architecture.md`、`lessons-learned.md`、`adr/`、`Core A`、`Core B`、`MediaSource`、`STRM`、`Restore`、`history`、`quarantine`、`Upload`、`subtitle_rollback_failed`、`limit`。
- 是否更新已有结论　是；同步单源 source 契约、Upload/history 边界、History limit 和可核验补偿契约到实施计划、架构、ADR、实现评审与维护经验。

### 分流判断

- `docs/lessons-learned.md`　更新
- `docs/architecture.md`　更新
- `docs/adr/`　更新 ADR-008
- `LOCAL_OPERATIONS.md`　不需要；本任务没有新的长期本机拓扑或恢复步骤

### 未验证范围与残余风险

- 未部署或连接 C92、SH 或 Emby；真实 Movie、Episode、单源、多 source、字幕流和实际客户端综合验收仍需独立授权后完成。
- 本轮没有 `go test -race` 通过证据：默认 `CGO_ENABLED=0`，单次启用后确认当前 Windows `PATH` 中没有 `gcc`。这不是源码测试失败；应在具备 C 工具链的 Linux CI 中运行 race 检测。
- archive/trash 的保留期清理、批量、自动下载、定时扫描、评分和永久删除均未实现。
