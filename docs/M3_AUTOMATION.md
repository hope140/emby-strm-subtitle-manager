# M3 自动补缺方案与进度

> 状态快照：2026-08-29。项目当前暂时搁置。本文保留暂停前 M3 的实施口径和验证证据，不等同于当前目标 Emby 的部署或上线状态。

## 1. M3 目标

M3 只自动处理明确授权媒体库中、单 Source 且缺少目标语言字幕的 Movie/Episode。它复用 M1 的 Provider Search/Fetch、M2 的 Health/Quality/Preference 分析和版本化 sidecar 写入，但把自动执行门槛收紧到可以解释和复核的范围。

M3 不负责修复已有字幕、不替换健康字幕、不处理 MultiSource STRM。低置信度双语证据不能触发替换或覆盖其他门禁；M3 只在目标缺失且候选已经通过整体 Preference 评估时继续补缺。

## 2. 当前已实现

- `M3SubtitleAutomationTask` 已接入 Emby `IScheduledTask`，默认每 24 小时触发一次。
- `AutomationEnabled` 默认关闭；未开启时任务直接结束，不查询媒体、不调用 Provider、不写文件。
- `AutomationLibraryIds` 是显式媒体库白名单，空白或无法解析时直接结束，不回退到全库。
- `AutomationDryRun` 默认开启。dry-run 会完成条目门禁、Search、有限 Fetch、Validate、Quality 和 Preference，但不写媒体。
- 每次运行最多检查 20 个 Movie/Episode；每个条目最多进行 3 次 Provider Fetch，搜索结果最多保留前 20 条。
- 条目必须满足单 Source、Source 有稳定 ID、目标语言明确缺失，以及本地安全写入锚点存在。
- STRM 不读取 `.strm` 内容，也不计算 STRM 文件哈希。自动候选必须有标题匹配加年份或集数等结构化媒体对应，自动匹配分数为 85；普通本地媒体如果有 Provider 媒体指纹则计为 100。只有标题匹配但没有结构化信息的候选继续保留给人工流程，不自动 Fetch/Install。
- 候选必须通过语言/变体门禁、正文目标语言检测、Health `PASS` 和 Preference `RECOMMENDED`。低置信度双语证据不能触发替换；M3 只处理缺失字幕，不以此阻断已经通过整体 Preference 评估的补缺候选。
- 自动对轴不要求媒体目录已有外置参考字幕。一次运行会在内存中 Fetch 最多 3 个候选，用同语言对白文本或跨语言对白序列两两比较，建立候选时间轴共识；只从共识组中选择最佳候选。候选不足、只有单个可校验候选、偏移不稳定或出现漂移时转“需人工”。STRM 的 Provider 哈希不参与媒体对应、排序或偏好加分。
- 成功写入使用新的版本化 sidecar，随后 Refresh，并按同一 Source 和目标文件名确认 MediaStream；确认失败会清理本次创建的文件。
- 每个条目返回“已完成”“已跳过”“失败”或“需人工”结果；API 的 `Status` 使用中文，`StatusCode` 保留机器码。任务日志只记录阶段、计数和安全摘要，不记录 Provider 原始 ID、Token、字幕正文或完整媒体路径。
- 已增加 API-first 入口 `GET /SubSteward/Automation` 和 `POST /SubSteward/Automation/Run`。运行接口只能在持久化 `AutomationEnabled=true` 后执行；请求不能把持久化的 `AutomationDryRun=true` 临时改成正式写入。
- 最近一次运行只在进程内保存紧凑摘要，服务重启后清空，不形成 history 或 recovery 数据库。

### C92 dry-run 证据

2026-08-29 已将 STRM 对应规则和中文状态修正版 M3 DLL 部署到 C92。动画电影库 dry-run 扫描 100 个条目，验证了缺失识别、候选失败隔离和中文结果状态，但没有合格候选。随后对华语电影库 dry-run 扫描 52 个条目，结果为 42 个已跳过、10 个需人工、0 个失败、0 个已完成；“妖猫传”按 STRM 标题+年份匹配分 85，第二候选 Health `PASS`、目标语言覆盖 99%、Preference `RECOMMENDED`，被 M3 自动链路接受。之后通过一次性 `ItemId` 定向正式运行，只扫描并安装“妖猫传”一条，结果为 1 个已完成、0 个已跳过、0 个失败、0 个需人工；新 `.zh-CN.ass` 文件存在且临时文件为 0，插件详情确认同一 Source 的外置 `ASS`、`UTF-8 BOM`、Health `PASS`，Action 为 `KEEP`。Emby 注册的 `SubSteward 自动补缺` 任务默认间隔 24 小时，API 的 `Status` 使用中文并保留 `StatusCode`。验收结束后已将 `AutomationEnabled` 恢复为 `false`，`AutomationDryRun` 保持 `true`，`AutomationLibraryIds` 恢复为空，单次上限恢复为 20；客户端播放仍待确认。

随后将候选互相对照版本部署到 C92，Release DLL SHA-256 为 `FEA3BE2769F694134DF987ACF367A88A8F6851F1A52DDD5BFE27DF696897B16F`。在“外语电影”库对《惊天魔盗团3》进行单条目 dry-run，条目满足单 Source、目标字幕缺失和年份对应，M3 按最多 3 次 Fetch 收集候选；运行摘要为 1 个扫描、1 个需人工、0 个写入，`SynchronizationDriftCount=1`，原因是“已抓取候选字幕之间存在时间漂移，无法建立稳定共识”。这证明没有已有外置字幕时也会执行候选间对轴，并在相对时间轴不稳定时停止。验收后已再次恢复关闭、dry-run、空白白名单，C92 容器保持 `running`、`restartCount=0`，任务保持 `Idle`。

## 3. M3 运行流程

```text
Emby Scheduled Task
        ↓
全局开关 → 媒体库白名单 → 条目数量上限
        ↓
单 Source → 目标语言缺失 → Source/写入路径安全
        ↓
Provider Search → STRM 标题/年份/集数匹配 → 最多 3 次 Fetch
        ↓
Validate → Health PASS → 正文语言 → Preference RECOMMENDED
        ↓
候选之间文本/序列对照 → 稳定时间轴共识 → 固定偏移 → 重新校验
        ↓
dry-run 结果，或版本化 sidecar → Refresh → MediaStream 对账
```

## 4. 配置口径

| 配置 | 默认值 | 规则 |
| --- | --- | --- |
| `AutomationEnabled` | `false` | 总开关。关闭时不读取媒体条目。 |
| `AutomationDryRun` | `true` | 校验候选但不写入媒体；只有明确切换为 `false` 才允许 Install。 |
| `AutomationLibraryIds` | `[]` | 媒体库白名单。空列表 fail-closed。 |
| `AutomationMaxItemsPerRun` | `20` | 每次最多检查 100 个，默认 20。 |
| `AutomationMaxCandidateFetchesPerItem` | `3` | 每条最多 3 次 Fetch，代码硬上限为 3；对轴共识至少需要 2 个可校验候选。 |

媒体库级 `LibraryOverrides` 仍只负责目标语言、第二语言、双语偏好和格式顺序。它的 `Enabled` 不等同于 M3 自动化授权，自动化授权必须单独出现在 `AutomationLibraryIds` 中。

API-first 入口如下：

| 方法 | 路由 | 用途 | 安全边界 |
| --- | --- | --- | --- |
| GET | `/SubSteward/Automation` | 查看 M3 配置摘要和最近一次运行摘要 | 管理员认证；不返回媒体路径、Provider 原始 ID 或字幕正文 |
| POST | `/SubSteward/Automation/Run` | 立即执行一次 M3 任务，可选 `DryRun=true` 和单次 `ItemId` | 必须已开启持久化总开关；单次条目必须属于授权白名单；不能用请求参数把持久化 dry-run 临时改成正式写入 |

## 5. 结果分类

| 结果 | 含义 |
| --- | --- |
| 已完成（`COMPLETED`） | 已写入新 sidecar，Refresh 后确认同一 Source 的字幕流存在。 |
| 已跳过（`SKIPPED`） | 已有目标字幕、dry-run、没有候选，或没有候选通过自动门禁。 |
| 失败（`FAILED`） | Provider Search、Fetch、Install 或 Refresh/MediaStream 对账发生运行时失败。 |
| 需人工（`MANUAL`） | 多 Source、状态不明、写入锚点不安全、STRM 缺少标题/年份/集数等结构化信息，候选之间无法形成稳定时间轴共识，或候选需要人工判断。 |

## 6. 明确不做的事

- 不读取 `.strm` 文件内容，也不解析其中的远端 URL。
- 不对已有健康字幕做替换，不自动执行 `REPAIR` 或 `UPGRADE`。
- 不对 MultiSource STRM 自动写入。
- 不使用 Provider 返回顺序代替媒体对应、语言和正文门禁；不把 STRM 文件哈希当作视频匹配。
- 不要求已有外置字幕作为自动对轴前提；自动对轴只使用本次运行中已 Fetch 且通过基本校验的候选。多候选共识只能发现相对错位，不能证明所有候选相对视频的绝对偏移都正确。
- 不读取音频或视频做波形/对白对齐。
- 不因为 Provider Search 成功就认为字幕可安装。
- 不在 UI 有问题时增加一个可以绕过后端门禁的“强制自动化”按钮。
- 不把 M3 运行结果永久写入数据库或重新引入旧 SubBridge 的 history/quarantine/recovery 平台。当前结果先由任务日志和内存返回值承载，持久化记录待真实运行需求确认后再做。

## 7. 当前剩余工作（暂停）

以下事项保留为恢复后的待办，暂停期间不继续执行：

### P0：本地完成 M3 后端闭环

1. 继续补充候选隔离、dry-run、安装失败清理、重复运行和取消任务的单元测试。
2. 增加 runner 级成功安装、Refresh/MediaStream 对账失败清理和并发互斥测试。
3. 在不改变后端门禁的前提下，决定是否需要保存更多可追溯的单条目结果。

### P1：目标 Emby dry-run 验收

动画电影库和华语电影库 dry-run 已完成白名单、范围扫描、目标缺失识别、STRM 标题+年份对应、3 次 Fetch 上限、Health/Preference 失败隔离和中文结果状态；“妖猫传”已完成历史单样本正式安装。候选互相对照版本已在“外语电影”库完成真实 dry-run，并成功识别候选间时间漂移后转人工；目前仍没有稳定共识候选的正式安装证据，不扩大批量自动化范围。

### P2：单样本真实安装验收

“妖猫传”的历史单 Source 安装、Refresh、同 Source MediaStream 对账和临时文件清理已完成；候选互相对照版本尚未执行正式写入。剩余验收项是找到稳定共识候选后完成单样本正式安装，并由指定客户端确认播放；通过后再讨论扩大每次运行数量。

### P3：再修 UI

UI 只需要补充 M3 配置、dry-run/正式运行提示、最近一次摘要和人工结果入口。UI 不重新定义门禁，也不能直接复制一套候选选择逻辑。

项目暂停前已清理 C92 上的 SubSteward 插件 DLL 及其部署备份；媒体目录未纳入清理范围。恢复时必须重新取得部署、重启、Provider 请求和媒体写入授权，并重新验证目标 Emby。

## 8. M3 完成标准

M3 只有在以下证据都具备后才能称为完成：

1. 本地构建和自动化门禁测试通过。
2. 默认配置不会查询、Fetch 或写入。
3. dry-run 在目标 Emby 上能对授权媒体库完成有限候选验证，并能解释跳过、失败、人工和候选时间轴漂移结果。
4. 单样本正式运行完成写入、Refresh、MediaStream 对账和指定客户端播放验收。
5. 未通过门禁的候选和复杂边界不会被自动安装，失败不会留下本次创建的正式文件或临时文件。
