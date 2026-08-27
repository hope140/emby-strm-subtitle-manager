# 项目协作规则

## 适用范围

本文件适用于 `substeward-v1` 分支上的 SubSteward 代码、文档和本地验证。SubSteward 是 Emby 字幕自动补全、质量检查与优化插件，不是 SubBridge Go 服务的 V2。

## 开始任务前的读取顺序

1. 阅读本文件。
2. 阅读 [产品说明](docs/PRODUCT.md)。
3. 按任务关键词搜索 [SubBridge 经验](docs/SUBBRIDGE_LESSONS.md)。
4. 涉及 Plugin 架构时阅读 [架构说明](docs/ARCHITECTURE.md)。
5. 涉及本机连接、路径或恢复时阅读本地 `LOCAL_OPERATIONS.md`，不得在输出中回显敏感内容。

## 文档职责

- `AGENTS.md` 保存稳定的协作规则和当前阶段门禁。
- `docs/PRODUCT.md` 保存产品边界与里程碑。
- `docs/SUBBRIDGE_LESSONS.md` 保存筛选后的可复用事实与未验证边界。
- `docs/ARCHITECTURE.md` 保存当前 Plugin Spike 的实现与实测状态。
- `LOCAL_OPERATIONS.md` 保存本机拓扑、连接与恢复信息，保持未跟踪且不复制到产品文档。

不要把 SubBridge 的部署、Canary、Go 服务计划、评审或恢复文档整套复制到本分支；完整旧历史保留在稳定基线 `3acdf27047338f81438fa611aed314533e170371` 与 `main`。

## 当前已接受的项目决策

- SubSteward 采用 Emby Plugin 主线；M0 未通过前不把旧 Go 服务当作隐式运行依赖。
- 业务模型固定为 Presence → Health → Preference → Action。
- 内封目标语言字幕只参与 Presence，默认不提取或深检。
- 不读取 `.strm` 内容；STRM sidecar 以 Emby `Item.Path` 为锚点。
- MultiSource STRM V1 不自动写入；复杂边界优先返回 Unsupported、Manual 或 TODO。
- Provider 候选失败按候选隔离，自动 Fetch 有上限；不直接依赖 Provider 私有类型。
- 基本正确性包括临时文件、简单备份、有限重试、明确失败与 Refresh/MediaStreams 可见性确认，不引入旧服务的重型恢复平台。

长期决策变更写入 `docs/PRODUCT.md` 或 `docs/ARCHITECTURE.md`，不能只改代码或聊天结论。

## 变更原则

- 先检查现有文件、测试和未提交改动；只改当前里程碑需要的范围。
- 不重置、清理或覆盖用户已有改动。
- API Key、Token、Cookie、候选原始 ID、私有路径和带认证参数的 URL 不得进入代码、测试夹具、日志、提交或文档。
- 不把旧 Go API、Docker、数据库、history、quarantine、认证体系或 UI 作为新插件的默认依赖。
- 不创建虚构的“真实字幕样本”；公开 fixture 必须脱敏且可公开保存。

## M0 门禁

M0 只验证 Plugin 架构：插件加载、数据目录、Item/MediaSource/MediaStreams、后台任务、公开 Plugin API 的 Search/Fetch，以及明确授权单 Source STRM 或本地媒体的写入、Refresh 与新字幕流可见性。

M0 前不扫描全库、不批量写媒体、不做自动化，也不迁移旧代码。部署、重启、连接 Emby、Provider 请求与媒体写入必须取得用户对目标实例和样本的明确授权。

## 验证要求

- 文档修改检查 Markdown 链接、UTF-8、代码围栏、尾随空白和 `git diff --check`。
- Plugin 代码至少运行受影响工程的 `dotnet build`；添加测试后运行受影响测试。
- 本地编译、Fake 或公开 API 文档不能替代目标 Emby 的插件加载、Provider、写入、Refresh 或客户端播放验收。
- 首次启用写入或发布时，核对实际文件、Emby MediaStreams、字幕流与对应客户端读取；没有实测就明确标为未验证。

## 本地操作文档

`LOCAL_OPERATIONS.md` 只保存长期有用的本机信息，继续保持未跟踪。不得包含明文凭据；需要凭据时只记录其安全位置、权限和轮换方式。
