# SubSteward 架构

SubSteward 以一个 Emby Plugin 为主运行时。新分支不包含旧 Go 服务、Web UI、Docker 部署、history、quarantine 或复杂 recovery 系统。

```text
Emby Server
  └─ SubSteward.Plugin
       ├─ Plugin identity and configuration
       ├─ M1 manual API
       └─ Future: quality, preference and conservative automation
```

## M0 Architecture Spike

当前工程位于 `src/SubSteward.Plugin`，使用 `netstandard2.0` 与公开的 `MediaBrowser.Server.Core` 编译基线。当前已实现：

- `BasePlugin<PluginConfiguration>` 插件入口和稳定 ID。
- 服务器管理的 Plugin 数据目录入口。
- M0 已完成并已从运行时移除：它只用于验证插件加载、公开 API 与单 Source STRM 的一次性闭环，不作为产品任务长期安装。

M1 提供管理员接口：

- `GET /SubSteward/Items` 与 `GET /SubSteward/Items/{Id}`：返回不含路径的 Item、Source 和字幕流摘要。
- `GET /SubSteward/Subtitles/Search`：按 Item/source/language 搜索候选，并将 Hash 或标题匹配的候选排在前面。
- `POST /SubSteward/Subtitles/Fetch`：服务端保存短期候选绑定，Fetch 字节流并 Validate；没有标题或 Hash 匹配的候选拒绝继续安装。
- `GET /SubSteward/Subtitles/Preview`：返回短期 Artifact 的 Health、编码、格式、原因和最多 200 条 cue。
- `POST /SubSteward/Subtitles/Install`：重新读取 Item/source，临时文件写入新版本 sidecar，Refresh 并确认新外置字幕流。

Candidate raw ID、候选和 Artifact 内容仅保存在进程内短期存储中，不进入响应或日志。

本机已完成 Release 编译与基础测试。C92 Emby 4.9.5.0 已成功加载插件和发现手动任务；“千与千寻”是单 Source STRM，M0 以其 `Item.Path` sidecar 锚点完成 Search、Fetch、安装、Refresh 与 Emby 新字幕流识别。清理本次任务生成的错误 sidecar 后，M1 管理员 API 完成 Search（20 候选）→ Fetch/Preview（ASS、Health PASS、200 cue 预览）→ Install → Refresh；外置字幕流从 2 增至 3，新流官方接口 HTTP 200。

M0 的一次公开 Download 调用最终被 Emby 识别为两条新增外置流，其中一条英语正文被错误标记为 `zh`；该三份本次任务生成的 sidecar 已移至插件数据目录的可恢复目录，原有用户文件未删除。进一步对账发现 Search 第一条 Ghibli 合集候选的 Fetch 字节包含鲁邦，第二条标称中英双语的 Fetch 字节为英语，第三条标题匹配候选才是当前中文 `zho.ass` 的来源。M1 现要求中文候选正文至少包含中文字符、标题或 Hash 匹配，并将候选、Artifact、安装目标和最终 MediaStream 绑定到同一 Item/source；无匹配候选不再进入 Fetch/Install。MultiSource STRM 仍不自动写入，且本次未做客户端实际播放验收。

## 写入最小正确性

写入只在 M0 后设计，并至少包含临时文件、简单备份、有限重试、明确失败与 Refresh/MediaStreams 复核。它不恢复旧项目的重型事务模型。
