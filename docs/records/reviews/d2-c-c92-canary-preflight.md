# D2-C C92 有界只读 Item 选择与 Canary 预检

- 日期：2026-08-25（香港时间）
- 范围：仅 C92；未执行 SH 管理操作、配置读取或服务变更
- 结论：**预检阻断，未执行 Search → Fetch → Preview Canary**

## 1. 执行边界

本轮只做实时只读探测和文档记录：

- 保留工作区已有未提交改动；没有重置、清理、提交或推送。
- 没有部署、重启、修改 Emby、修改媒体、修改字幕或修改元数据。
- 没有调用 D2 的 Search、Fetch、Preview POST 路由，也没有访问 Provider。
- 没有输出或写入 API Key、Bearer Token、Cookie、原始候选 ID、认证 URL、媒体路径、标题或字幕正文。

依据为 [当前架构](../../reference/architecture.md)、[D2 搜索预览契约](../../planning/d2-search-preview-contract.md)、[ADR-005](../../decisions/adr/005-conditional-d2-entry-without-live-multisource.md) 和 [本机操作文档](../../../LOCAL_OPERATIONS.md)。本机操作文档只用于连接和边界核对，未把其中的历史部署快照当作当前运行状态证明。

## 2. C92 实时只读证据

| 检查 | 结果 | 说明 |
|---|---|---|
| `/livez` | HTTP 200 | 进程可响应 |
| `/readyz` | HTTP 200 | 应用就绪探针可响应 |
| `/v1/health`（不带应用 Bearer） | HTTP 401 | 认证边界生效；本轮没有读取或输出应用凭据 |
| 内嵌页面 `/` | HTTP 200 | C92 应用页面可达 |
| 实时 `/assets/app.js` | HTTP 200，13,840 bytes | 未发现 D2 Search、Fetch、Preview 路由字符串，也未发现 `remote_search_enabled` 或 `write_enabled` 前端状态标记 |

实时静态资源仍是 D1 只读 UI 形态，不能证明 C92 正在运行经过审核的 D2 版本。`/v1/health` 受保护且当前没有在本地取得应用 Bearer，因此 `remote_search_enabled`、`d2.canary.enabled`、allowlist generation、专用 cache 权限和当前运行时 `write_enabled` 均未被冒充为已确认。

## 3. 只读 Item 选择

通过 C92 Emby 的现有服务端 API Key 只读查询了一个已知 Movie 样本的详细 Item；密钥只在本地请求内存中使用，未进入输出。结果如下：

| 字段 | 结果 |
|---|---|
| 脱敏 Item 引用 | `sha256(raw_item_id)[:12] = b6a1f28428d3` |
| `Type` | `Movie` |
| `MediaSources` 数量 | `1` |
| 唯一 source ID | 非空，未记录原文 |
| STRM | `true` |
| Emby 详细 Item 请求 | HTTP 200 |

该 Item 满足 D2 首轮的“Movie/Episode 且恰好一个有效 MediaSource”选择条件。Item ID、MediaSource ID、Item 标题和路径没有写入本报告；上面的引用只用于主线程独立复核时的脱敏关联，不是服务端 allowlist 值。

## 4. D2-C 门禁判定

| 门禁 | 判定 | 依据 |
|---|---|---|
| C92 正在运行经过审核的 D2 版本 | **不通过** | 实时页面资源没有 D2 三条路由标记；当前部署版本/构建身份未能通过受保护 health 接口复核 |
| `remote_search_enabled=true` | 未确认 | health 无授权时返回 401；没有读取应用凭据 |
| `d2.canary.enabled=true` | 未确认 | 同上，未读取运行时配置 |
| allowlist 存在且非空、且包含选中 Item | 未确认 | 未取得 C92 容器内 Secret 内容或原始 Item allowlist 值 |
| 专用 cache 位于媒体映射之外并满足权限 | 未确认 | 未执行远端容器/宿主文件检查；本轮不部署 overlay |
| 当前运行时 `write_enabled=false`、rootfs 和 `/media` 只读 | 未确认 | 历史 D1 文档不足以替代当前运行时核对 |

首个硬阻断是“审核过的 D2 版本”未通过实时证据；其余 D2 门禁也没有完整证据。因此不能进入真实 Search，更不能把单源 Item 选择结果扩展成 Canary 通过结论。

## 5. 停止结论与后续入口

本轮在预检阶段安全停止。没有产生 Candidate Token、Artifact、缓存文件或 Provider 请求，也没有任何媒体目录写入、Refresh、Save、Add、Replace、Delete 或 Upload 请求。

只有在后续获得独立授权并完成 C92-only 运行时复核后，才可继续：

1. 核对不可变、经过审核的 D2 构建身份和真实运行版本。
2. 通过受保护 health/容器只读检查确认 `remote_search_enabled=true`、`d2.canary.enabled=true`、allowlist 非空且命中选中 Item。
3. 确认专用 cache 的宿主权限、媒体映射不重叠、rootfs 只读、`/media` 为 `RW=false`，并确认 `write_enabled=false` 为运行时值。
4. 在固定时间窗内，按 D2 契约只对该单源 Item 执行一次有界 Search，选择一个服务端返回的候选执行 Fetch，再按有限分页执行 Preview；报告只记录稳定状态码、候选数量、字节数、cue 数和脱敏引用。
5. 窗口结束后再次核对搜索开关和 Canary 开关均关闭。

## Knowledge Review

任务或阶段：C92 D2-C 有界只读 Item 选择与 Canary 预检

验证范围：`AGENTS.md`、`docs/architecture.md`、`docs/d2-search-preview-contract.md`、`docs/phase2-readonly-canary.md`、`docs/adr/005-conditional-d2-entry-without-live-multisource.md`、`docs/lessons-learned.md`、`LOCAL_OPERATIONS.md`、C92 实时探针、C92 实时内嵌资源和 C92 Emby 单源 Item 详情。

Knowledge Findings

- 新增约束：无。现有 D2 契约已经规定审核版本、双开关、非空 allowlist、稳定专用 cache、写关闭和单源门禁。
- 隐蔽坑：历史 D1 部署快照和实时 `/readyz` 只能证明服务可达，不能证明当前 D2 版本或 D2 运行时配置。
- 被证明错误的假设：无；本轮没有把静态页面、旧部署记录或单源 Item 结果冒充 D2 Canary 通过。
- 建议沉淀项：无。该结论是当前 C92 的一次性预检，暂不更新长期维护经验。

证据

- 代码：未修改应用代码。
- 测试：未执行 D2 真实 API 测试；未产生 Provider 或媒体写操作。
- 实际运行、日志或可复现结果：C92 `/livez`、`/readyz`、根页面、实时 `app.js` 和 Emby Item 只读请求；结果见第 2、3 节。

去重检查

- 已搜索文档和关键词：`D2`、`Canary`、`remote_search_enabled`、`d2.canary.enabled`、`allowlist`、`cache_dir`、`write_enabled`、`MediaSource`、`Search`、`Fetch`、`Preview`、`SH`、`C92`。
- 是否更新已有结论：否；本报告只保存本次 C92 预检证据和阻断状态。

分流判断

- `docs/lessons-learned.md`：不需要。
- `docs/architecture.md`：不更新；当前运行状态不满足进入架构事实的证据门槛。
- `docs/adr/`：不需要新增或更新 ADR。
- `LOCAL_OPERATIONS.md`：不更新；没有新的长期拓扑、连接方法或恢复步骤。

未验证范围与残余风险

- C92 当前 D2 构建身份、应用 health flags、allowlist、专用 cache 权限和 `write_enabled` 运行时值未验证。
- 没有执行真实 D2 Search、Fetch、Preview、客户端预览、Provider 失败隔离或缓存 TTL 验收。
- 选中的单源 Movie 仅证明 Item 选择条件，不证明 allowlist 命中，也不证明真实多源支持。
