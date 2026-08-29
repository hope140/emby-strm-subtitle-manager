# SubSteward 架构与验证状态

> 状态快照：2026-08-29。本文描述当前 Plugin/API/UI 形态和已记录证据；部署、连接和本机凭据只看未跟踪的 `LOCAL_OPERATIONS.md`。历史部署 Hash 必须重新核对，不能直接当成当前运行状态。

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

插件通过 `IHasWebPages` 注册嵌入资源 `SubSteward.Plugin.Web.configPage.html` 和 `SubSteward.Plugin.Web.subSteward.js`，由 Emby 管理界面托管，不引入独立 HTTP 服务或旧 SubBridge Web UI。当前主页面名为 `SubStewardUI8.html`，控制器名为 `SubStewardUI8.js`，用于避开宿主旧缓存；同时保留 UI3–UI7 兼容别名，避免已经打开的旧页面直接失效。

页面有四个顶层页签，采用现代控制台的信息层级和较克制的 Emby 表面样式：

- **概况**：通过独立 Summary API 显示当前媒体库/筛选范围的完整计数、目标语言存在性、中文动作建议和待关注条目；不再把当前页当作全库。自动化摘要在功能未开放时只显示“尚未启用”，不生成虚假批次或成功率。
- **自动化**：当前是明确标注的规划中页面，解释未来的批次结果、完成/跳过/失败/人工判断分类和折叠技术日志；它不启动扫描、后台任务或媒体写入。
- **手动检查**：先选择媒体库和每页条数；指定媒体库后按 `剧 → 季 → 集` 逐层钻取，电影库直接显示电影，电影/集才进入字幕处理。Items API 仍提供全库分页和名称搜索，随后完成 `Search → Fetch/Preview → 可选固定偏移对轴 → Install`。页面以中文主标签解释存在性、健康、偏好和建议动作，内部英文状态码只作为次要标记。候选和 Artifact token 只在页面和插件短期内存中流转，不展示 Provider 原始 ID。
- **设置**：编辑全局默认和媒体库覆盖。覆盖可设置目标语言、第二语言、双语偏好和格式顺序，关闭后恢复继承全局默认。

页面只调用本节 API。多 Source 条目保持 fail-closed，自动全库扫描、批量补全、健康字幕替换和 MultiSource STRM 写入不由 UI 开启。

插件业务日志通过 Emby 的 `ILogManager` 写入宿主日志，前缀为 `[SubSteward]`。日志覆盖 Items/Summary/Browse、Search、Fetch、Preview、Align 和 Install 的开始、结果、质量计数、匹配状态与失败阶段；不写入候选原始 ID、candidate/artifact token、字幕正文、认证信息或完整媒体路径。

## 3. 管理员 API 索引

所有路由要求 Emby 管理员认证。响应不包含完整媒体路径、Provider 原始候选 ID 或 Artifact 内容。

| 方法 | 路由 | 输入/输出要点 | 当前边界 |
| --- | --- | --- | --- |
| GET | `/SubSteward/Items` | `SearchTerm`、可选 `LibraryId`、`Page` 或 `Offset`、`PageSize`；返回 `items`、`page`、`offset`、`pageSize`、`totalCount` 和 Movie/Episode 摘要 | 每页为 1–100；筛选和排序明确，列表不再受首 100 条限制 |
| GET | `/SubSteward/Summary` | 可选 `SearchTerm`、`LibraryId`；返回完整范围的总数、目标语言 Presence、MultiSource、人工判断数与 Action 分布 | 只汇总状态，不开启自动扫描、Fetch 或媒体写入 |
| GET | `/SubSteward/Browse` | 必填 `LibraryId`，可选 `ParentId`、`SearchTerm`、`Page`/`Offset`、`PageSize`；返回 `Series → Season → Episode` 节点及分页元数据 | 根层显示 Series/Movie；只有 Series 和 Season 可继续展开，Movie/Episode 进入单项处理 |
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
| 2026-08-28 | 全库分页/摘要与人工工作台改造部署，Release DLL SHA-256 `F23AC5F3288BDAAF88F3AD38B3AED27DFA6A6763A0EFEEB7DEDD57C2125A17E6` | 本地 Release build 0 警告/0 错误、测试 68/68、Web JavaScript `node --check` 通过；C92 覆盖前 DLL 已备份且校验一致；远端 DLL 大小 204800 字节、Hash 与本地一致；只重启 `emby-server`，容器保持 `running` 且自动重启次数为 0；启动日志确认 `SubSteward.Plugin 0.1.0.0` 已加载；未认证探针访问 `Summary`、`Items`、`Libraries` 均返回 401 | 尚未用管理员会话验证新的 `totalCount`/分页响应 200；未做线上视觉、真实 Provider Fetch/Install、Refresh/MediaStream 和客户端播放复验 |
| 2026-08-28 | C92 全库分页/媒体库筛选修正版与“千与千寻”人工验收，Release DLL SHA-256 `9346A7837A1958D02D152B6196B73002895FB73F6138988C34AE138B686BA7C8` | 本地 Release build 0 警告/0 错误、测试 68/68、Web JavaScript `node --check` 通过；C92 只重启 `emby-server`，容器 `running` 且自动重启次数为 0，远端 DLL Hash 与本地一致并确认加载；管理员 API：`Libraries` 200（12 个库）、全库 `Items` 200（`totalCount=7322`），第 2 页与 `offset=100` 200，`动画电影` `LibraryId=24232` 筛选和 Summary 200（`totalCount=100`）；“千与千寻”列表、详情、Provider Search 200；候选 14 经 Fetch/Preview 200，ASS、UTF-8 BOM、Health PASS、简体正文覆盖 99%、Preference ACCEPTABLE，因 Hash 未匹配由 Action 返回 `MANUAL`；用户确认后 Install 200，生成 `千与千寻.2001.中文简体.zh-CN.ass`，Refresh 后同一 Source 外置字幕流为 2 条；原有 `千与千寻.2001.中日双语.zh-CN.ass` 保留，新增 sidecar 存在且无残留临时文件 | 未重新做客户端播放确认；候选 11 的 ASS 结构校验失败并按候选隔离拒绝，候选 14 的人工安装属于用户明确授权的样本验收，不代表批量自动替换已开放 |
| 2026-08-28 | C92 UI4 缓存失效兼容修正版，Release DLL SHA-256 `D0599ED58234EE952BF3CDAA45012BC2B76F2920B07DC934670B5C3C6DAC90B6` | 修正页面资源名称不变导致浏览器继续使用旧 JS、旧 JS 将分页对象当数组而显示空白的问题；主资源 `SubStewardUI4.html/js` 与 UI3 兼容别名均 HTTP 200，资源包含分页、全库摘要和媒体库筛选标记；新控件在旧 HTML 缺失时安全跳过绑定；本地 Release build 0 警告/0 错误、测试 68/68、Web JavaScript `node --check` 通过；C92 只重启 `emby-server`，容器 `running` 且自动重启次数为 0，远端 DLL Hash 与本地一致并确认加载 | 需要用户重新打开插件页或执行一次硬刷新后确认可视页面；管理员 API 与千与千寻媒体状态已在上一修订和当前最终 DLL 上复验，客户端播放仍未重新确认 |
| 2026-08-28 | C92 下载状态与左侧可读性修正版，Release DLL SHA-256 `7ADA2125A8F3C2AD7D25E3079655601FAC51338273EAE8C7A9D4A27F9D45AB31` | 依据“小时代2：青木时代”现场截图修正：左侧媒体/元数据不再继承 Emby 暗色主题的浅色变量；候选 Fetch 同时请求改为单候选锁定，按钮显示最多 60 秒等待，超时、HTTP 429 与 Provider 校验失败分别给出恢复提示并允许重试/换候选；离开条目或切换来源时丢弃过期响应；本地 Release build 0 警告/0 错误、测试 68/68、Web JavaScript `node --check` 通过；远端 DLL Hash 与本地一致，UI4 HTML/JS HTTP 200，容器 `running` 且自动重启次数为 0 | 现场验证的前三个 Thunder 候选未出现 HTTP 429，候选 0/1/2 分别快速返回校验失败；当前尚未用用户浏览器重新确认视觉截图，客户端播放不在本次修正范围 |
| 2026-08-28 | C92 UI5 缓存失效部署，Release DLL SHA-256 `C8DFD18B20C94966BB853DC65A57D4A8CE32199249B2817F986A7D7A4F04E1F9` | 主页面/控制器版本从 UI4 升级到 UI5，保留 UI3/UI4 兼容别名，避免固定资源名缓存旧页面；UI5/旧别名资源均 HTTP 200，容器 `running` 且自动重启次数为 0，远端 DLL Hash 与本地一致并确认加载；管理员 API 回归 `小时代2：青木时代` 200、Summary 200、Libraries 200 | 需要用户重新打开插件页确认左侧高对比文字和候选下载状态提示已生效 |
| 2026-08-28 | C92 UI7 层级浏览、双语误判修正、候选源拦截与业务日志，Release DLL SHA-256 `E9B7D12EF7551ECF187397C050F68FA65179BF00ED421C1E87BA81B453E12C8B` | 主页面/控制器升级到 UI7，保留 UI3/UI4/UI5/UI6 兼容别名；本地 Release build 0 警告/0 错误、测试 74/74、Web JavaScript `node --check` 通过；`/SubSteward/Browse` 实测 `国产剧` 根层 `Series` → `Season` → `Episode` 分页均 200 且按编号排序；`小时代3：刺金时代` 的 Bilibili clip 候选标记为疑似非完整来源，Fetch 在调用 Provider 前拒绝；宿主日志已出现 `[SubSteward]` 的 Browse、Items、Search、Fetch 阶段及拒绝原因；UI7 HTML/JS HTTP 200；未执行错误候选 Install | 需要用户重新打开插件页确认实际层级视觉；客户端播放不在本次范围 |
| 2026-08-29 | C92 UI8 B2 控制台与中文术语修正版，Release DLL SHA-256 `879BEF98A4B88420F718665F63DA503091D7E99CD1363E3510DF9BB0061F1030` | 保留 UI3–UI7 兼容别名；本地 Release build 0 警告/0 错误、测试 74/74、Web JavaScript `node --check` 和 `git diff --check` 通过；覆盖前 UI7 DLL Hash `E9B7D12EF7551ECF187397C050F68FA65179BF00ED421C1E87BA81B453E12C8B` 已做时间戳备份；远端 DLL 大小 263168 字节且 Hash 与本地一致；只重启 `emby-server`，容器 `running`、自动重启次数 0，日志确认插件加载和 `Core startup complete`；认证 UI8 HTML/JS、Libraries、Items 和 Summary 均返回 200，Items/Summary `totalCount=7356`，资源包含自动化未启用状态、手动检查和中文动作标签 | 未执行 Provider Search/Fetch、Align/Install、Refresh/MediaStream 或媒体写入；HTTP 200 不代表页面视觉已验收，需要管理员在 Emby 中打开插件页确认布局与交互 |

因此当前最准确的状态是：M1 基线能力和当前样本的客户端验收已有证据；UI8 工作树已通过本机 build/test，完成 C92 插件、只读 API 和管理页面资源烟测，但新版 B2 布局仍等待管理员浏览器视觉确认。此前“千与千寻”的 Provider、地区码命名、安装、Refresh、MediaStream 对账和地区码修正后的真实客户端播放证据不等于本次 UI 改造重新执行了媒体验收；人工 Align 后再安装的真实链路仍未单独验收。

## 7. 当前收口边界

- 正文级简繁识别仍不足以驱动自动替换。
- 正文级语言检测目前重点覆盖中文、英语和日语，其他第二语言主要保留配置和 Presence 语言码。
- ASS 的 Script Info、Styles、残缺 HTML 和更深层结构校验仍较浅。
- Preference 支持目标语言、第二语言、双语开关和格式顺序；用户自定义权重以及普通/样式化/高特效偏好尚未纳入。
- `PreferenceAnalyzer` 支持已 Fetch 候选的纯计算排序，但服务入口没有接入大规模候选的批量 Deep Ranking。
- 上述限制不改变 fail-closed 规则，也不授权自动 Repair、Upgrade 或 MultiSource STRM 写入。
