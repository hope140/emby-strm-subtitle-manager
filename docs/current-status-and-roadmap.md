# SubBridge 当前状态与后续路线图

状态日期：2026-08-25
用途：给主任务和新会话提供统一的“已完成、部分完成、待做”入口。部署版本和服务状态仍须实时核对，不能只根据本文件推断。

## 状态口径

- **已完成**：代码和相应自动化验证已经通过；涉及真实 Emby、文件写入或客户端行为时，也已有对应真实证据。
- **部分完成**：底层能力或安全拒绝已经完成，但正向产品流程、真实客户端或分发体验仍有缺口。
- **待做**：尚无可交付实现，主计划中的描述仍只是目标。

历史报告中的“D3 已完成”只指 [ADR-003](adr/003-phase2-milestones-and-deployment.md) 定义的**专用单源 Add Canary**。Core A/B 已在本地源码、Fake Emby 和最小浏览器 E2E 中实现日常受控模式、多 source、Replace、Upload、可恢复 Delete 和 Restore；它仍不代表已经部署或完成真实 C92 综合验收。

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
| Core A/B 本地实现 | 日常 gate、显式多 source Search→Fetch→Preview→Add、Upload→PreviewArtifact、Replace、可恢复 Delete、History/Restore 与最小 UI 已完成 | [Core A/B 实现评审](core-ab-implementation-review.md)、[ADR-008](adr/008-core-ab-daily-source-bound-recovery.md) |

D3.1 的客户端证据包含两层：自动化控制的 Emby Web 播放器读取，以及环境负责人在手机端进行的独立实际播放确认。手机端结果由用户在 2026-08-25 明确确认；未保存账号、媒体名称、Item ID、截图或客户端私有数据。

## 部分完成

| 能力 | 已有部分 | 仍缺什么 |
|---|---|---|
| D2 真实管理 UI | 本地 Fake Emby 浏览器 E2E、真实 C92 API Search→Fetch→Preview 已通过 | 真实 C92 管理 UI 中完整点击 Search→Fetch→Preview 尚未形成独立验收报告 |
| 多 MediaSource | 本地实现已要求显式 source，并在 Fake Emby/浏览器 E2E 覆盖正向流程 | 真实 C92 多 source、浏览器 UI、Refresh/字幕流和客户端对应验收尚未执行；不得把本地证据扩大为线上支持声明 |
| Add 日常使用 | 日常 gate、写入 overlay 示例、目标目录隔离、scope/CSRF 和清晰稳定错误码已完成本地实现 | 仍需独立授权后做普通 Compose/实际目录权限与真实运行验收 |
| V1 管理 UI | 浏览、清单、搜索预览、Add、Replace、Upload、可恢复 Delete、历史/Restore 的最小入口已完成 | Provider 状态页、完整设置/日志页、媒体库层级重构和面向发布镜像的安装体验仍未完成 |
| 公开分发 | 公开 GitHub 仓库、Dockerfile 和 Compose 示例已存在 | 尚无正式版本标签、GHCR/其他镜像发布、升级/回滚指南和面向新用户的端到端安装验收 |

## 尚未实现

- Core A/B 的真实 C92 综合验收：Movie、Episode、单源、多 source、字幕流和实际客户端读取，结束后恢复 closed/只读边界。
- Provider 状态页、完整设置/日志页、发布版设置说明和正式镜像发布。
- 缺字幕筛选、批量任务、自动下载、评分和时间轴校正；这些仍属于 V1 之后或独立研究线。

## 核心功能优先的压缩路线

Core A/B 的接口、模块、恢复语义、自动化矩阵和执行边界已经固定在 [Core A/B 连续实施计划](core-ab-implementation-plan.md)。

### Core A　完成搜索到 Add 的日常闭环（本地实现完成）

在同一个开发任务中完成日常 Add 和正向多源兼容，不再为单源、多源和管理 UI 点击分别建立独立版本。需要同时兼容“一个 Item 含多个 MediaSource”和“多个独立 Item”两种组织方式，明确绑定选中的 Item/source，再完成 Search→Fetch→Preview→Add。现有 UI 只增加完成闭环所需的最小控件，不调整媒体库层级和整体风格。

### Core B　补齐字幕管理写操作（本地实现完成）

在同一个开发任务中实现 Replace、Upload 和可恢复 Delete，共用现有 Item 锁、Artifact、Validator、原子写入、Refresh、Hash、history 和 quarantine。Replace 必须先验证新版本再归档旧字幕；Upload 忽略不可信文件名；Delete 默认移动到媒体库外回收目录，不执行即时永久删除。

Core A 与 Core B 已在同一核心开发会话连续完成，中间只做本地自动化和代码审核，未逐项部署。下一步只能在独立授权后进行一次 Movie、Episode、单源、多源、字幕流和手机端的综合 C92 验收。

### Core 0.x　核心测试版

沿用当前 UI 架构的必要操作入口、错误提示和最小操作记录查询已完成本地实现。Core 功能测试版仍需在真实验收后才能发布；Compose 继续作为设置来源，本阶段不开发完整设置页面，也不进行 Emby 风格的媒体库、电视剧、季、集、版本层级重构。

### UI/V1　界面重构与正式发布

核心测试版稳定后，再统一重构媒体库层级、完整字幕信息、设置、脱敏日志、历史/恢复入口和整体视觉，并补齐正式版本号、容器镜像、安装升级文档及全新环境验收。UI 重构不再反向改变已经稳定的核心写入契约。

## 当前运行边界

最近一次 C92 验收结束时，应用已恢复 `write_enabled=false`、`remote_search_enabled=false` 和 `/media:ro`。D2/D3 的重新启用、容器重建、媒体目录权限修改和真实写入仍需要独立授权与实时预检。SH、FRP 和 OpenResty 不属于上述开发里程碑的默认修改范围。

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

未验证范围与残余风险：本轮没有实时连接 C92，运行边界来自最近一次验收报告；日常 Add、多源正向能力、Replace、Upload 和 Delete 仍须完成实现，并在核心周期结束后统一真实验收。
