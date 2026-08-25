# D2 多版本 MediaSources 实测记录

- 日期：2026-08-25（香港时间）
- 范围：C92 Emby，只读 API 核对
- 结论：已找到可用于多源门禁的真实 Movie 版本组；此前“只有一个 MediaSource”的判断是因为请求遗漏了 `AlternateMediaSources` 字段。客户端字段请求、DTO 合并和回归测试现已完成；当前仍未重新开启 D2 Search、Fetch、Preview，也未宣称多源能力已验收。

## 1. 执行边界

本次只调用 Emby `GET /Items`：

- 使用服务端 API Key 的 `X-Emby-Token` 请求头，凭据未进入报告、日志或 URL。
- 没有调用 RemoteSearch、Fetch、Save、Refresh、Playback 或任何媒体写接口。
- 没有修改神医设置、Emby 元数据、媒体文件、C92/SH/FRP/OpenResty 或应用容器。
- 报告不记录真实标题、Item ID、MediaSource ID、媒体路径、候选 ID 或源名称；样本称为“已知 Movie 版本组”。

## 2. 实时 API 证据

对同一已知 Movie 版本组分别进行列表查询和按 Item 详情重读：

| 请求字段 | 返回结构 | 结果 |
|---|---|---|
| `Path,MediaSources,MediaStreams,PresentationUniqueKey` | 两个关联 Item；每个只返回 1 个 `MediaSource` | 不能据此判断没有多源 |
| `Path,ProviderIds,MediaStreams,MediaSources,AlternateMediaSources` | 对每个选中 Item 的详情重读返回 2 个 `MediaSources` | 满足 D2 多源样本形态 |

两个 Item 的只读核对结果：

- 类型均为 `Movie`。
- `Item.Path` 均为 STRM，所在目录相同。
- 父级和 `PresentationUniqueKey` 相同，属于同一版本组。
- 使用 `AlternateMediaSources` 后，每个详情 Item 的完整 source 列表均包含两个版本。

这说明 Emby 4.9.x 的版本组在列表查询中可能显示为多个关联 Item；详情请求必须显式带上 `AlternateMediaSources`，否则响应看起来只有默认版本。Emby 官方命名规则要求同一电影的多版本放在同一电影目录，并使用“目录名 - 版本后缀”的命名方式；Emby 4.9.x 的版本源查询还需要在 `Fields` 中加入 `AlternateMediaSources`。[Movie Naming](https://emby.media/support/articles/Movie-Naming.html)、[Emby 4.9.x AlternateMediaSources 说明](https://emby.media/community/topic/148258-getshowsbyidepisodes-returns-incomplete-mediasources-for-non-admin-users-since-49x/)

## 3. a70bf89 应用对应性核对

在 a70bf89 镜像完成 C92 app-only 重建后，对同一组两个真实 Movie Item 做了第二轮有界只读核对：

- 应用 API 的两个详情请求均识别为 `source_count=2`，未选择默认 source；未带 `media_source_id` 时均返回 `409 media_source_required`。
- 直接 Emby 详情请求固定使用 `Path,ProviderIds,MediaStreams,MediaSources,AlternateMediaSources`。应用响应中的 source ID 集合与 Emby 完整 source ID 集合逐项相等，脱敏探针结果为 `checked=2 corresponding_source_sets=2`。
- 只调用了应用只读媒体详情和 Emby `GET /Items`；没有调用 Search、Fetch、Preview、Save、Refresh 或任何媒体写接口。

这证明 a70bf89 的 `AlternateMediaSources` 修正已经进入实际 C92 应用，并且真实多源 Item 会被服务端安全拒绝而不是猜测第一 source；它不等于多源 Search、Fetch、Preview 已经开放或通过真实 Canary。

## 4. 对 D2 的影响

1. `internal/embyclient.Client.GetItem` 的详情请求已固定包含 `AlternateMediaSources`。
2. DTO 边界已保留完整 `MediaSources`；如果某个 Emby 版本把备用源单独返回为 `AlternateMediaSources`，客户端会合并，并只抑制两个字段之间重复的非空 source ID；同一字段内部的重复仍保留给源校验拒绝。
3. D2 的多源前置检查继续以服务端详情中的完整 source 列表为准；数量大于一时保持 `409 d2_multisource_unsupported`，不能选择第一项或默认项。
4. 当前样本只证明真实 API 能提供完整的两个 source，不等于真实 Search → Fetch → Preview、UI 显式选择和 source 对应门禁已经通过。
5. `remote_search_enabled=false`、`d2.canary.enabled=false` 和 `write_enabled=false` 继续保持；完成真实 API/UI/source 对应验收和独立授权前不部署或开启真实多源搜索。

## Knowledge Review

任务或阶段：C92 真实多版本 MediaSources 样本、a70bf89 应用对应性核对与 D2 客户端字段修正入口

验证范围：C92 Emby 只读 `GET /Items` 两种 `Fields` 组合、当前 `internal/embyclient` 详情请求、D2 多源契约、Emby 官方 Movie Naming 和 4.9.x `AlternateMediaSources` 说明。

Knowledge Findings

- 新增约束：D2 的 Emby 详情读取必须显式请求 `AlternateMediaSources`，否则版本组可能只返回默认 `MediaSource`。
- 隐蔽坑：列表查询返回多个关联 Item 或单 source 不能证明版本组没有完整多源；必须用按 Item 详情读取并带完整字段。
- 被证明错误的假设：此前“当前真实样本仍缺失”以及“同目录版本组只能是两个各一 source”不成立；真实样本已获得，问题在客户端请求字段遗漏。
- 建议沉淀项：更新 `embyclient` 请求字段、DTO 合并去重测试和 D2 文档门禁；多源 Search/Fetch/Preview 仍需独立 Canary。

证据

- 代码：`internal/embyclient.Client.GetItem` 已固定请求 `Path,ProviderIds,MediaStreams,MediaSources,AlternateMediaSources`；`itemDTO` 支持备用 source 合并。
- 测试：客户端查询字段、双 source DTO、跨字段重复抑制和同字段重复保留测试已补齐；`internal/httpapi` 集成请求断言同步更新，完整 Go 测试、vet、构建和仓库验证已通过。
- 实际运行：C92 Emby 两种只读 `GET /Items` 查询，以及 a70bf89 应用详情/409/source-set 对应探针；结果见第 2、3 节。

去重检查

- 已搜索关键词：`AlternateMediaSources`、`MediaSources`、`PresentationUniqueKey`、`多版本`、`多源`、`D2`。
- 是否更新已有结论：是；把“真实多源样本缺失”收窄为“样本已找到，客户端字段修正已完成，完整 D2 Canary 待验收”。

分流判断

- `docs/lessons-learned.md`：更新 Emby 版本源字段规则。
- `docs/architecture.md`：更新 D2 详情读取事实和多源门禁边界。
- `docs/adr/`：更新 ADR-005 的后续状态，不撤销其 fail-closed 决策。
- `LOCAL_OPERATIONS.md`：不更新；没有新增拓扑、连接方式、路径或恢复步骤。

未验证范围与残余风险

- 客户端字段请求和 DTO 修正已合并到公开 `main` 的 `a70bf89`，并以 `d2.5-a70bf89` 镜像完成 C92 app-only 重建；两个真实 Item 的 source 集合对应核对已通过，尚未进行真实 Search、Fetch、Preview 多源 Canary。
- 尚未在 D2 真实 Canary 中执行 Search、Fetch、Preview，也未验证 UI 的显式 source 选择；本轮只验证了 UI 公共登录表单存在。
- 版本组的两个详情 Item 都能返回两个 source，但应用需要按绑定的 Item/source 做一致性检查，不能只依赖列表结果。
