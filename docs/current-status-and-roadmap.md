# SubBridge 当前状态与后续路线图

状态日期：2026-08-26
用途：给主任务和新会话提供统一的“已完成、部分完成、待做”入口。部署版本和服务状态仍须实时核对，不能只根据本文件推断。

## 状态口径

- **已完成**：代码和相应自动化验证已经通过；涉及真实 Emby、文件写入或客户端行为时，也已有对应真实证据。
- **部分完成**：底层能力或安全拒绝已经完成，但正向产品流程、真实客户端或分发体验仍有缺口。
- **待做**：尚无可交付实现，主计划中的描述仍只是目标。

历史报告中的“D3 已完成”只指 [ADR-003](adr/003-phase2-milestones-and-deployment.md) 定义的**专用单源 Add Canary**。Core A/B 已在本地源码、Fake Emby 和最小浏览器 E2E 中实现日常受控模式、普通本地媒体多 source、Replace、Upload、可恢复 Delete 和 Restore；单源 STRM 现按 Item.Path 写入，多源 STRM 写入 fail closed。2026-08-25 的精确 C92 app-only 尝试仍使用 source-bound 旧目标规则，在媒体操作前阻断并恢复 closed/只读；随后修复后候选已通过 C92 **单源 STRM 服务端闭环**验收。后者只覆盖受控 Upload/Add/Replace/Delete/Restore、MediaStreams、官方字幕流和 closed 收尾，不代表普通本地媒体、多源 STRM、真实 Provider、完整 UI 提交或新的客户端播放通过。

## 已完成

| 能力 | 当前结果 | 主要证据 |
|---|---|---|
| Gate 0 | Emby Remote Subtitle Bridge 路线和真实 Provider 基础行为已验证 | [Gate 0 报告](../GATE0_REPORT.md) |
| Phase 1 | 新建轻量 Go 后端的方案 B、复用边界和构建基线已收口 | [Phase 1 基线](../BASELINE.md)、[ADR-002](adr/002-project-codebase-route.md) |
| D1 只读纵向切片 | Movie/Episode、STRM、MediaContext、PathMapper、Inventory、真实 C92 部署和只读安全边界已验收 | [D1 部署验收](d1-deployment-acceptance.md) |
| D1.5 管理 UI 基础 | Go 内嵌 UI、媒体浏览、详情、字幕清单和本地浏览器 E2E 已完成 | [D1.5 UI](d1.5-readonly-ui.md) |
| D2 单源后端 | Search、候选隔离、Fetch、Validator、PreviewArtifact 和 Preview 已实现；真实 C92 单源 API Canary 通过 | [D2 契约](d2-search-preview-contract.md)、[D2 C92 Canary](d2-c92-canary-acceptance.md) |
| D2.5 管理员认证 | Compose environment 用户名密码、HttpOnly 会话、CSRF、自动化 Bearer scope 和 C92 登录验收已完成 | [D2.5 管理员认证](d2.5-admin-auth.md) |
| 多源识别与安全拒绝 | Emby 4.9.x `AlternateMediaSources` 读取、source 对应核对和 D2 多源 `409` fail-closed 已完成 | [多源真实 API Canary](d2-multisource-c92-canary-acceptance-20260825.md) |
| D3.1 专用单源 Add | Artifact 绑定、Item 锁、幂等、原子非覆盖写入、Refresh/轮询、history/quarantine、Hash、字幕流和真实客户端读取已完成 | [D3 Add 契约](d3-dedicated-add-contract.md)、[D3 C92 Canary](d3-c92-canary-acceptance-20260825.md) |
| Core A/B 本地实现 | 日常 gate、普通本地媒体显式多 source Search→Fetch→Preview→Add、单源 STRM Item.Path 写入、Upload→PreviewArtifact、Replace、可恢复 Delete、History/Restore 与最小 UI 已完成；多源 STRM 写入稳定拒绝 | [Core A/B 实现评审](core-ab-implementation-review.md)、[ADR-008](adr/008-core-ab-daily-source-bound-recovery.md)、[ADR-009](adr/009-strm-write-target-and-multisource-boundary.md) |
| Core A/B 单源 STRM C92 服务端闭环 | 受控 Upload→Preview→Add、Replace→Restore、Delete→Restore→最终 Delete、Hash、Refresh、MediaStreams、官方字幕流、source-specific history 和 closed 回滚已完成 | [C92 单源 STRM 正式验收](core-ab-c92-acceptance-20260826.md) |

D3.1 的客户端证据包含两层：自动化控制的 Emby Web 播放器读取，以及环境负责人在手机端进行的独立实际播放确认。手机端结果由用户在 2026-08-25 明确确认；未保存账号、媒体名称、Item ID、截图或客户端私有数据。

## 部分完成

| 能力 | 已有部分 | 仍缺什么 |
|---|---|---|
| D2 真实管理 UI | 本地 Fake Emby 浏览器 E2E 与受控 C92 单源 STRM 管理 UI 的 Search→Fetch→Preview 已通过 | 普通本地媒体和多源 STRM 的 UI 边界仍需独立验收 |
| 多 MediaSource | 本地实现已要求显式 source；普通本地媒体在 Fake Emby/浏览器 E2E 覆盖正向流程，多源 STRM 的 Search/Fetch/Preview 保留而 D3 写入稳定 409；C92 真实 source 对应和 D2 安全拒绝已有历史证据 | 多源 STRM 的 Core A/B D3 409 尚无新的真实 C92 路由级复核；普通本地媒体的真实 C92 正向流程和实际客户端读取也仍未执行 |
| Add 日常使用 | 单源 STRM 的受控 C92 管理 UI 已完成真实 Provider Search→Fetch→Preview→Add、Upload、Replace、Delete、Restore 和实际播放器读取；日常 gate、scope/CSRF 与 closed 收尾通过 | 仍需独立验收普通本地媒体和多源 STRM D3 409；这些不能从单源 STRM 推断 |
| V1 管理 UI | 浏览、清单、搜索预览、Add、Replace、Upload、可恢复 Delete、历史/Restore 的最小入口已完成；M1 已实现只读媒体库→剧集→季→集→版本惰性导航和安全操作能力摘要；M2 已增加脱敏运行摘要、当前搜索 Provider 候选汇总与当前媒体源历史筛选 | 完整设置/日志页不在 V1 范围；仍缺正式 UI 视觉收口 |
| 公开分发 | `v0.4.0` GitHub Release、公开 GHCR 不可变镜像、OIDC keyless 签名、SBOM/provenance、安装/升级/回滚/排障指南已完成；C92 已完成默认 closed app-only 部署及隔离临时实例的安装、同版本重建/恢复演练 | 新主机、跨版本升级/回滚与凭据初始化/轮换演练仍未执行 |

## 尚未实现

- Core A/B 的剩余独立边界验收：普通本地 Movie/Episode 正向写入与客户端读取，以及多源 STRM D3 409 和无媒体变更。单源 STRM 的真实 Provider、完整管理 UI 写入提交与实际客户端播放已经完成，但不替代这些范围。
- 新主机的默认 closed 安装，以及跨版本升级/回滚和凭据初始化/轮换演练；它们不代替已有 C92 受控写入验收。
- 缺字幕筛选、批量任务、自动下载、评分和时间轴校正；这些仍属于 V1 之后或独立研究线。

## 核心功能优先的压缩路线

Core A/B 的接口、模块、恢复语义、自动化矩阵和执行边界已经固定在 [Core A/B 连续实施计划](core-ab-implementation-plan.md)。

### Core A　完成搜索到 Add 的日常闭环（本地实现完成）

在同一个开发任务中完成日常 Add 和普通本地媒体的正向多源兼容，不再为单源、多源和管理 UI 点击分别建立独立版本。需要同时兼容“一个 Item 含多个 MediaSource”和“多个独立 Item”两种组织方式，明确绑定选中的 Item/source，再完成 Search→Fetch→Preview→Add；单源 STRM 以 Item.Path 作为写入锚点，多源 STRM 写入保持安全拒绝。现有 UI 只增加完成闭环所需的最小控件，不调整媒体库层级和整体风格。

### Core B　补齐字幕管理写操作（本地实现完成）

在同一个开发任务中实现 Replace、Upload 和可恢复 Delete，共用现有 Item 锁、Artifact、Validator、原子写入、Refresh、Hash、history 和 quarantine。Replace 必须先验证新版本再归档旧字幕；Upload 忽略不可信文件名；Delete 默认移动到媒体库外回收目录，不执行即时永久删除。

Core A 与 Core B 已在同一核心开发会话连续完成。旧 source-bound 规则曾在 C92 样本门禁处停止；STRM 锚点修复后已重新构建并通过单源 STRM 的受控服务器端验收。下一步只针对未覆盖范围进行独立预检和验收，不重新把已通过的单源 STRM API 闭环描述为待做。

### Core 0.x　核心测试版

单源 STRM 的真实管理 UI 和客户端闭环已经完成。正式可追溯镜像、升级/回滚说明和隔离临时实例的默认 closed 安装演练已经完成；新主机与跨版本演练留待具备对应版本和环境时执行，不阻断下一阶段 UI/V1；Compose 仍是设置来源，不在这一阶段开发完整设置页面。

### UI/V1　界面重构与正式发布

下一项开发工作是只读媒体层级、字幕信息和状态展示；随后补 Provider 状态、只读设置摘要、脱敏日志和历史/恢复体验。UI 重构不反向改变已经稳定的 Item/source、写入目标和恢复契约。正式分发在 UI/V1 稳定后统一收口。详细里程碑、验收条件与非目标见 [发布收口与 UI/V1 计划](release-and-ui-v1-plan.md)。

## 当前运行边界

截至 2026-08-26 的 `v0.4.0` C92 app-only 默认 closed 部署后，应用运行 `write_enabled=false`、`remote_search_enabled=false` 和 `/media:ro`，容器 healthy、restart=0。验收记录见 [v0.4.0 C92 默认 closed 部署验收](v0.4.0-c92-closed-deployment-acceptance.md)。D2/D3 的重新启用、媒体目录权限修改和任何新的真实写入仍需要独立授权与实时预检。SH、FRP 和 OpenResty 不属于上述开发里程碑的默认修改范围。

## 暂不作为阻断项

- UI 信息架构和视觉重构不阻断 Core A、Core B；核心阶段只保留最小可测试入口。
- Windows 本机已安装 MSYS2 GCC；仅在测试进程中把 `C:\msys64\ucrt64\bin` 临时加入 `PATH` 后，`go test -race ./...` 已通过，未修改系统环境变量。发布流水线仍应在固定工具链的 CI 中保留 race 检查。
- 批量和自动下载不属于当前核心收口路径，不应提前占用 Core A、Core B 和 UI/V1 的实现范围。

## Knowledge Review

2026-08-25 的 Core A/B 实施、文档分流和未验证范围见 [Core A/B 实现评审](core-ab-implementation-review.md)。以下内容保留为实施前状态复核的历史记录，不能覆盖该评审或实时运行状态。

任务或阶段：截至 2026-08-25 的项目完成度、证据边界和后续顺序复核。

验证范围：总体规划、当前架构、ADR、D1–D3 阶段报告、维护经验、本地操作记录、当前 Git 分支与最近提交，以及环境负责人的手机端实测反馈。

Knowledge Findings：

- 新增约束：后续文档中的“D3 完成”必须写成“D3.1 专用单源 Add 完成”，不能据此宣称整个 Phase 4 或日常写入能力完成。
- 隐蔽坑：真实 Canary 证明底层闭环可行，但临时 allowlist、可写 overlay 和精确目录授权仍不是普通管理员的日常使用流程。
- 被证明不完整的假设：把日常 Add、多源、Replace、Upload 和 Delete 拆成各自独立版本会产生重复评审与部署；当前改为一个连续核心开发周期和一次综合真实验收。
- 建议沉淀项：以本文件作为唯一当前进度入口；历史报告只保留其任务发生时的范围和证据。

证据：

- 代码：当前 `main` 已包含 D1、D2、D2.5、完整 MediaSources 修正和 D3 专用 Add 实现。
- 测试：各阶段报告保存当时的全包测试、静态检查、Fake Emby、浏览器 E2E 和构建证据；本轮未修改代码。
- 实际运行、日志或可复现结果：D1、D2 单源、D2 多源安全拒绝和 D3 C92 报告；手机端读取结论来自环境负责人 2026-08-25 的明确反馈。

去重检查：已搜索 `AGENTS.md`、当前架构、维护经验、ADR、总体规划、README 和 D1–D3 阶段报告；已更新重复或过时的当前状态表述。

分流判断：更新当前架构、ADR-003、总体规划、README、文档索引和 `LOCAL_OPERATIONS.md`；手机端反馈补入 D3 验收和实现评审。状态复核本身没有新的底层技术规则需要写入维护经验；后续发生的 SubBridge 技术标识迁移与 C92 兼容边界另见 [ADR-007](adr/007-subbridge-brand-and-legacy-deployment-identifiers.md)。

未验证范围与残余风险：实施前复核本身没有实时连接 C92；后续 [Core A/B C92 综合部署验收](core-ab-c92-acceptance.md) 已记录一次真实部署尝试，但在 source-bound 门禁处阻断。日常 Add、多源正向能力、Replace、Upload、Delete、Restore、字幕流和客户端读取仍未形成真实 C92 通过证据。

## 2026-08-25 Core A/B C92 尝试结果

精确提交 `947d847bb8ee620fc0362081fdff981069472081` 已完成源码归档、OCI 构建、版本化 Compose/config、daily 启动和 app-only 回滚。daily 窗口健康运行时，真实 C92 有界查询覆盖 7,321 个 Movie/Episode Item（37 页，最多 7,400 个槽位），但没有找到选中 `MediaSource.Path` 位于应用 `/media` 映射下的本地 source。该次尝试按当时 ADR-008 的 source-bound 规则拒绝以 Item.Path 替代 source path，因此没有产生任何媒体写操作，C92 已恢复 closed/只读。该次报告作出时 ADR-009 修复尚未部署或连接 C92；后续状态见下一节。完整历史证据见 [Core A/B C92 综合部署验收](core-ab-c92-acceptance.md)。

## 2026-08-26 Core A/B 单源 STRM C92 正式验收

ADR-009 修复后的候选提交已以 app-only 方式部署到 C92。单源 STRM 使用 `Item.Path` 锚点并保持显式 source 绑定，完成受控 Upload→Preview→Add、Replace→Restore、Delete→Restore→最终 Delete，以及 Hash、Refresh、MediaStreams、官方字幕流、source-specific history 和 closed 回滚。随后用户在同一受控窗口完成真实管理 UI 的 Provider Search→Fetch→Preview、Upload、Add、Replace、Delete、Restore 和实际播放器字幕显示；ASS/UI 兼容修复复测后再次恢复 closed。普通本地媒体和多源 STRM D3 409 仍不在通过结论内。完整证据和清理边界见 [Core A/B C92 单源 STRM 正式验收](core-ab-c92-acceptance-20260826.md) 与 [ASS/UI 修复 C92 验收](core-ab-ass-ui-fix-c92-acceptance-20260826.md)。
