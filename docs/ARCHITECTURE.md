# SubSteward 架构与验证状态

> 状态快照：2026-08-28。本文描述当前 Plugin/API/UI 形态和已记录证据；部署、连接和本机凭据只看未跟踪的 `LOCAL_OPERATIONS.md`。历史部署 Hash 必须重新核对，不能直接当成当前运行状态。

## 1. 运行时与边界

SubSteward 以一个 Emby Plugin 为主运行时。当前主线不包含旧 Go 服务、独立 Web UI、Docker 运行依赖、数据库、history、quarantine 或复杂 recovery 系统。

```text
Emby Server
  └─ SubSteward.Plugin
       ├─ Plugin identity and configuration
       ├─ Embedded admin UI
       ├─ M1 manual API
       └─ M2 quality, preference and conservative action advice
```

当前工程位于 `src/SubSteward.Plugin`，目标框架为 `netstandard2.0`，编译基线为公开的 `MediaBrowser.Server.Core 4.9.1.90`。测试位于 `tests/SubSteward.Tests`，公开测试数据位于 `testdata/subtitles`。

M0 已完成并已从运行时移除。它只用于验证插件加载、服务器管理的数据目录、Item/MediaSource/MediaStream 读取、公开字幕 API 和单 Source STRM 的一次性闭环，不作为额外的产品服务长期安装。

## 2. 管理页面

插件通过 `IHasWebPages` 注册嵌入资源 `SubSteward.Plugin.Web.configPage.html` 和 `SubSteward.Plugin.Web.subSteward.js`，由 Emby 管理界面托管，不引入独立 HTTP 服务或旧 SubBridge Web UI。当前注册页面名为 `SubStewardUI3.html`，控制器名为 `SubStewardUI3.js`，用于避开宿主旧缓存。

页面有三个顶层页签：

- **状态**：显示当前载入范围、目标语言 Presence、Action 分布和待关注条目。Items API 默认返回 50 条，允许范围为 1–100；页面明确标记为当前样本，不冒充全库扫描。
- **手动处理**：确认 Item/source 后完成 `Search → Fetch/Preview → 可选固定偏移对轴 → Install`。候选和 Artifact token 只在页面和插件短期内存中流转，不展示 Provider 原始 ID。
- **设置**：编辑全局默认和媒体库覆盖。覆盖可设置目标语言、第二语言、双语偏好和格式顺序，关闭后恢复继承全局默认。

页面只调用本节 API。多 Source 条目保持 fail-closed，自动全库扫描、批量补全、健康字幕替换和 MultiSource STRM 写入不由 UI 开启。

## 3. 管理员 API 索引

所有路由要求 Emby 管理员认证。响应不包含完整媒体路径、Provider 原始候选 ID 或 Artifact 内容。

| 方法 | 路由 | 输入/输出要点 | 当前边界 |
| --- | --- | --- | --- |
| GET | `/SubSteward/Items` | `SearchTerm`、`Limit`；返回 Movie/Episode 摘要、Source、Presence 和 Action | `Limit` 为 1–100，列表只代表当前样本 |
| GET | `/SubSteward/Items/{Id}` | 返回单 Item 详情、Source、字幕流、Presence、Health、Quality 和 Action | 外置字幕深检有上限；内封正文保持 `UNKNOWN` |
| GET | `/SubSteward/Libraries` | 返回媒体库 ID 和名称 | 不返回媒体路径，仅供设置页选择 |
| GET | `/SubSteward/Subtitles/Search` | `ItemId`、可选 `MediaSourceId`、`Language` | 当前 M1 最终要求 Item 恰好一个 MediaSource；候选最多 20 条 |
| POST | `/SubSteward/Subtitles/Fetch` | `CandidateToken` | 重新绑定 Item/source、Fetch 字节并 Validate；标题/Hash 不匹配时拒绝 |
| GET | `/SubSteward/Subtitles/Preview` | `ArtifactToken` | 返回 Health、编码、格式、Quality、Preference、Action 和最多 200 条 cue |
| POST | `/SubSteward/Subtitles/Align` | `ArtifactToken`、非零 `OffsetMilliseconds` | 只做人工整体对轴；累计偏移前后最多 10 分钟，ASS/SSA 使用 10ms 步进 |
| POST | `/SubSteward/Subtitles/Install` | `ArtifactToken` | 重读 Item/source，写版本化 sidecar，Refresh 并确认新外置 MediaStream |

插件使用的 Emby 内部能力是 `ILibraryManager`、`BaseItem.GetMediaSources`、`ISubtitleManager.SearchSubtitles`、`ISubtitleManager.GetRemoteSubtitles` 和 `IProviderManager.RefreshFullItem`。这些能力不等于旧 SubBridge 的 HTTP API。

## 4. 数据流与安全门禁

```text
Item + MediaSource
       ↓
Presence → Search candidate metadata
       ↓
candidate binding → Fetch bytes → Validate/language gate
       ↓
Preview → Quality/Preference → optional manual Align
       ↓
new versioned sidecar → Refresh → MediaStream → client check
```

### 媒体和 STRM

- `Item`、`MediaSource`、`MediaStream` 是独立事实层，不能因为某个字段为空就猜另一个字段。
- 不读取 `.strm` 内容，不解析其中的远端 URL，也不探测 URL 指向的视频。
- STRM sidecar 以本地 `Item.Path` 对应的 `.strm` 文件目录为锚点；远程 `MediaSource.Path` 只用于播放定位，不能作为写入路径。
- 非 STRM 媒体只有在 Source 是本地普通文件、目录安全且进程具备权限时才允许写入。
- MultiSource STRM 当前只允许明确绑定后的读取、搜索和预览，不自动写入。

### 候选和 Artifact

- Search 结果按 Hash/标题匹配优先展示，但顺序本身不代表质量。
- candidate token 和 artifact token 只保存在插件进程短期内存；原始候选 ID、内容和认证信息不写入响应或日志。
- Fetch 读取上限为 16 MiB。格式/编码/时间轴校验失败、内容为空或中文候选正文没有中文字符时，当前候选失败，不放宽到安装。
- Health 的 `PASS`、`WARNING`、`FAIL` 与 Preference 的 `RECOMMENDED`、`ACCEPTABLE`、`NOT_RECOMMENDED` 分开计算。M2 Action Advisor 只给建议，不执行自动 Repair/Upgrade。

### 外置字幕深检与对轴

- 单条目详情最多深检 8 条外置字幕，只接受与本地媒体锚点同目录的普通文件，单文件读取上限为 16 MiB。
- 内封目标语言只提供 Presence，不提取正文、不 OCR、不深检。
- 对轴支持 SRT、ASS、SSA。SRT 保留毫秒精度，ASS/SSA 以 10ms 为步进；生成新 Artifact 后重新 Validate，不猜测音画偏移。

### 写入与确认

- 写入不覆盖已有字幕，使用新的版本化 sidecar 和临时文件。
- Refresh 成功后还要按同一 Item/source 和目标文件名确认外置 MediaStream。
- 任何失败都要清理本次新建文件；文件存在、Refresh 成功或 MediaStream 出现，都不能单独代替实际客户端读取验收。

## 5. 配置与业务模型

全局配置位于 `PluginConfiguration`：

| 配置 | 默认值 | 说明 |
| --- | --- | --- |
| `TargetLanguage` | `zh-Hans` | 默认简体中文；支持显式选择通用中文或繁体变体 |
| `SecondaryLanguage` | `eng` | 第二语言 |
| `PreferBilingual` | `false` | 是否偏好双语 |
| `FormatOrder` | `ass,ssa,srt` | 格式偏好，逗号和分号均可分隔 |
| `LibraryOverrides` | `[]` | 可启停的媒体库级覆盖 |

`zho/zh/chi`、`eng/en`、`jpn/ja` 和简繁别名会归一化。简繁变体会继续作为标签证据参与 MediaStream、标题和安全文件名判断，但当前没有可靠的正文级简繁识别，因此不能自动替换。

搜索语言码与落盘文件名标签分开处理：调用 Emby 官方 `ISubtitleManager.SearchSubtitles` 时，`zh-Hans` 先归一化为基础语言码 `zho`，再由 Emby/Provider 映射为各自的 `chi` 或 `zh`；请求变体单独保留。写入外置字幕时遵循 [Emby 字幕命名说明](https://emby.media/support/articles/Subtitles.html)，简体使用 `zh-CN`、繁体使用 `zh-TW`，其他无地区变体语言使用对应 ISO 语言码。当前不追加 `.default`、`.forced` 或 `.sdh`，默认选择交由 Emby 用户字幕设置调整。

业务判断固定为：

```text
Presence → Health → Preference → Action
```

目标存在但 Health 未测量时保守返回 `MANUAL`；多 Source、状态未知、候选 `WARNING` 或低置信度双语也返回 `MANUAL`。缺少目标语言且没有可用候选时返回 `SEARCH`；目标存在且 Health 为 `PASS` 时返回 `KEEP`。`REPAIR` 和 `UPGRADE` 目前只保留为动作枚举。

## 6. 验证状态

### 已有基线证据

- 本机已记录 Release 编译和基础测试通过。
- C92 Emby 4.9.5.0 曾成功加载插件并发现手动任务；单 Source STRM 样本曾完成 Search、Fetch/Preview、Install、Refresh、Emby 新字幕流识别和实际客户端播放验收。
- 该基线还发现过错误候选和一次错误语言标记，因此当前实现增加了标题/Hash 绑定、正文语言门禁、候选隔离和最终 MediaStream 对账。

### 带时间的 C92 记录

以下记录只证明对应时间和对应修订的局部证据，不能合并成“当前版本全部通过”。

| 日期 | 修订/内容 | 已验证 | 未验证或限制 |
| --- | --- | --- | --- |
| 2026-08-27 | Action 版本，Release DLL SHA-256 `F118F30AB8AA36904ADC77B64F475188266FCABFD12F133EC59918193AE952C9` | Items、Item 详情、Search、候选失败隔离、Fetch、Preview；容器重启后运行 | 未执行 Install；线上缺失目标的 `SEARCH` 分支仍由本地 Action 测试覆盖 |
| 2026-08-27 | 嵌入式 UI 修正版，Release DLL SHA-256 `378279EAAF0113731A39F2B6987DEA6A36EDE1A10067BE64DD378718BA83AA4A` | 管理员会话下单实例、CSS、100 条读取、Presence/详情和控制台无错误 | 未执行 Provider Search/Fetch 或 Install |
| 2026-08-28 | 三页签 UI、人工对轴和移动端聚焦，Release DLL SHA-256 `2B9E630C9395D34EDD2145E6F00D351FCE63C111B0E6CAE5505161B5D158E8BE` | Items、Libraries、配置、UI3 资源 HTTP 200；无效 Artifact token 返回 400；本地模拟 Items → Search → Fetch → Align → 撤销；375px 无横向溢出 | 未执行真实 Provider Fetch、Align Artifact 安装或媒体写入；线上视觉截图受应用内浏览器访问策略限制 |
| 2026-08-28 | Emby 白底配色修正版，Release DLL SHA-256 `ADF35DE8F2E369C0F01AB8E6AF7369DC50F5D6DFE9384C4AF3DA155523C83E2E` | 覆盖前 Hash 已备份；宿主与容器内 DLL Hash 一致；只重启 `emby-server` | 该最新修订尚未完成认证管理员 API、线上视觉、Provider Fetch、Install 或媒体写入复验 |
| 2026-08-28 | 当前工作树本机验证（部署前，未提交），Release DLL SHA-256 `BE006FEC3107DD07E36C2B33721036078ABB4BE43861A2AE4A04D81AE382E88DD` | 本机 Release build 0 警告/0 错误；Release 测试程序集 68/68 通过；Web JavaScript `node --check` 通过；MediaStream 路径无法规范化时按不匹配处理 | 当时尚未部署 C92；最新修订的插件加载、管理员 API、Provider Search/Fetch、Align/Install、Refresh/MediaStream 和客户端播放均未复验 |
| 2026-08-28 | 当前工作树部署到 C92，Release DLL SHA-256 `BE006FEC3107DD07E36C2B33721036078ABB4BE43861A2AE4A04D81AE382E88DD` | 旧 DLL 备份 Hash 为 `1EA1F0473ACF4DE070172E3AB780ECF9344F2187AE6418896CC37B1324E23BFE`；新 DLL 远端 Hash 与本地一致；只重启 `emby-server`；容器恢复 `running` 且重启次数为 0；认证管理员 API `/SubSteward/Libraries`、`/SubSteward/Items?Limit=1` 返回 200；Emby `ConfigurationPage` 的 HTML/JS 资源返回 200 且关键页面/接口标记存在 | 未执行 Provider Search/Fetch、Align/Install、Refresh/MediaStream 和真实客户端播放；这些步骤需要明确的媒体样本与后续验收授权 |
| 2026-08-28 | C92 真实样本“千与千寻”首次安装（通用语言码） | 单 Source STRM、无既有外置字幕；Provider Search 返回 20 条候选；候选 12 的 ASS 结构校验失败，候选 1 的 English.srt 触发中文正文门禁并被拒绝；候选 2 经 Fetch/Preview 校验为 ASS、UTF-8 BOM、Health PASS、中文覆盖约 89.7%、Preference RECOMMENDED；Install 返回 200，生成 `千与千寻.2001.中日双语.zho.ass`，Refresh 后同一 Source 外置字幕数为 1 | 未执行人工 Align；后续已用地区码命名修正版迁移该样本 |
| 2026-08-28 | C92 真实样本“千与千寻”地区码修正，Release DLL SHA-256 `E0C6261CA796009D71DF9F027E11F47B5A450296001F6F8800921DE03F0ACD81` | C92 `TargetLanguage=zh-Hans`；默认 Search 返回 `Language=zho`、`RequestedLanguageVariant=zh-Hans`；候选 2 经 Fetch/Preview/Install 返回 200，生成 `千与千寻.2001.中日双语.zh-CN.ass`，未添加 `.default`；旧 `.zho.ass` 与新文件内容 Hash 一致后改为退役备份名；官方 Item Refresh 返回 204；同一 Source 保持 1 条外置字幕，条目详情识别为中文（简体）/ASS/PASS；地区码修正后的真实客户端播放已由用户确认通过 | 未执行人工 Align |

因此当前最准确的状态是：M1 基线能力和当前样本的客户端验收已有证据；当前工作树已通过本机 build/test，完成 C92 插件、API、管理页面烟测，以及“千与千寻”的 Provider、地区码命名、安装、Refresh、MediaStream 对账和地区码修正后的真实客户端播放确认。人工 Align 后再安装的真实链路仍未单独验收。

## 7. 当前收口边界

- 正文级简繁识别仍不足以驱动自动替换。
- 正文级语言检测目前重点覆盖中文、英语和日语，其他第二语言主要保留配置和 Presence 语言码。
- ASS 的 Script Info、Styles、残缺 HTML 和更深层结构校验仍较浅。
- Preference 支持目标语言、第二语言、双语开关和格式顺序；用户自定义权重以及普通/样式化/高特效偏好尚未纳入。
- `PreferenceAnalyzer` 支持已 Fetch 候选的纯计算排序，但服务入口没有接入大规模候选的批量 Deep Ranking。
- 上述限制不改变 fail-closed 规则，也不授权自动 Repair、Upgrade 或 MultiSource STRM 写入。
