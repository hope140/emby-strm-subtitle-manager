# ADR-002：项目代码路线

- 状态：accepted
- 日期：2026-08-24
- 决策范围：Phase 1-A 完成后的代码路线，不授权进入 Phase 2 实现

## 背景

项目已通过 Gate 0，V1 已接受 Emby Remote Subtitle Bridge，当前仓库暂时只有规划、架构和实测文档，没有应用源代码。Phase 1-A 对 ChineseSubFinder 上游 commit `3335a9c95eec8e1664b7ab29368c34ce10f13575` 做了仓库外隔离快照，并检查了构建链、Emby API、Library、Parser、Cloud、旧扫描器和 Provider Hub 的实际关系。

基线结果并非全绿：`pkg/gss` 单包测试和前端生产构建通过；后端正式构建和最小启动被缺少 GCC/MinGW 阻断；Docker CLI/Engine 不存在；Parser 测试夹具在快照之外。上述环境问题需要记录，但路线判断主要依据源码耦合证据。

## 选项

### 选项 A：整仓渐进改造

在 ChineseSubFinder 现有 Gin、DAO、Cron、Downloader、Provider Hub、旧扫描器和 Library 页面上逐步替换业务。

优点是能保留现有前端、配置和部分下载流程，短期看起来改动集中。

代价是运行时边界已经交叉。`main` 创建旧下载器和 Cron，Cron 会创建任务队列、扫描器并做供应商检查；Library 依赖旧视频列表、物理路径、轮询和上传接口；Cloud 参与默认媒体信息、下载、缓存和反馈；Provider Hub 同时负责供应商生命周期和扫描策略。为了支持 STRM 和 Remote Subtitle Bridge，需要在多个旧模块中加入条件分支，容易让旧 Cloud/扫描路径继续成为隐式启动依赖。

### 选项 B：新建轻量后端，选择性复用

在当前项目路线下新建一个面向 Emby Item 和 Remote Subtitle Bridge 的轻量 Go 后端，先只设计必要的认证、媒体事实、搜索候选、预览和受控写入边界。复用范围限制为经过隔离后的 ASS/SRT Parser 核心、语言与命名处理经验、相关测试思路、Emby HTTP 调用经验和少量无状态前端组件；不复制旧扫描器、Cloud/SubtitleBest 下载链、Provider Hub、Cron/PreJob、旧任务队列、按视频物理路径保存逻辑和视频 Hash 逻辑。

这个选项不等于现在开始写 MediaContext、Inventory 或 Installer。它只决定后续代码应放在哪里，以及哪些上游模块不应成为架构基础。ADR 被接受后，仍须按总体规划和阶段门禁逐步实现。

## 决策

**选择选项 B：新建轻量 Go 后端，选择性复用。**

理由如下：

1. 旧启动链和后台任务链具有不可忽略的默认副作用，难以通过配置干净地关闭。
2. Library 页面与物理路径、旧扫描状态、任务队列和 Cloud 下载 API 紧密绑定，不能作为新 Bridge 的现成领域层。
3. 旧 Emby API 可提供调用经验，但 DTO 和 helper 已把本地物理文件假设带入调用链，需要重新建立最小边界。
4. ASS/SRT Parser 核心相对独立，适合在补齐可复现 fixture、许可证保留和边界测试后选择性复用。
5. V1 已确定使用 Remote Subtitle Bridge；把 SubtitleBest Cloud 和旧 Provider Hub 带入主运行时，会违背“Provider 只筛选已返回候选”和候选失败隔离的既有决策。
6. 当前上游构建链存在 CI 路径漂移、CGO 编译器依赖、外部 Parser fixture 和未固定 Docker 基础镜像等维护负担。它们不是单独否决整仓的理由，但与运行时耦合叠加后，整仓改造的收益不足以抵消边界重写成本。

## 复用与禁止带入的边界

| 类别 | ADR-002 结论 |
| --- | --- |
| 可以调查性复用 | ASS/SRT Parser 核心、语言与命名处理经验、相关测试思路、Emby HTTP 调用经验、少量无状态前端组件 |
| 必须先适配 | Emby HTTP client 的最小请求层、Parser Hub 的格式 dispatch、错误/日志边界、前端候选展示组件 |
| 不带入新运行时 | `video_scan_and_refresh_helper`、`scan_logic`、旧 `task_queue`、Cron/PreJob 启动副作用、SubtitleBest/Cloud 下载链、旧 Provider Hub、按视频物理路径保存和视频 Hash |
| 本阶段明确不做 | MediaContext、Inventory、Installer、Emby 服务部署/重启/配置修改、外部发布 |

完整证据见 [BASELINE.md](../../BASELINE.md) 和 [CSF_REUSE_MATRIX.md](../../CSF_REUSE_MATRIX.md)。

## 后果

接受选项 B 后，需要承担一部分新代码成本：重新定义最小 API、状态和错误模型；为 Emby Item/MediaSource/MediaStream 建立测试替身；重新处理 STRM 的路径和写入安全；为 Parser 准备可复现 fixture；未来再按阶段实现只读媒体模型、清单和 Installer。

同时获得明确的运行时边界：V1 搜索由 Bridge 提供候选，Cloud 不再是启动条件；旧扫描器不再决定 STRM 媒体范围；Provider 失败可按候选隔离；服务端可以在写入前重新解析 ItemID 并保留可恢复版本。

如果后续证据证明 ChineseSubFinder 的这些边界可以在不引入旧扫描/Cloud/物理路径副作用的前提下完整拆出，可另行提出 ADR 修订或新 ADR；本 ADR 不预先承诺整仓升级。

## 证据索引

- 上游构建和工具链结果：[BASELINE.md](../../BASELINE.md)
- 模块耦合和复用边界：[CSF_REUSE_MATRIX.md](../../CSF_REUSE_MATRIX.md)
- 项目当前架构：[architecture.md](../architecture.md)
- Phase 1 门禁：[phase1-baseline-checklist.md](../phase1-baseline-checklist.md)
- 已接受的 Bridge 决策：[ADR-001](001-v1-uses-emby-remote-subtitle-bridge.md)
- Gate 0 事实起点：[GATE0_REPORT.md](../../GATE0_REPORT.md)

## 进入下一阶段的门禁

ADR-002 已接受，Phase 1 的路线决策和文档收口完成。上游完整后端构建、Docker 构建和最小启动仍有环境阻断或未验证项，但因为项目不采用 CSF 整仓运行时，这些缺口不再阻塞方案 B。后续仍需按阶段门禁另行确认，不能以本 ADR 直接视为 MediaContext、Inventory 或 Installer 已获准实现；本次不进入 Phase 2。
