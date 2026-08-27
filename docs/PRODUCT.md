# SubSteward 产品说明

> Subtitle Automation for Emby

SubSteward 是 Emby 的字幕自动补全、质量检查与优化插件。它以 Presence、Health、Preference、Action 为统一业务模型，而不是延续 SubBridge 的 Go 服务、HTTP API 或 Docker 部署。

## 产品边界

- Presence 只判断目标语言字幕是否存在；内封目标语言字幕默认通过，不提取或深检。
- Health 只报告影响观看的明确问题，状态为 `PASS`、`WARNING`、`FAIL`。
- Preference 在 Health 合格后才评估双语、第二语言、格式、特效和 Provider。
- Action 统一为 `KEEP`、`REPAIR`、`SEARCH`、`UPGRADE`、`MANUAL`、`IGNORE`。

## 里程碑

| 阶段 | 范围 |
|---|---|
| M0 | Plugin 架构、公开 API 与一次明确授权的单 Source 普通媒体闭环。 |
| M1 | 人工 Search → Fetch → Preview → Validate → Install → Refresh（管理员 API 已在 C92 单 Source STRM 通过，候选 ID 只保留服务端内存）。 |
| M2 | 字幕质量、语言、双语、特效和偏好排序。 |
| M3 | 仅对授权媒体库的单 Source 缺字幕做保守自动补缺。 |
| M4 | 在真实使用数据足够后再评估自动 Repair 与 Upgrade。 |

MultiSource STRM V1 不自动写入。复杂边界优先返回 Unsupported、Manual 或 TODO，不以牺牲普通媒体流程为代价。

M0 与 M1 已通过：C92 插件加载、单 Source STRM 的公开 Search/Fetch、候选预览、ASS/SRT 结构校验、版本化 sidecar 安装、Refresh 与官方字幕流读取均已完成。M1 当前以管理员 API 提供人工操作，尚未做真实客户端播放验收。MultiSource STRM 仍不自动写入。
