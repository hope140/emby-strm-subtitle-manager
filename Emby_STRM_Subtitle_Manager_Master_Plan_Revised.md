# Emby STRM 字幕管理器修订版总体规划

文档状态　可执行规划草案

修订日期　2026 年 8 月 24 日

适用场景　Emby、CMS、115、CD2、STRM

## 1. 项目目标

本项目面向已经由 CMS 整理完成、由 Emby 建立索引的 STRM 媒体库，提供独立的中文字幕管理界面。

项目负责这些工作。

- 从 Emby 获取电影、剧集和媒体路径
- 汇总 Emby 已识别的字幕流与同目录字幕文件
- 通过 Emby 的远程字幕接口搜索和获取 Meiam 候选
- 预览、上传、添加、替换和删除可管理的外挂字幕
- 在写入后触发 Emby 刷新，并核验 Emby 与实际播放路径看到的新字幕
- 在人工流程稳定后增加缺字幕列表、批量处理和保守自动下载

以下职责继续留在现有系统中。

- CMS 负责目录整理和媒体命名
- Emby 负责媒体索引、媒体流识别和播放
- 115、CD2 与现有代理链负责媒体访问
- Meiam 第一阶段负责 Thunder 与 ASSRT 的远程接口适配

本项目不读取 STRM 内部地址，不管理 115 或 CD2 凭据，也不建立第二套媒体索引。

## 2. 已核实的技术边界

### 2.1 Emby 远程字幕接口可以支撑搜索和预览

当前公开接口包含以下能力。

```text
GET /Items/{Id}/RemoteSearch/Subtitles/{Language}
GET /Providers/Subtitles/Subtitles/{Id}
POST /Items/{Id}/RemoteSearch/Subtitles/{SubtitleId}
```

第一条返回候选列表，第二条返回远程字幕内容，第三条让 Emby 下载并保存字幕。本项目在 V1 中使用前两条，由自己的 Installer 负责落盘和命名。

搜索接口公开参数只包含 Item、MediaSource、语言及 forced 等条件。它没有自定义搜索词和指定 Provider 的参数。因此，V1 中的 Thunder、ASSRT 标签只负责结果筛选和排序，不能承诺只调用某一个 Provider。

如果以后需要精确控制搜索词、Provider 调用次序和失败回退，就新增 NativeThunderProvider 或 NativeAssrtProvider。这个能力不写入 Emby Bridge 的 V1 承诺。

### 2.2 Meiam Thunder 能按名称搜索，但会尝试计算媒体路径 CID

当前 Meiam Thunder 会先读取 `request.MediaPath` 计算 CID，随后按媒体名调用 Thunder 名称搜索。CID 只参与结果排序。

当 MediaPath 是可读的 `.strm` 文件时，Meiam 会读取这个文本文件并计算一个没有实际匹配价值的 Hash。它不会因此访问 STRM 内部 URL。

V1 采用以下处理。

- 本项目自身不读取 STRM 内容，也不计算 CID
- 集成测试必须证明 STRM 内部 URL 没有收到请求
- 接受 Meiam 当前版本读取 STRM stub 的既有行为，并把这项行为写入兼容记录
- 如果后续确认服务器日志、性能或安全策略要求严格跳过，再维护一个仅增加 `.strm` 判断的小型 Meiam 补丁

这样可以避免在第一阶段同时维护 Go、Vue 和 C# 三条代码线。

### 2.3 搜索词由 Emby 和 Meiam 共同决定

使用 Emby Bridge 时，本项目不能把自定义关键词传给字幕 Provider。Meiam 根据 Emby 提供的 `Name`、`SeriesName`、季集号或媒体文件名生成查询。

原规划中的多关键词生成保留为 Native Provider 的后续能力，不进入 V1 Bridge。

### 2.4 Refresh 成功不代表用户端已经读到新字幕

字幕文件写入后，需要分层确认。

```text
文件系统存在且 Hash 正确
↓
Emby Item 已出现新的外部字幕流
↓
Emby 直连返回新字幕内容
↓
CMS 或公网播放路径返回同一内容
```

代理层可能按字幕 URL 缓存旧内容。替换同名字幕时，单独调用 Emby Refresh 无法清除代理缓存。

默认替换流程先写入带版本的新文件名，确认新流可用后再归档旧文件。若部署环境已经关闭字幕代理缓存，可以启用同名原子替换，但仍需完成 Emby 直连检查。

## 3. 总体架构

```text
Emby API
  │
  ├─ EmbyCatalogService
  │     └─ MediaContext
  │
  ├─ Emby MediaStreams
  │     └─ 已识别字幕流
  │
  └─ Remote Subtitle API
        └─ Meiam Thunder / ASSRT

PathMapper
  └─ EmbyPath → LocalPath

SubtitleInventoryService
  ├─ Emby MediaStreams
  └─ 同目录 Sidecar 文件

SubtitlePreviewService
  └─ Fetch → Validate → Parse → Cue[]

SubtitleInstaller
  └─ Add / Replace / Delete / Upload

SubtitleReconciler
  └─ Refresh → Poll → Direct Verify → Public Verify

Web UI
  └─ 浏览 / 搜索 / 预览 / 管理
```

## 4. 核心数据模型

### 4.1 MediaContext

```go
type MediaContext struct {
    ItemID        string
    MediaSourceID string

    MediaType     string
    Title         string
    OriginalTitle string
    Year          int

    SeriesTitle   string
    Season        int
    Episode       int

    ImdbID        string
    TmdbID        string
    TvdbID        string

    EmbyPath      string
    LocalPath     string
    LocalDir      string
    BaseName      string

    IsStrm        bool
}
```

`MediaSourceID` 必须保存。远程字幕搜索、字幕流读取和多版本媒体处理都可能依赖它。

### 4.2 CurrentSubtitle

当前字幕不能只表示本地文件。统一模型需要区分来源和可管理性。

```go
type CurrentSubtitle struct {
    ID          string
    Kind        string
    // embedded / external

    DiscoveredBy []string
    // emby / filesystem

    StreamIndex int
    Path        string
    FileName    string
    Language    string
    Format      string

    IsDefault   bool
    IsForced    bool
    IsText      bool
    Manageable  bool
}
```

内嵌字幕可以查看和参与状态判断，`Manageable` 必须为 false。只有经过 PathMapper 和路径安全检查的 Sidecar 才能被 Installer 修改。

同一个外挂字幕往往会同时出现在 Emby MediaStreams 和文件系统扫描结果中。Inventory 先按规范化路径合并，再用 StreamIndex 和文件内容 Hash 补充核对，不能把两次发现显示成两条字幕。

### 4.3 SubtitleCandidate

```go
type SubtitleCandidate struct {
    Token        string
    Provider     string
    Name         string
    Language     string
    Format       string
    Comment      string
    IsHashMatch  bool
    Score        float64
    Reasons      []string
    ExpiresAt    time.Time
}
```

远程字幕 ID 视为短期、不透明的服务端数据。前端只得到本项目签发的短期 `Token`，后端缓存 Token 与 Emby 原始 ID 的映射。这样可以避免把可能包含下载信息的 Provider ID 暴露给浏览器和日志。

候选完成 Fetch 和校验后，PreviewService 生成短期 PreviewArtifact，记录内容 Hash、格式、语言和临时文件位置。用户从预览页执行安装时，Installer 使用这份已验证内容，确保落盘内容与预览内容一致。未预览直接添加时，Installer 重新 Fetch 并完成相同校验。

### 4.4 ProviderCapabilities

```go
type ProviderCapabilities struct {
    SupportsProviderSelection bool
    SupportsCustomQuery       bool
    SupportsHashMatch         bool
}
```

`EmbyRemoteSubtitleProvider` 在 V1 中返回前两项为 false。UI 根据能力显示准确提示，不把结果筛选写成来源控制。

## 5. 字幕状态口径

在开发缺字幕列表前，先配置一套明确的 SubtitlePolicy。

```yaml
subtitle_policy:
  accepted_languages:
    - zh-CN
    - zh
    - zho
    - chi
  embedded_text_counts_as_present: true
  embedded_graphic_counts_as_present: true
  external_text_counts_as_present: true
```

状态由 Emby MediaStreams 与本地 Sidecar 合并计算。

- `Missing` 没有满足策略的中文字幕
- `Present` 至少有一条满足策略的字幕
- `Unmanaged` Emby 能看到字幕，但本项目不能安全定位或修改文件
- `Duplicate` 存在多条语义相同的外挂字幕，只提示
- `NeedsReview` 搜索结果存在，但没有达到人工或自动采用条件
- `SyncProblem` 用户明确标记时间轴异常

自动下载只处理 `Missing`。`Unmanaged`、`Duplicate` 和 `SyncProblem` 都不能触发自动替换。

## 6. PathMapper 与文件安全

PathMapper 使用规范化后的最长前缀匹配，拒绝模糊匹配。Windows 路径比较默认不区分大小写，Linux 路径按实际文件系统处理。

所有写操作必须完成以下检查。

1. 从 ItemID 重新取得 MediaContext，不信任前端提交的路径。
2. 对映射结果执行绝对路径规范化。
3. 确认目标目录位于配置的媒体根目录内。
4. 检查软链接、目录联接和 reparse point，防止解析后逃逸。
5. 确认目录存在、可写，目标扩展名属于允许列表。
6. 临时文件创建在目标目录，随后执行同文件系统原子重命名。
7. 每个 Item 使用互斥锁，避免人工操作与批量任务同时写入。
8. 操作使用幂等 ID，重复请求不能产生额外字幕副本。

允许写入 `.ass`、`.ssa`、`.srt`。其他 Emby 已识别格式可以显示为只读字幕，不应因此被判定为不存在。

备份、回收文件和失败隔离文件保存在媒体库之外的项目数据目录。它们不能使用 Emby 会识别的字幕扩展名留在媒体目录中。

## 7. Installer 行为

### 7.1 Add

Add 生成不冲突的新文件名，不修改已有字幕。文件名冲突时返回候选名称，由服务端按确定规则分配版本号。

### 7.2 Replace

Replace 必须指定一个 `Manageable` 的目标字幕 ID。

```text
取得 Item 锁
↓
重新解析目标字幕
↓
Fetch 或读取上传内容
↓
Validate
↓
写入新版本文件
↓
Emby Refresh 与轮询
↓
验证新字幕内容
↓
归档旧文件
↓
记录历史
```

任何一步失败都保留旧字幕。新文件已经落盘但未被 Emby 接受时，将其移入失败隔离目录或标记为待清理，不能静默删除证据。

### 7.3 Delete

Delete 只接受服务端生成的字幕 ID。内嵌字幕、不可定位字幕和媒体根目录外文件返回明确错误。

默认行为是移动到项目管理的回收目录。永久清理通过独立的保留期任务完成。

### 7.4 Upload

上传限制总大小，验证扩展名与内容格式。上传文件名只用于显示，最终文件名由 NamingService 生成。

## 8. 搜索与预览

V1 搜索请求只接受 ItemID、MediaSourceID 和语言。Provider 标签在结果返回后执行过滤。

```text
ItemID
↓
Emby Remote Subtitle Search
↓
合并候选
↓
按 ProviderName 归类
↓
生成短期 Token
↓
UI 展示
```

预览流程如下。

```text
Candidate Token
↓
解析服务端缓存
↓
Emby Remote Subtitle Fetch
↓
大小和格式校验
↓
ASS / SSA / SRT Parser
↓
Cue[]
```

搜索请求和 Fetch 都设置超时、并发限制及短期缓存。Provider 超时不会永久阻塞任务，候选 Token 到期后要求重新搜索。

## 9. API 规划

```text
GET  /v1/emby/libraries
GET  /v1/emby/items

GET  /v1/media/{itemId}
GET  /v1/media/{itemId}/subtitles

POST /v1/media/{itemId}/subtitles/search
POST /v1/media/{itemId}/subtitles/preview
POST /v1/media/{itemId}/subtitles/add
POST /v1/media/{itemId}/subtitles/replace
POST /v1/media/{itemId}/subtitles/upload
DELETE /v1/media/{itemId}/subtitles/{subtitleId}

GET  /v1/providers
GET  /v1/providers/capabilities
GET  /v1/tasks
GET  /v1/audit-events
```

Search 返回结果时附带 ProviderCapabilities。Refresh 由 Installer 和 Reconciler 内部调用，不作为普通前端按钮的主要接口。

## 10. 配置与凭据

```yaml
emby:
  url: <EMBY_PRIVATE_URL>
  api_key_file: /run/secrets/emby_api_key

subtitle:
  default_language: zh-CN
  backup_before_replace: true
  max_upload_bytes: 10485760

provider_bridge:
  enabled: true
  result_ttl_seconds: 600
  timeout_seconds: 35

path_mappings:
  - emby: /media
    local: /media

verification:
  emby_direct_url: <EMBY_PRIVATE_URL>
  public_url: <PUBLIC_VERIFY_URL>
  verify_public_content: false

automation:
  enabled: false
  min_score: 90
```

API Key、ASSRT Token 和下载地址不进入前端，不写入普通日志。设置页只显示是否已配置和脱敏后的末尾标识。

### 10.1 应用访问安全

V1 默认以单实例运行。多实例部署需要分布式锁和共享任务状态，留到后续版本。

管理界面必须经过登录或现有反向代理认证。所有写接口检查会话权限和 CSRF Token，CORS 使用明确白名单。搜索、预览、上传和写操作分别限流，Candidate Token 与当前用户会话绑定。

项目容器只挂载必要媒体目录。读写媒体库使用独立服务账号，项目数据目录与媒体挂载分开。

### 10.2 状态存储

V1 使用 SQLite 保存操作历史、字幕版本、任务状态和内容 Hash。数据库不保存 Emby API Key 或 ASSRT Token。

候选映射和 PreviewArtifact 使用私有缓存目录与短 TTL。服务重启后旧 Token 全部失效，定时任务清理过期临时文件。备份和回收目录按可配置保留期清理，清理动作单独记录审计事件。

## 11. 开发阶段

### Gate 0　真实链路验证

在大规模改造 ChineseSubFinder 前，先用独立脚本或最小 Go 程序验证当前部署。

必须完成以下工作。

1. 固定 Emby Server、Meiam Thunder、Meiam ASSRT 的实际版本。
2. 选择一个电影 STRM 和一个剧集 STRM。
3. 用计划采用的 API Key 调用 Emby 搜索接口，确认权限并记录脱敏后的状态码和响应字段。
4. Fetch 任意候选并解析实际字幕文本。
5. 证明 STRM 内部的不可达测试 URL 没有收到请求。
6. 记录 Meiam 是否读取 STRM stub 计算 Hash。
7. 确认搜索接口不能按请求指定 Provider 和关键词。
8. 写入一份隔离测试字幕，验证文件系统、Emby 直连和现有代理路径。
9. 验证替换同一路径时是否出现旧字幕缓存。

Gate 0 通过后才能决定是否沿用 Emby Bridge。若 Fetch 不稳定或 Provider 行为无法接受，停止主项目改造，先评估 Native Provider。

### Phase 1　构建基线与代码路线决策

- 在自己的 Fork 中固定 ChineseSubFinder 上游快照
- 固定 Go、Node、包管理器和 Docker 版本
- 完成后端、前端和 Docker 构建
- 统计 CSF Cloud、SubtitleBest、旧扫描器和 Provider Hub 与核心代码的耦合范围
- 记录已停更依赖、许可证、NOTICE 要求和已知漏洞
- 建立 `BASELINE.md`
- 建立一份架构决策记录，比较整仓改造和新建轻量后端两条路线

不得在此阶段升级主框架或清理无关代码。

如果去云端改造需要持续侵入旧扫描器、任务系统和 Provider Hub，应选择新建轻量 Go 后端，只迁移经过测试的 Parser、命名逻辑和必要 UI 组件。若旧 Library 页面和 Emby API 已经能够独立运行，才继续整仓渐进改造。

### Phase 2　只读纵向切片

本阶段按 [ADR-003](docs/adr/003-phase2-milestones-and-deployment.md) 作为 D1 只读 Canary 执行。默认使用 Linux Docker Compose 单应用容器、媒体只读挂载和 `write_enabled=false`，通过私网或 SSH 隧道做实际验收。具体 API、安全边界和门禁见 [D1 验收定义](docs/phase2-readonly-canary.md)。

当前进度：D1 的 Go 代码切片、Linux 全包自动化验证、C92 Docker Compose 部署以及 Movie、Episode STRM 真实 Canary 已完成并记录在 [D1 部署验收报告](docs/d1-deployment-acceptance.md)。Docker Compose schema/build、host-network、UID 10001、只读 root、只读媒体、三份 Secret 权限、`/readyz`、Bearer 401 和版本溯源标签均已通过。真实多媒体源样本尚未找到，自动化 409 与显式 source 选择测试已通过但不能替代真实样本；FRP 公网 HTTPS 也尚未验收。`write_enabled=false` 和 `remote_search_enabled=false` 继续保持关闭，在多源真实样本补齐前不进入 Phase 3。

- 实现 MediaContext 和 PathMapper
- 从 Emby 浏览电影与剧集
- 合并 MediaStreams 和 Sidecar Inventory
- 显示 Embedded、External、Manageable 状态
- 正确识别 STRM

验收需要覆盖一个电影、一个剧集和一个多媒体源 Item。

### Phase 3　远程搜索与预览

本阶段对应 ADR-003 的 D2。只有 D1 的自动化和真实 Canary 验收通过，才允许开始搜索和预览；本阶段仍不写入媒体库。

- 实现 EmbyRemoteSubtitleProvider
- 引入 ProviderCapabilities
- 完成候选 Token、Fetch、Validator 和 Preview
- UI 显示 Provider、格式、语言、Comment 与 Hash Match
- Provider 标签只作为结果筛选

Phase 3 完成后暂停。只有真实 Movie、Episode 和 STRM 验收都通过，才进入写操作。

### Phase 4　安全写入

本阶段先以 ADR-003 的 D3 专用样本 Add 作为第一步。Replace、Delete、Upload、批量处理和其他写操作必须在专用样本验收证据充分后另行开放。

- Add、Replace、Delete、Upload
- Item 锁、幂等操作和原子写入
- 版本归档与操作历史
- Emby Refresh、轮询和内容核验
- 代理缓存兼容策略

验收以客户端能够读取新字幕内容为终点，不能以文件存在或 Refresh 返回成功代替。

### Phase 5　正式 V1 UI

- 媒体库与搜索
- 当前字幕来源和可管理状态
- 在线候选和文本预览
- 添加、替换、删除和上传
- Settings、Provider 状态和错误提示
- 操作历史与恢复入口

### Phase 6　缺字幕与批量管理

- SubtitlePolicy
- Missing、Present、Unmanaged 筛选
- 批量搜索和人工确认
- 队列、限流、取消和失败重试

### Phase 7　评分与保守自动下载

评分先用于排序和解释。项目积累真实匹配样本以后，再启用自动安装。

自动安装需要同时满足这些条件。

```text
State == Missing
CandidateScore >= Threshold
Provider response validated
No concurrent manual operation
```

阈值不能只依赖人为设定的 90 分。发布前需要保存人工选择结果，统计误匹配率并据此校准。

### 独立研究线　时间轴校正

手工 Offset、Scale 和字幕版本保存可以在 V1 稳定后实现。

自动 VAD、语音识别、跨语言字幕对齐和删减版匹配拆成独立研究线。它们不写入 V1 或 V1.1 的交付承诺，也不能阻塞字幕管理器发布。

## 12. 测试和验收

### 单元测试

- MediaContext 与多媒体源选择
- PathMapper 最长前缀和大小写规则
- 字幕语言归一化
- Embedded 与 Sidecar 状态合并
- Naming、Validator 和 Parser
- 路径逃逸、链接逃逸和非法扩展名
- Add、Replace、Delete 的幂等性
- Item 锁与并发冲突

### 集成测试

- Fake Emby 搜索、Fetch、Refresh 和轮询
- 候选 Token 过期与重放
- Provider 超时和部分失败
- STRM 不可达 URL 监测
- 写入失败和 Refresh 失败回滚
- 同路径旧缓存与新路径缓存测试

### 真实验收

每次发布至少选择一部电影和一集剧集，完成下面的闭环。

```text
搜索
↓
预览
↓
添加或替换
↓
Emby 识别
↓
直连读取
↓
实际客户端读取
↓
历史记录可回滚
```

静态测试、构建和 Fake Emby 集成都不能替代真实客户端验收。

## 13. 错误与审计

错误代码按媒体、路径、Provider、字幕、安装和核验分类。错误内容要告诉用户下一步能做什么。

每次写操作记录以下字段。

```text
OperationID
ItemID
MediaSourceID
SubtitleID
Provider
Action
Result
Duration
BeforeHash
AfterHash
InstalledAt
```

日志不记录 API Key、Token、Cookie、原始远程字幕 ID、完整下载 URL和不必要的隐私路径。

## 14. V1 范围

V1 包含 Emby 媒体浏览、字幕清单、STRM、Meiam 搜索桥、文本预览和安全的外挂字幕管理。

V1 不包含原生 Thunder、原生 ASSRT、自动下载、字幕质量升级、自动时间轴校正和全盘定时扫描。

全盘诊断、批量自动化与时间轴研究在 V1 稳定后分别推进。任何后续能力都不能绕过 Installer、路径安全和真实内容核验。

## 15. 最终验收条件

项目达到 V1 可用需要同时满足以下条件。

- CSF Cloud 和 SubtitleBest 不可用时仍能启动和浏览 Emby
- Movie、Episode 和 STRM 都能显示准确的字幕状态
- 内嵌字幕不会被误删，无法管理的字幕有明确标记
- Thunder 与 ASSRT 候选能够搜索、Fetch 和预览
- UI 没有虚构 Provider 控制能力
- 本项目不读取 STRM 内容或访问 STRM URL
- 字幕写入满足路径安全、原子性和并发控制要求
- Replace 失败时旧字幕仍可用
- Emby 直连和实际客户端都能读取新字幕
- 操作历史可以定位来源并恢复上一版本

## 16. 关键参考

- [Emby SubtitleService API](https://dev.emby.media/reference/RestAPI/SubtitleService.html)
- [Emby 远程字幕搜索参数](https://dev.emby.media/reference/RestAPI/SubtitleService/getItemsByIdRemotesearchSubtitlesByLanguage.html)
- [MeiamSubtitles](https://github.com/91270/MeiamSubtitles)
- [Meiam ThunderProvider 当前实现](https://github.com/91270/MeiamSubtitles/blob/master/Emby.MeiamSub.Thunder/ThunderProvider.cs)
- [ChineseSubFinder](https://github.com/ChineseSubFinder/ChineseSubFinder)
