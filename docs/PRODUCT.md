# SubSteward 产品说明

> Subtitle Automation for Emby

## 一句话定义

SubSteward 是一个独立运行在 Emby 内的字幕自动化插件，负责发现目标语言字幕、判断字幕是否健康、按用户偏好选择候选，并在明确授权后完成补全、修复或升级。

它的最终目标不是复刻旧 SubBridge，而是让用户在 Emby 中稳定获得一份能正常播放、没有明显错误、尽量符合个人偏好的字幕。

## 用户最终得到什么

用户可以配置：

- 目标语言，默认简体中文。
- 第二语言，例如英语或日语。
- 是否偏好双语字幕。
- 字幕格式优先级，默认 `ASS > SSA > SRT`，但默认保持原格式。
- 是否偏好特效字幕、指定 Provider 或 Hash 匹配。
- 是否开启自动补缺，以及允许处理的媒体库范围。

SubSteward 对每个视频最终给出清晰状态和动作建议，而不是只返回一个搜索结果：

```text
Presence → Health → Preference → Action
```

动作统一为：

```text
KEEP / REPAIR / SEARCH / UPGRADE / MANUAL / IGNORE
```

## 三种实际使用场景

### 没有目标语言字幕

在用户授权的范围内：

```text
发现缺失 → 搜索候选 → Fetch → 质量校验 → 偏好排序 → 安装 → Refresh
```

只有候选与目标 Item/source 绑定、Health 通过且写入目标明确时，才允许进入安装。

### 已有外挂字幕

先判断字幕是否影响观看，再判断是否符合偏好：

```text
Health → Preference → 状态与建议
```

“不是双语”“不是 ASS”“没有特效”属于 Preference，不应直接判为字幕损坏。已有健康字幕在自动化早期不自动替换。

### 已有内封目标语言字幕

默认只用于 Presence：

```text
存在目标语言内封字幕 → PASS → KEEP
```

默认不提取、不 OCR、不深检、不重封装 MKV。是否继续搜索外挂字幕可以作为用户配置，但默认关闭。

## 四层业务模型

### Presence

只回答“目标语言字幕是否存在”。内封和外挂目标语言字幕都可以使 Presence 通过，但 Presence 不代表字幕内容一定健康。

### Health

只报告影响正常观看的明确问题，状态为 `PASS`、`WARNING`、`FAIL`。

计划覆盖：

- UTF-8、UTF-8 BOM、UTF-16，以及实际样本需要的 GBK/GB18030 编码识别。
- SRT 的解析、时间轴、空字幕、编号、控制字符和明显乱码。
- ASS/SSA 的 Script Info、Styles、Events、Format、Dialogue、时间轴和明显损坏的 override tag。
- 残缺 HTML 标签、替换字符、NUL 和确定性的坏字符。

高确定性问题才自动修复；不确定的内容只报告 `WARNING`，不擅自改写字幕正文。

### Preference

只在 Health 合格后评估：

- 目标语言覆盖率和第二语言覆盖率。
- 双语 cue 比例和检测置信度。
- ASS/SSA/SRT 格式偏好。
- 普通、样式化或高特效字幕偏好。
- Provider、Hash 和文件名匹配。
- 用户自定义权重。

低置信度的双语判断只能用于人工建议，不能单独触发自动替换。

### Action

根据 Presence、Health 和 Preference 统一给出 `KEEP`、`REPAIR`、`SEARCH`、`UPGRADE`、`MANUAL` 或 `IGNORE`。人工 API、后台任务和未来自动化必须使用同一套判断逻辑。

当前 M2 的 Action Advisor 只输出保守建议，不执行安装、替换、Repair 或 Upgrade。单 Source 且目标字幕存在、已有 Health 明确为 `PASS` 时返回 `KEEP`；目标缺失且没有可用候选时返回 `SEARCH`；多 Source、状态不明、候选 `WARNING` 或双语判断置信度不足时返回 `MANUAL`。候选没有标题或 Hash 绑定，或 Health 为 `FAIL`、Preference 为 `NOT_RECOMMENDED` 时继续 `SEARCH`。Item Presence 本身不等于 Health，因此仅凭 Item 详情发现已有目标流时，Health 未知会保守返回 `MANUAL`。

## 候选搜索与选择

候选选择采用两阶段：

1. Metadata Ranking：Provider、名称、语言、格式、Hash、Provider 分数和文件名关键词排序。
2. Deep Ranking：Fetch 后检查 Health、实际语言、双语、特效、编码、格式和内容问题。

Health FAIL 的候选直接淘汰。Provider 返回的语言字段不能代替正文语言检查；中文候选必须经过正文门禁，避免“标称中文但正文是英语或其他影片”的错误写入。初期只 Fetch 有限数量的候选，默认上限为 3。

候选原始 ID 和 Artifact 内容只在插件进程短期保存，不写入响应、日志或仓库。

## 自动化路线

自动化必须逐阶段开放：

| 阶段 | 目标 |
|---|---|
| M0 | 验证 Emby Plugin 架构、公开 API、Item/source/stream 读取和一次授权写入闭环。 |
| M1 | 提供人工 `Search → Fetch → Preview → Validate → Install → Refresh` 闭环。 |
| M2 | 完成字幕 Health、语言/双语检测、ASS 特效分析和 Preference 排序；主要用于展示、推荐和人工选择。 |
| M3 | 只对明确授权媒体库中、单 Source、缺少目标语言字幕的媒体做保守自动补缺。 |
| M4 | 在积累真实误判、候选选择和播放反馈后，再评估自动 Repair 与 Upgrade。 |

M3 的自动补缺至少要求：目标字幕确实缺失、写入目标明确、候选 Health PASS、匹配置信度达标，并且用户明确开启自动化。

以下行为在早期不自动执行：

- 替换已有健康字幕。
- MultiSource STRM 写入。
- 低置信度双语判断驱动的替换。
- 大规模 Warning 修复。
- 复杂 ASS 重写、OCR、自动翻译或整体时间轴猜测。

## STRM 与写入边界

- 不读取 `.strm` 文件内容，不解析其中的远端 URL。
- STRM sidecar 以 Emby `Item.Path` 对应的本地文件目录为锚点。
- 远程 `MediaSource.Path` 只用于播放定位，不能当成本地写入路径。
- MultiSource STRM V1 只允许明确绑定后的读取、搜索和预览，不自动写入。
- 写入必须使用临时文件和简单备份，失败不能留下半截正式文件。
- Refresh 后必须确认 Emby 的 MediaStreams 能看到新字幕，并报告明确失败原因。
- 外置字幕文件名遵循 [Emby 字幕命名说明](https://emby.media/support/articles/Subtitles.html)：搜索层使用基础语言码，简繁地区信息写入文件名，例如简体 `zh-CN`、繁体 `zh-TW`。插件当前不追加 `.default`、`.forced` 或 `.sdh`，默认选择交由 Emby 用户字幕设置调整。

## 明确不属于 SubSteward 主线的内容

除非未来真实需求证明必要，不重新引入旧 SubBridge 的 Go 服务、HTTP 服务依赖、Docker 部署、数据库、完整 history、quarantine、重型 recovery、Restore Center、复杂安全 gate 或旧 Web UI。

这些内容保留在 `legacy/subbridge` 和 Git 历史中，只作为知识和回退来源。

## 成功标准

本项目成功，不以“旧 SubBridge 功能是否全部迁移”为标准，而以以下结果为标准：

1. 插件可以独立加载并使用 Emby 的公开能力。
2. 用户能看到每个视频的字幕存在性、健康状态、偏好匹配和建议动作。
3. 人工流程可以安全地搜索、校验、安装和刷新字幕。
4. 自动补缺只处理用户授权的单 Source 缺字幕场景，不写错媒体、不静默覆盖错误内容。
5. 复杂边界明确返回 `Unsupported`、`Manual` 或 `TODO`，不阻塞普通媒体流程。

## 当前状态

M0 与 M1 已完成：插件加载、单 Source STRM 的公开 Search/Fetch、候选预览、ASS/SRT 结构校验、版本化 sidecar 安装、Refresh、Emby 字幕流读取和实际客户端播放验收均已验证。当前 M1 通过嵌入式管理员页面和管理员 API 提供人工流程，页面分为状态、手动处理和设置三个页签，可查看条目摘要、Presence、Health、Preference、Action，并完成 Search → Fetch/Preview → 可选固定偏移对轴 → Install。单条目详情会对安全范围内的外置字幕执行有上限深检，内封字幕继续保持未知，不提取正文。对轴只按管理员明确输入的毫秒值整体提前或延后，派生新 Artifact 后重新校验，不自动猜测音画偏移。设置支持全局默认与媒体库级覆盖。M2 核心分析与保守 Action Advisor 已实现，并接入 Item 详情及 Fetch/Preview Artifact 响应；语言标签区分规范宏语言与简繁变体，配置和 Provider 返回的语言变体会统一归一化，社区别名只作输入归一化。Health FAIL 或缺少标题/Hash 绑定的候选项不能进入推荐，自动 Repair/Upgrade 仍未开放。

### M2 当前收口边界

以下项目已经作为已知限制记录，本阶段不作为 Action 和人工 Search → Fetch → Preview → Install 主链路的收口阻断项：

- 简体/繁体目前会贯通请求配置、MediaStream/Provider 语言码、标题和安全文件名证据；仍没有可靠的正文级简繁识别时，系统保持低置信度，不据此自动替换。
- 正文级语言检测当前对中文、英语和日语有明确规则；其他第二语言可以保留配置和 Presence 语言码，但展示与正文覆盖分析仍有限。
- ASS 的 Script Info、Styles、残缺 HTML 和更深层结构校验仍较浅，后续扩展 Health 时处理。
- Preference 当前支持目标语言、第二语言、双语开关和格式顺序；用户自定义权重以及普通、样式化、高特效偏好暂不纳入本阶段。
- `PreferenceAnalyzer` 支持对已 Fetch 的候选做纯计算排序，但服务入口尚未接入大规模候选的批量 Deep Ranking，后续在自动化范围明确后再评估。

这些限制不改变当前的 fail-closed 规则，也不授权自动 Repair、Upgrade 或 MultiSource STRM 写入。
