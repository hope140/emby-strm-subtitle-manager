# ChineseSubFinder 复用矩阵

范围：上游 commit `3335a9c95eec8e1664b7ab29368c34ce10f13575` 的静态代码取证，以及本次基线中可执行的局部测试。结论针对当前 SubBridge（SB，字幕桥）V1，V1 的事实约束仍以 [ADR-001](docs/adr/001-v1-uses-emby-remote-subtitle-bridge.md)、已接受的 [ADR-002](docs/adr/002-project-codebase-route.md)、[总体规划](SubBridge_Master_Plan_Revised.md) 和 Gate 0 为准。

ADR-002 已正式接受方案 B：新建轻量 Go 后端，选择性复用；本矩阵仍只记录复用边界和证据，不授权现在开始编写 MediaContext、Inventory 或 Installer。

“复用”只表示可以在隔离边界内直接移植或引用；“适配”表示需要先抽象接口、替换路径/状态模型或补测试；“替换”表示不应把原模块带入新运行时。

## 总览

| 模块 | 上游实际耦合 | 证据 | 路线结论 | 复用边界 |
| --- | --- | --- | --- | --- |
| Emby API | API 类直接负责设置检查、HTTP 请求、Item 查询、Ancestors、Refresh、字幕流和物理路径字段；调用方继续向下传递旧类型 | `pkg/emby_api/emby_api.go:18-312`；`pkg/types/emby/type.go:35-174` | 适配 | 只提取认证、Item/MediaSource/MediaStream 和字幕流所需的最小 client facade；不直接复用旧 DTO 作为新领域模型 |
| Emby API 测试 | 测试文件存在，但真实网络测试大多为 TODO/注释，不能证明实例兼容性 | `pkg/emby_api/emby_api_test.go:1-80`；`pkg/logic/emby_helper/embyhelper_test.go` | 替换为受控测试 | 先用 Fake Emby 覆盖请求/响应，再按 Gate 规则在允许的真实实例验证；本阶段不触碰服务 |
| Library 页面 | 页面依赖旧 `/v1/video/*`、任务队列、轮询刷新、物理路径、侧车字幕路径和旧上传接口 | `frontend/src/api/LibraryApi.js:5-56`；`frontend/src/pages/library/use-library.js:9-133`；`frontend/src/pages/library/movies/index.vue:18-39` | 替换页面数据层，有限保留视觉组件 | 不把旧 store、轮询协议和路径 DTO 带入 V1；后续按方案 B 另行设计 Bridge 搜索/预览状态 |
| Library 搜索弹窗 | `SearchPanelCsfApi` 和 TV package 直接调用 SubtitleBest Cloud，按本地视频路径获取 IMDB、下载 URL，再通过旧上传接口写盘 | `frontend/src/pages/library/SearchPanelCsfApi.vue:160-277`；`SearchPanelCsfApiTvPackage.vue:196-256` | 替换搜索协议，适配 UI | 可参考交互和展示结构；候选、Provider 标签和下载/写入动作必须改为 Bridge 领域协议 |
| Manual 搜索 | 直接调用旧预览搜索接口并拼外部链接 | `frontend/src/pages/library/SearchPanelManual.vue:45-70` | 替换 | 只保留人工选择的交互意图，不保留旧 URL 和预览队列协议 |
| ASS/SRT Parser | Parser 主要做文件/bytes 读取、编码转换、时间轴和语言识别，依赖相对独立 | `pkg/logic/sub_parser/ass/ass.go:16-145`；`pkg/logic/sub_parser/srt/srt.go:18-247` | 适配后复用 | 先移植纯解析核心、语言统计和错误边界；排除旧的物理文件扫描、日志初始化和全局设置耦合 |
| Parser Hub | Hub 同时负责扩展名分发、Emby codec/language 判断、递归侧车扫描和中文判断 | `pkg/sub_parser_hub/subParserHub.go:19-213` | 适配 | 仅保留格式 dispatch 与内容校验；sidecar 扫描和 Emby 媒体流映射拆到后续明确边界的模块 |
| Parser 测试夹具 | ASS 11 个、SRT 4 个、Hub 9 个测试用例引用快照外的 `ChineseSubFinder-TestData` | `pkg/logic/sub_parser/ass/ass_test.go`；`srt_test.go`；`pkg/sub_parser_hub/subParserHub_test.go` | 补齐后再复用 | 夹具必须放入受控测试目录并脱敏；本次执行因目录不存在而失败，不能视为通过 |
| Cloud / SubtitleBest | 启动时由 `PreDownloadProcess` 检查供应商和网络，媒体信息、下载、缓存、反馈、队列都可以进入 Cloud | `pkg/logic/pre_download_process/pre_download_proces.go:28-183`；`pkg/logic/sub_supplier/subtitle_best/subtitle_best.go:17-256`；`pkg/logic/cron_helper/cron_helper.go:246-256` | V1 运行时替换/移除 | V1 使用 Emby Remote Subtitle Bridge；Cloud 只能作为未来隔离适配器，不能成为启动条件或主搜索链 |
| SubtitleBest API | `FileDownloader` 和 `media_info_dealers` 默认持有并调用 SubtitleBest API，包含网络检查、媒体信息查询、下载 URL 和等待队列 | `pkg/logic/file_downloader/downloader_hub.go:28-43,189-227`；`pkg/media_info_dealers/dealers.go:15-196`；`pkg/subtitle_best_api/subtitle_best_api.go:17-60` | 替换 | 新服务不复制随机认证、Cloud 下载、缓存和配额语义；Provider 失败只隔离到候选级别 |
| 旧 Provider Hub | Hub 不只是 provider registry，还持有扫描策略、设置、路径/剧集 helper、并发、状态回报和供应商生命周期 | `pkg/logic/sub_supplier/subSupplierHub.go:22-258`；`pkg/ifaces/iSupplier.go:9-24` | 替换为窄接口 | 新接口只表达搜索候选、状态和可选下载/流信息；不能把旧 `ISupplier` 契约直接搬入 Bridge |
| 旧扫描器 | 同时扫描本地电影/剧集目录、触发 Emby 扫描刷新、更新 DAO/cache、计算 hash、过滤跳过项、驱动下载 | `pkg/video_scan_and_refresh_helper/video_scan_and_refresh_helper.go:55-351`；`pkg/scan_logic/scan_logic.go:12-71` | 替换 | 不带入新运行时；STRM 媒体清单必须由 Emby Item/MediaSource 事实驱动，不能从本地目录扫描推断 |
| Cron / PreJob / Downloader 启动链 | `main` 创建 FileDownloader、CronHelper、Backend；CronHelper 创建任务队列、扫描器并立即做 supplier check；PreJob 还会批量改名/扫描 | `cmd/chinesesubfinder/main.go:45-161`；`pkg/logic/cron_helper/cron_helper.go:20-70`；`pkg/logic/pre_job/pro_job.go:25-90` | 替换 | 新服务采用显式请求/任务边界，不把旧启动副作用和后台 cron 作为基础设施依赖 |
| Backend Router | Gin 路由直接暴露旧视频列表、任务、上传、预览、设置和静态本地媒体路径 | `internal/backend/backend.go:38-122`；`internal/backend/base_router.go:21-157` | 替换 API 层 | 不兼容旧 `/v1/video/*` 作为新领域 API；可参考认证、错误 envelope 和健康检查的实现经验 |
| File/path/save | 路径映射是简单字符串替换；保存逻辑直接写入视频目录并删除/覆盖既有默认字幕；hash 读取视频物理文件 | `pkg/path_helper/path_helper.go:34-39`；`pkg/save_sub_helper/save_sub_helper.go:27-88`；`pkg/sub_file_hash/sub_file_hash.go:15-83` | 替换 | STRM 不假设本地视频内容或可写目录；写入必须由服务端 ItemID 重新解析、原子写新版本、校验后归档旧文件 |
| Sidecar 匹配/剧集路径 | 依赖本地目录、文件名、物理 basename、季集目录和 `.default/.forced` 重命名 | `pkg/sub_helper/sub_helper.go:271-309`；`pkg/logic/series_helper`；`pkg/video_list_helper/video_list_helper.go:25-79` | 替换 | 仅可参考命名解析测试；不复用其 Inventory 结论或物理路径作为客户端事实 |

## 1. Emby API 与媒体模型

旧 API 层的优点是已经覆盖了 Item、用户、Ancestors、Refresh、MediaSource、MediaStream 和字幕流等调用；问题是它把设置读取、认证、HTTP、旧 DTO 和物理文件约束混在一起。`EmbyVideoInfo` 同时承载 Path、MediaSources、MediaStreams 和 ProviderIds，旧 helper 还会把它转换成物理视频文件路径并检查本地文件存在。

这与 STRM 的边界冲突：STRM 入口的播放和媒体源事实应由 Emby 播放链提供，字幕清单和写入目标需要从服务端 ItemID 重新解析。可复用的是经过测试的 HTTP 访问经验和最小字段映射，不是整个 `EmbyApi`、`EmbyVideoInfo` 或 `EmbyHelper`。

## 2. Library 页面与 Cloud 的实际关系

Library 页面表面上是字幕搜索页面，实际由旧扫描器和下载器提供数据。`use-library.js` 会加载旧视频列表、轮询刷新状态、按物理路径生成静态 URL，并把跳过状态保存为路径相关信息。搜索组件又直接使用 SubtitleBest Cloud 获取 IMDB、候选下载 URL，并把下载结果交给旧后端上传。

因此，单独替换一个搜索按钮不能解除耦合。若整仓改造，至少要同时替换 Library 数据 API、任务状态、候选模型、下载/写入链和路径安全策略；这已经接近重写运行时。新建轻量后端可以把 Bridge 搜索预览作为独立边界，未来再决定是否保留 Library UI 的部分视觉组件。

## 3. Parser 的可复用范围

ASS 和 SRT 的解析函数具有较好的局部性，支持文件和 bytes 两种入口，并完成编码转换、时间轴解析和语言统计。这是当前快照里最适合复用的部分。Parser Hub 则夹带了扩展名分发、Emby stream 判断和递归 sidecar 扫描，必须拆开后再使用。

本次运行了：

```text
go test -mod=readonly ./pkg/logic/sub_parser/... ./pkg/sub_parser_hub/...
```

测试没有通过，因为 fixture 指向快照之外的 `ChineseSubFinder-TestData`。这揭示了上游测试的可复现性问题，不能据此给 Parser 核心打通过标记。后续复用前必须提供最小、脱敏、仓库内可获得的测试样本，并补充 bytes 入口、UTF-16LE、`[Script Info]`、异常时间轴和 STRM 不存在物理视频文件等边界测试。

## 4. 路线判定输入

支持整仓改造的证据是前端、后端、Parser、Cloud 和 Emby API 都已存在可运行实现；反向证据更关键：旧启动链会创建并启动 Cron、任务队列、扫描器和供应商检查，Library 以物理路径和旧任务接口为中心，Cloud 参与默认媒体信息和下载链，Provider Hub 还承担扫描策略。把它们逐项关闭会形成大量侵入式条件分支。

因此，当前证据支持“新建轻量后端，选择性复用 Parser 核心和少量表现层经验”，不支持把 ChineseSubFinder 整仓作为 V1 主框架。具体决策和后果见 [ADR-002-project-codebase-route.md](docs/adr/002-project-codebase-route.md)。
