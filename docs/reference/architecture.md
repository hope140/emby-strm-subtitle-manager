# SubBridge 当前架构

本文只描述截至 2026 年 8 月 26 日已经由当前源码、自动化检查或真实运行确认的内容；完成度和后续优先级统一见 [当前状态与后续路线图](../planning/current-status-and-roadmap.md)。D1 的只读切片与 C92 部署、D2 单源后端真实 API Canary、D2.5 管理员认证和 D3.1 专用单源 Add 已通过相应验收。Core A/B 已在本地完成日常 gate、普通本地媒体正向多 source Search→Fetch→Preview→Add、单源 STRM Item.Path 写入、Upload、Replace、可恢复 Delete、History 与 Restore，并通过单元、Fake Emby 与最小浏览器 E2E；多源 STRM 写入保持安全拒绝。2026-08-25 的旧 source-bound C92 尝试在媒体操作前阻断并恢复 closed/只读；随后候选提交 `5deaf519f69ba1226840836516c07124965a4afc` 已在 C92 通过受控的**单源 STRM 服务端闭环**验收。该结论不扩展为普通本地媒体、多源 STRM、真实 Provider 或本次新的客户端播放验收，细节见 [2026-08-26 Core A/B C92 单源 STRM 正式验收](../records/acceptance/core-ab-c92-acceptance-20260826.md)。

当前品牌、GitHub 仓库、Go module、构建二进制和新安装 Compose 示例统一使用 `SubBridge`/`subbridge`。已经验收的 C92 Compose project、镜像、容器、目录和 FRP proxy 保留旧技术标识，直到后续获得有功能收益的部署授权；历史报告不追溯改写当时的资源名称。

## D1 本地实现

当前代码由 `cmd/server` 和以下内部模块组成：`config`（非密配置、服务端文件凭据与管理员 environment 校验）、`domain`（Emby 领域 DTO）、`embyclient`（D1/D2 只读调用与 D3 有界 Refresh）、`media`（MediaContext 与 MediaSource 选择）、`pathmap`（跨平台路径映射和目录安全边界）、`inventory`（字幕清单与私有 resolver）、`preview`（Candidate/Artifact 与统一 Item gate）、`d3`（Add、Replace、Delete、Restore 的原子写入与恢复事务）、`httpui`（内嵌管理页面）以及 `httpapi`（认证、读写和历史路由）。

服务还提供同源的内嵌 D1.5 只读 UI，并公开 7 个 GET API 路由，3 个运维路由和 4 个业务路由：

```text
UI：  GET /
      GET /assets/{asset}
运维：GET /livez
      GET /readyz
      GET /v1/health
业务：GET /v1/emby/libraries
      GET /v1/emby/items
      GET /v1/media/{itemId}
      GET /v1/media/{itemId}/subtitles
```

`/livez` 只表示进程存活；`/readyz` 会对 Emby 发起受超时和缓存控制的真实只读探测；`/v1/health` 返回版本、功能开关和当前 Emby readiness 状态。业务路由始终由服务端使用 ItemID 重新查询 Emby。响应只投影展示字段，不暴露 Emby 绝对路径、字幕正文、认证参数或 STRM 内部地址。

内嵌 UI 保留既有媒体库、Movie/Episode 混合分页、媒体详情和字幕清单布局。Core A/B 仅补选中 source 状态、Search/Fetch/Preview/Add、Upload、Replace、Delete、History 与 Restore 入口；不会重构媒体库层级、设置页、日志页或整体视觉。发布版 UI 使用管理员用户名和密码登录，服务端签发短期 HttpOnly 会话 Cookie；密码和 CSRF Token 不进入 JavaScript 持久化存储，刷新后 UI 回到登录界面。CLI、定时任务和 CI 使用独立 Bearer Token。UI、静态资源和 API 必须保持同源。访问方式和公网 HTTP/HTTPS 边界见 [D1.5 最小只读 Web UI](../records/reviews/d1.5-readonly-ui.md)，认证细节见 [D2.5 管理员认证](../records/reviews/d2.5-admin-auth.md)。

应用 API Key 与独立的 identity secret 分离。identity secret 由 Inventory 用于生成稳定、不可逆的本地字幕标识，不能替代或复用 Emby API Key，也不会进入响应和普通日志。

管理 API 使用独立的 `security.api_auth_token_file` Bearer Token，发布版 UI 使用私有 Compose environment 中的 `APP_ADMIN_USERNAME`、`APP_ADMIN_PASSWORD`。两个变量缺失或非法时服务启动失败，不回退到 Bearer-only UI。`POST /v1/auth/login` 只接受无 query 的 JSON 用户名和密码，成功后签发 `HttpOnly`、`SameSite=Lax`、固定 TTL 的内存会话和仅留在页面内存的 CSRF Token；会话服务重启即失效。`/livez` 与只返回极小状态的 `/readyz` 保持公开；除登录路由外的所有 `/v1/*` 路由均接受有效管理员会话或 `Authorization: Bearer <token>`。缺失或错误统一返回 401、`WWW-Authenticate: Bearer` 和脱敏错误 envelope。Bearer 不接受 query 参数，也不写入日志或响应，并且不能复用 Emby API Key、identity secret 或管理员密码。Bearer scope 由 `security.api_auth_scopes` 控制，媒体路由需要 `media:read`，Search 需要 `subtitle:search`，Fetch/Preview/Upload 需要 `subtitle:preview`，Add/Replace/Delete/History/Restore 需要独立的 `subtitle:write`；缺少 scope 返回 403。写入只有在 `write_enabled=true`、远程搜索开关、统一 Item gate、私有目录和可写 Compose overlay 同时满足时才注册；管理员浏览器的 Upload 以及所有 D3 写入请求还必须通过 CSRF/同源校验。

MediaContext 对单源自动选择，对多源要求显式 `media_source_id`，不会猜测列表第一项。STRM 的 Inventory 和 PathMapper 始终使用 Emby Item.Path；非 STRM 的 D3 写入以显式选中的本地 MediaSource.Path 为锚点，D1 只读模型在 source path 缺失时保留受限 fallback，远程 source path 只作为内部播放定位事实，不参与本地映射、目录检查、写入、响应或日志。单源 STRM 的 D3 Add、Replace、Delete、Restore 通过 `Item.Path` 映射到现存普通 `.strm` 文件，并以该文件的目录和 basename 写入；普通本地媒体继续使用所选 source path，远程 source 不回退到 Item.Path。多源 STRM 写入稳定返回 `409 strm_multisource_write_unsupported`；Inventory 只扫描共享 Item 目录，将 sidecar 标记为不可管理，不把它们按 source 目录写入。STRM 的 IsStrm 判断只看 Item.Path。PathMapper 支持 POSIX、Windows drive 和 UNC 形式，采用规范化、最长前缀匹配及目录 containment 检查；路径不安全、未映射、缺失或非普通文件时返回降级状态和稳定 warning。公开 MediaDTO 只返回不含路径的 `write_capabilities`；多源 STRM 的 D3 控件按能力隐藏或禁用，旧 source history 的 Restore 能力按当前 Item/source 返回稳定原因。Inventory 只读取文件元数据；D3 resolver 在锁内有界读取并校验目标字幕，绝不读取 STRM 内容或媒体正文。

本地 `scripts/verify.ps1` 和 Linux 全包测试均已通过且无 skip；C92 的 Docker Compose schema/build、启动安全边界、`/readyz`、Bearer 认证和版本标签，以及 FRP 公网 HTTPS、单代理加密和公网应用端口防火墙边界另有部署证据。C92 真实版本组样本已补齐，客户端已固定请求 `AlternateMediaSources` 并通过本地回归测试；真实 API/source 对应和 D2 多源安全拒绝 Canary 已完成，但浏览器 UI source 点击和多源正向支持仍未完成，因此不能支撑真实多源搜索的支持声明；该缺口不阻断单源 D2。D3 C92 专用样本 Add 的 Docker、宿主目录权限、Hash、Refresh、字幕流、客户端读取和 closed 回滚另见 [D3 C92 Canary 验收](../records/acceptance/d3-c92-canary-acceptance-20260825.md)。Core A/B 的旧 source-bound 阻断见 [2026-08-25 综合部署验收](../records/acceptance/core-ab-c92-acceptance.md)；修复后单源 STRM 的 Upload/Add/Replace/Delete/Restore、MediaStreams、官方字幕流和 closed 回滚见 [2026-08-26 单源 STRM 正式验收](../records/acceptance/core-ab-c92-acceptance-20260826.md)。

## 当前系统边界

```text
115 / CD2
   │
   ▼
CMS 整理目录和生成 STRM
   │
   ▼
Emby 4.9.5.0
   ├─ 媒体索引与播放
   ├─ Remote Subtitle API
   └─ 外部字幕识别与字幕流
          │
          ├─ MeiamSub.Assrt 1.0.16.0
          └─ MeiamSub.Thunder 1.0.16.0
```

版本号来自 Gate 0 的当日实测。后续任务必须重新检查当前版本，不能把本节当作实时状态。

## 已验证的远程字幕数据流

```text
服务端 API Key
   │
   ▼
GET /Items/{Id}/RemoteSearch/Subtitles/{Language}
   │
   ▼
RemoteSubtitleInfo[]
   │
   ▼
GET /Providers/Subtitles/Subtitles/{Id}
   │
   ▼
字幕字节流
```

独立 API Key 请求已经完成 Search 和 Fetch。请求不需要浏览器 Cookie 或登录会话。

搜索请求无法指定单次调用使用哪个 Provider，也无法传入自定义搜索词。额外的 `ProviderName` 和 `SearchTerm` 参数在 Gate 0 中没有改变候选结果。

## Provider 行为

MeiamSub.Assrt 和 MeiamSub.Thunder 都能通过 Emby 返回可用候选。Thunder 搜索结果中可能包含已经失效的上游下载地址。

Gate 0.1 的第一个 Thunder 候选在 Fetch 时遇到上游 HTTP 404。Meiam 将其包装为 `InvalidDataException`，Emby 对外返回 HTTP 500。同次搜索的第二和第三个候选均成功返回有效 SRT。

V1 因此采用候选级失败模型。上游 4xx 和内容无效直接标记候选失败，临时网络错误最多重试一次，其他候选继续可用。

## STRM 边界

Gate 0 在受控搜索与 Fetch 期间监测已知媒体代理端口，没有观察到访问 STRM 内部媒体 URL 的流量。

当前 Meiam Thunder 源码会对可读的 MediaPath 尝试计算 CID。Gate 0 没有执行进程级文件打开追踪，因此 STRM stub 读取属于源码确认的兼容行为，不属于本次 live trace 结果。

未来应用自身不得读取 STRM 内容或计算 CID。是否维护 Meiam `.strm` 跳过补丁留作兼容性决定。

## 外部字幕写入与读取

Gate 0 已经验证 Emby 能把成功 Fetch 的 Thunder 候选写入 STRM 同目录，并识别为外部 SRT。通过下面的字幕流接口读取时，内容与文件系统字幕归一化后一致。

```text
/Videos/{Id}/{MediaSourceId}/Subtitles/{Index}/Stream.{Format}
```

本次同路径替换测试显式使用 `Cache-Control: no-cache`，只证明该请求范围内没有返回旧字幕。V1 继续采用新版本文件名写入、验证成功后归档旧文件的策略。

## V1 选择与代码路线

## Core A/B 字幕操作

Core A/B 的当前实现由 [ADR-008](../decisions/adr/008-core-ab-daily-source-bound-recovery.md) 和 [ADR-009](../decisions/adr/009-strm-write-target-and-multisource-boundary.md) 固定。D2 Search、Fetch、Preview、Upload 与 D3 Add、Replace、Delete、History、Restore 共同使用显式 Item/source 绑定、统一 Item gate、管理员认证、scope/CSRF、Artifact、PathMapper/PathGuard 和 Item 锁。仅 Search 可在单 source 时省略 source；Fetch/Preview 只从绑定 Token 取得 source；Upload 和所有 D3 写入无论 source 数量都必须显式选择精确 source。D3 只从 Inventory resolver 接收当前可管理字幕的 opaque ID，先验证 Hash 与文件状态，再进行非覆盖原子提交、Refresh/轮询、history 和 quarantine。

Replace 仅在新版本可见后才归档旧版本，后续失败时恢复旧版本并隔离新版本。单源 STRM 的新版本目标来自 Item.Path，普通本地媒体的目标来自选中 source path；Replace/Delete history 的 `OriginalLocation` 直接来自最终写入目标的显式类别，不通过相同目录反推。多源 STRM 的四类 D3 写操作在 Artifact/媒体写入前返回稳定 409。Delete 仅转移到媒体外 trash；Restore 在认证、gate、Item/source 结构与绑定校验后重新解析当前 Item/source，旧 STRM `source` history 在坏锚点检查前返回 `strm_history_location_unsupported`。每次补偿都会重新核对文件 Hash 和 Emby MediaStreams；补偿任一步无法验证时返回稳定 `subtitle_rollback_failed`，保留 archive/trash/quarantine 并明确需要人工恢复。Upload 只生成 PreviewArtifact，不直接落盘或写持久 history。History 使用默认值及最大值均受限的 `limit` 查询。私有 history 不保存媒体路径或上传原文件名。默认 `remote_search_enabled=false`、`write_enabled=false` 仍保持关闭，日常模式与 Canary 都需要独立写入 overlay 和目录权限预检。

该实现通过本地单元、Fake Emby 和浏览器 E2E 验证；修复后的真实 C92 单源 STRM 受控窗口已完成 Upload/Add/Replace/Delete/Restore、精确目录权限、Refresh、MediaStreams、官方字幕流和 closed 回滚。它仍未替代普通本地媒体、多源 STRM、真实 Provider、真实管理 UI 写入提交或本次新的客户端播放验收。

V1 通过 Emby Bridge 使用 Meiam Provider。Native Thunder 和 Native ASSRT 暂缓，详见 [ADR-001](../decisions/adr/001-v1-uses-emby-remote-subtitle-bridge.md)。

## D2-B1 后端实现（历史单源 Canary 基线）

D2-B1 在独立 `d2` Service 中实现 Search、Fetch、Preview 的后端闭环，并由 `config`、`embyclient`、`subtitleprovider`、`subtitle`、`preview` 和 `httpapi` 提供边界能力。本段记录原始单源 Canary 的证据；Core A/B 已以 [ADR-008](../decisions/adr/008-core-ab-daily-source-bound-recovery.md) 取代“多源统一 fail closed”的运行时限制，但保留 source 结构无效、缺失、错误、重复和变化的安全拒绝。

Remote Subtitle Bridge 只通过服务端 API Key 调用固定的两个 GET 接口。候选原始 ID 不进入 HTTP 响应、日志或 Artifact；Fetch 先做有界字幕校验和 canonical UTF-8 解析，再写入显式配置的专用稳定短期缓存。启用 D2 时 cache_dir 缺失、为根目录、与媒体映射双向 overlap 或通过 symlink/reparse point 到达其他位置都会 fail closed；启动同一缓存目录会回收旧 Artifact。Candidate/Artifact 绑定认证上下文、Item、source、语言和 allowlist generation，过期状态遵循一次 410、清理后 404 的无 tombstone 语义；成功 Fetch 重放复用 Artifact，Preview 只额外重读一次 Item，不重新 Fetch Provider。

D2 HTTP API 接受有效管理员会话或现有 Bearer 自动化凭据，JSON 请求体上限为 8 KiB，并提供固定错误码、并发/频率限流和脱敏请求日志。D2 不注册 Emby Remote Subtitle Save；Emby Refresh 和媒体写入只存在于独立、默认关闭的 D3 服务。内嵌 UI 只在 health 明确报告开关启用时显示当前已选 source 的候选、Fetch、预览与最小写入入口。管理员密码和 Bearer 不进入 JavaScript 存储，Candidate Token、Artifact Token 和 CSRF Token 仅留在页面内存，页面刷新即回到登录界面。D2-B1 后端证据见 [D2-B1 后端实现评审](../records/reviews/d2-b1-backend-implementation-review.md)，本地浏览器证据见 [D2-B2 UI 评审](../records/reviews/d2-b2-readonly-ui-review.md)，Core A/B 本地与真实单源 STRM 证据分别见 [Core A/B 实现评审](../records/reviews/core-ab-implementation-review.md) 和 [C92 单源 STRM 正式验收](../records/acceptance/core-ab-c92-acceptance-20260826.md)。真实 C92 单源 Provider API Canary 已通过；当前仍缺真实管理 UI 的完整提交、多 source 正向、普通本地媒体写入和新一轮客户端综合验收。

项目路线决策已经完成；上游完整构建基线仍有环境阻断和未验证项，但因为项目不采用 CSF 整仓运行时，这些缺口不再阻塞方案 B。ADR-002 已接受，选择新建轻量 Go 后端，选择性复用 ASS/SRT Parser 核心、语言与命名处理经验、相关测试思路、Emby HTTP 调用经验和少量无状态前端组件。

ChineseSubFinder 的旧扫描器、Cloud/SubtitleBest 下载链、Provider Hub、Cron/PreJob、旧任务队列、按视频物理路径保存和视频 Hash 逻辑不进入新运行时。Core A/B 已完成最小日常 Add、Replace、Delete、Upload、History/Restore UI；批量写入、自动下载和永久删除仍属于后续阶段。

Phase 2 的历史交付顺序和默认部署边界已记录在 [ADR-003](../decisions/adr/003-phase2-milestones-and-deployment.md)：D1 只读 Canary、D2 单源搜索预览和 D3 专用单源 Add 均已完成对应真实验收。Core A/B 的本地多 source 和可恢复写入实现由 [ADR-008](../decisions/adr/008-core-ab-daily-source-bound-recovery.md) 补充；真实正向支持的独立部署/验收门禁仍未解除，功能开关默认关闭。详细完成度和后续里程碑见 [当前状态与后续路线图](../planning/current-status-and-roadmap.md)。

## 证据

- [Gate 0 实测报告](../records/acceptance/gate0-report.md)
- [总体规划](../planning/master-plan.md)
- [ADR-002：项目代码路线](../decisions/adr/002-project-codebase-route.md)
- [ADR-005：缺少真实多源样本时有条件进入 D2](../decisions/adr/005-conditional-d2-entry-without-live-multisource.md)
- [ADR-008：Core A/B 日常模式、显式 source 绑定与可恢复字幕操作](../decisions/adr/008-core-ab-daily-source-bound-recovery.md)
- [ADR-009：STRM 写入锚点与多源写入边界](../decisions/adr/009-strm-write-target-and-multisource-boundary.md)
- [Core A/B 实现评审](../records/reviews/core-ab-implementation-review.md)
- [D2-B1 后端实现评审](../records/reviews/d2-b1-backend-implementation-review.md)
- [Phase 1 基线报告](../planning/phase1-baseline.md)
- [ChineseSubFinder 复用矩阵](../planning/chinesesubfinder-reuse-matrix.md)
- [Emby SubtitleService API](https://dev.emby.media/reference/RestAPI/SubtitleService.html)
- [Meiam ThunderProvider](https://github.com/91270/MeiamSubtitles/blob/master/Emby.MeiamSub.Thunder/ThunderProvider.cs)
