# ADR-001　V1 使用 Emby Remote Subtitle Bridge

- 状态　accepted
- 日期　2026-08-24
- 相关组件　Provider、Search、Preview、Installer

## 背景

项目需要在 Emby 与 STRM 媒体库中搜索 Thunder 和 ASSRT 中文字幕。MeiamSub 已经作为 Emby Subtitle Provider 部署，原规划同时保留了 Emby Bridge 和 Native Provider 两条可能路线。

Gate 0 与 Gate 0.1 已经在真实实例上验证独立 API Key Search、候选 Fetch 和外部字幕读取。Assrt 候选成功返回 ASS，Thunder 的三个候选中两个成功返回 SRT。

## 问题

项目需要决定 V1 是否继续通过 Emby 调用 Meiam，或者立即维护 Thunder 和 ASSRT 原生实现。

Emby Bridge 无法在单次请求中指定 Provider 或自定义关键词。Thunder 候选也可能因为上游地址失效而 Fetch 失败。

## 可选方案

### 方案 A

V1 使用 Emby Remote Subtitle Bridge。应用负责候选归类、预览、错误隔离和后续安全安装。

### 方案 B

V1 直接实现 Native Thunder 和 Native ASSRT。应用自行维护搜索协议、Token、压缩包和上游兼容性。

### 方案 C

修改或 Fork Meiam，增加自定义搜索和 STRM 专用接口，再由应用调用扩展接口。

## 最终选择

V1 采用方案 A。Native Provider 暂缓，保留为未来替代能力。

## 选择原因

- Search、Fetch 和 API Key 权限已经在真实环境跑通。
- 单个失效候选可以由候选级错误模型处理。
- V1 的人工管理流程不依赖自定义关键词和请求级 Provider 调度。
- 该选择避免同时维护 Go、Vue 和 C# Provider 代码。

## 已知代价

- Provider 标签只能筛选和排序结果。
- 搜索关键词由 Emby 与 Meiam 决定。
- Meiam 或 Emby 的行为变化会影响 Bridge。
- Thunder 对 STRM stub 的 CID 行为继续作为兼容事项保留。
- 上游 404 被包装为 HTTP 500，应用需要结合候选级上下文处理。

## 后续影响

`EmbyRemoteSubtitleProvider` 必须声明 `SupportsProviderSelection` 和 `SupportsCustomQuery` 为 false。UI 不能把筛选标签描述成只搜索某个来源。

Fetch 失败不能清空搜索结果。临时错误最多重试一次，上游 4xx 和无效内容直接标记候选失败。自动模式最多按顺序尝试前三个候选。

出现以下情况时重新评估 Native Provider。

- Emby Bridge 无法提供稳定 Search 或 Fetch
- Meiam 停止兼容当前 Emby 版本
- 产品明确需要自定义关键词或精确 Provider 调度
- Bridge 的错误包装妨碍可靠自动化且无法在适配层处理

## 验证依据

- [Gate 0 实测报告](../../records/acceptance/gate0-report.md)
- [当前架构](../../reference/architecture.md)
- [总体规划](../../planning/master-plan.md)
