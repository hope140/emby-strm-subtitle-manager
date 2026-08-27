# SubSteward

> Subtitle Automation for Emby

SubSteward 是 Emby 的字幕自动补全、质量检查与优化插件。它围绕 Presence、Health、Preference 和 Action 管理字幕，不以旧 SubBridge Go 服务为运行依赖。

## 许可证

本项目采用 [MIT License](LICENSE)，允许商用、修改和分发，且须保留许可证声明。

## 当前状态

M1 人工字幕管理闭环已在 C92 的“千与千寻”单 Source STRM 上通过：Search、Fetch、Preview、Validate、Install、Refresh 与官方字幕流读取均成功。下一步是 M2 质量与偏好分析；MultiSource STRM 仍拒绝自动写入。

## 文档

- [文档索引](docs/index.md)
- [产品说明](docs/PRODUCT.md)
- [SubBridge 经验](docs/SUBBRIDGE_LESSONS.md)
- [架构说明](docs/ARCHITECTURE.md)

## 核心边界

- 不读取 `.strm` 内容；外挂字幕以 Emby `Item.Path` 为 sidecar 锚点。
- MultiSource STRM V1 不自动写入。
- 内封目标语言字幕只用于 Presence，默认不提取或深检。
- 首期自动化只处理用户授权范围内的单 Source 缺字幕。
- 未经明确授权，不部署、重启 Emby、调用 Provider 或写入媒体。
