# SubSteward

> Subtitle Automation for Emby

SubSteward 是 Emby 的字幕自动补全、质量检查与优化插件。它围绕 Presence、Health、Preference 和 Action 管理字幕，不以旧 SubBridge Go 服务为运行依赖。

## 许可证

本项目采用 [MIT License](LICENSE)，允许商用、修改和分发，且须保留许可证声明。

## 当前状态

项目当前暂时搁置。M0/M1 已完成，M2 核心分析与保守 Action 建议已接入，M3 后端已完成定时任务、白名单、dry-run、单 Source 安全门禁、STRM 标题/年份/集数匹配、有限 Fetch 和候选间时间轴共识对照。候选互相对照已在 C92 的“外语电影”库 dry-run 中识别出时间漂移并转人工，历史上“妖猫传”已完成单样本安装、Refresh 和 MediaStream 对账，客户端播放仍待确认。暂停期间不继续开发、部署或运行自动化；服务器上的插件 DLL 和部署备份已清理，媒体文件未清理。

详细状态见 [项目状态](docs/PROJECT_STATUS.md)。

## 文档

- [文档索引](docs/index.md)
- [产品说明](docs/PRODUCT.md)
- [SubBridge 经验](docs/SUBBRIDGE_LESSONS.md)
- [架构说明](docs/ARCHITECTURE.md)
- [M3 自动补缺方案与进度](docs/M3_AUTOMATION.md)
- [项目状态](docs/PROJECT_STATUS.md)

## 核心边界

- 不读取 `.strm` 内容；外挂字幕以 Emby `Item.Path` 为 sidecar 锚点。
- MultiSource STRM V1 不自动写入。
- 内封目标语言字幕只用于 Presence，默认不提取或深检。
- M3 自动化只处理明确白名单内、单 Source 且缺少目标语言字幕的媒体；默认关闭并先 dry-run。
- 未经明确授权，不部署、重启 Emby、调用 Provider 或写入媒体。
