# SubSteward 架构

SubSteward 以一个 Emby Plugin 为主运行时。新分支不包含旧 Go 服务、Web UI、Docker 部署、history、quarantine 或复杂 recovery 系统。

```text
Emby Server
  └─ SubSteward.Plugin
       ├─ Plugin identity and configuration
       ├─ M1 manual API
       └─ M2 quality, preference and conservative action advice
```

## M0 Architecture Spike

当前工程位于 `src/SubSteward.Plugin`，使用 `netstandard2.0` 与公开的 `MediaBrowser.Server.Core` 编译基线。当前已实现：

- `BasePlugin<PluginConfiguration>` 插件入口和稳定 ID。
- 服务器管理的 Plugin 数据目录入口。
- M0 已完成并已从运行时移除：它只用于验证插件加载、公开 API 与单 Source STRM 的一次性闭环，不作为产品任务长期安装。

M1 提供管理员接口：

- `GET /SubSteward/Items` 与 `GET /SubSteward/Items/{Id}`：返回不含路径的 Item、Source 和字幕流摘要；每个 Source 额外返回按配置计算的 Presence，Item 顶层返回保守 Action 建议。已知语言码优先，未知字幕标题仅对中文使用保守文本回退。
- `GET /SubSteward/Subtitles/Search`：按 Item/source/language 搜索候选，并将 Hash 或标题匹配的候选排在前面。
- `POST /SubSteward/Subtitles/Fetch`：服务端保存短期候选绑定，Fetch 字节流并 Validate；没有标题或 Hash 匹配的候选拒绝继续安装。
- `GET /SubSteward/Subtitles/Preview`：返回短期 Artifact 的 Health、编码、格式、原因、Quality、Preference、Action 和最多 200 条 cue。
- `POST /SubSteward/Subtitles/Install`：重新读取 Item/source，临时文件写入新版本 sidecar，Refresh 并确认新外置字幕流。

Candidate raw ID、候选和 Artifact 内容仅保存在进程内短期存储中，不进入响应或日志。

M2 已加入纯计算的 `QualityAnalyzer`。它只消费 M1 Validation 结果，在 Preview 中输出目标语言、第二语言、双语和 ASS 特效强度摘要；不改变 Search/Fetch/Install 行为，也不自动重写字幕正文。

当前中文字符用作 zho 的保守正文证据；eng 需要至少两个连续拉丁字母，单个字母或孤立代号不作为第二语言证据。

M2 的 `PreferenceAnalyzer` 也只消费同一 Validation 结果。当前 Preview 输出 RECOMMENDED / ACCEPTABLE / NOT_RECOMMENDED 的 suitability 和理由。排序要求候选有标题或 Hash 绑定、Health 不为 FAIL、正文可见目标语言；Hash 匹配权重高于仅标题匹配。该结果用于人工选择展示，不是最终 Action 判定；默认不对双语和特效强制替换。

M2 的 `ActionAdvisor` 是独立纯计算层。输入包括 Source 数量、目标 Presence、当前或候选 Health、Preference suitability、标题/Hash 绑定和双语置信度。多 Source 或状态不明返回 `MANUAL`；目标缺失且没有可用候选返回 `SEARCH`；目标存在且已有 Health `PASS` 返回 `KEEP`；候选 `PASS` 且绑定和 Preference 合格时仍返回 `MANUAL`，等待人工确认安装。候选 `WARNING`、低置信度双语、已有目标但 Health 未知也返回 `MANUAL`；Health `FAIL`、无标题/Hash 绑定或 `NOT_RECOMMENDED` 候选继续 `SEARCH`。`REPAIR` 与 `UPGRADE` 只保留为产品动作枚举，M2 不自动执行。

M2 的 `PresenceAnalyzer` 只消费 MediaStream 的语言、展示标题和外挂字幕安全文件名，不读取 `.strm` 内容，也不提取内封字幕正文。它能识别 zho/zh/chi 与 eng/en；外挂文件名中的显式简繁标签会用于变体证据。完整路径不进入 API 输出。

2026-08-27 在 C92 已用既有授权样本完成 M2 API 验证：Item Presence 正常返回目标语言流；Search 返回候选并显示标题/Hash 绑定状态。一个 Provider 候选 Fetch 失败后按候选隔离处理；另一条标题匹配的 zho SRT Fetch 成功，Preview 返回 PASS、1124 条 cue、目标语言覆盖和 Preference `ACCEPTABLE`，并带有保守 Action 建议。随后实际客户端播放验收已完成；本文不记录具体设备或客户端名称。

2026-08-27 本轮 Action 版本已部署到 C92。Release DLL SHA-256 为 `F118F30AB8AA36904ADC77B64F475188266FCABFD12F133EC59918193AE952C9`，覆盖前旧 DLL 已按同 Hash 备份；仅重启 `emby-server`，重启后状态为 running、RestartCount 为 0。日志确认重新加载 `SubSteward.Plugin`。管理员 API 验证中，Items 列表和 Item 详情均返回 HTTP 200，Item 顶层 `Action` 存在且在当前 Presence 已有、Health 未测量的样本上保守返回 `MANUAL`；Search 返回 HTTP 200。绑定候选中失败内容继续被校验器隔离，另一条候选 Fetch 和 Preview 均返回 HTTP 200、Health `PASS`、Preference `ACCEPTABLE`、Action `MANUAL`。本轮未执行 Install，未修改媒体文件；列表前 100 条均为单 Source 且已有目标 Presence，因此缺失目标的线上 `SEARCH` 分支仍由本地 Action 测试覆盖。

插件配置当前提供 `TargetLanguage`、`SecondaryLanguage`、`PreferBilingual` 和 `FormatOrder`。默认值分别是 `zho`、`eng`、false 和 `ass,ssa,srt`；分隔符接受逗号或分号。`chs/cht、zh-Hans/zh-Hant、简/繁` 等输入别名会归一化为规范宏语言 `zho` 和显式变体标签；Emby 语言码或未知标题中的简繁标记用于 Presence 变体观察。

受控实测确认，尾部标准语言码决定 Emby 的 language/display 字段，紧邻它的自定义段会成为 MediaStream 的 title。Fetch/Install 新写入采用：

```text
<媒体文件主名>.<实际类型标签>.<Emby 语言标签>.<原格式>
```

例如 `<movie>.中文简体.zh-CN.ass` 或 `<movie>.中日双语.zh-CN.srt`。无显式变体时不插入类型标签，保持既有行为。写入仍采用版本化新文件，不覆盖已有文件，也不启用自动替换。

本机已完成 Release 编译与基础测试。C92 Emby 4.9.5.0 已成功加载插件和发现手动任务；“千与千寻”是单 Source STRM，M0 以其 `Item.Path` sidecar 锚点完成 Search、Fetch、安装、Refresh 与 Emby 新字幕流识别。清理本次任务生成的错误 sidecar 后，M1 管理员 API 完成 Search（20 候选）→ Fetch/Preview（ASS、Health PASS、200 cue 预览）→ Install → Refresh；外置字幕流从 2 增至 3，新流官方接口 HTTP 200。

M0 的一次公开 Download 调用最终被 Emby 识别为两条新增外置流，其中一条英语正文被错误标记为 `zh`；该三份本次任务生成的 sidecar 已移至插件数据目录的可恢复目录，原有用户文件未删除。进一步对账发现 Search 第一条 Ghibli 合集候选的 Fetch 字节包含鲁邦，第二条标称中英双语的 Fetch 字节为英语，第三条标题匹配候选才是当前中文 `zho.ass` 的来源。M1 现要求中文候选正文至少包含中文字符、标题或 Hash 匹配，并将候选、Artifact、安装目标和最终 MediaStream 绑定到同一 Item/source；无匹配候选不再进入 Fetch/Install。MultiSource STRM 仍不自动写入；实际客户端播放验收已完成，本文不记录具体设备或客户端名称。

## 写入最小正确性

写入只在 M0 后设计，并至少包含临时文件、简单备份、有限重试、明确失败与 Refresh/MediaStreams 复核。它不恢复旧项目的重型事务模型。
