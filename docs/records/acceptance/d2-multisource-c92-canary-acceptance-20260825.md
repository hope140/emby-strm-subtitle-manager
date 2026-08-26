# D2 多源真实 API Canary 验收报告

- 日期：2026-08-25（香港时间）
- 范围：仅 C92；未连接、读取或修改 SH/上海服务器、FRP 或 OpenResty
- 代码引用：`784ad32d68edc79712cf01f01f2b39d9d56213db`
- 结论：**真实多源 Item 的 API/source 对应和 D2 fail-closed 通过；没有宣称或开放多源搜索支持，Canary 窗口已关闭**

## 1. 执行边界

本次使用用户提供的两个真实 Movie 版本 Item 做有界、脱敏、只读验收。报告不记录标题、Item ID、MediaSource ID、媒体路径、候选 ID、Token 或字幕正文。

- 通过 C92 本机 Emby `GET /Items` 的限定搜索找到 2 个候选，并对每个 Item 用包含 `AlternateMediaSources` 的详情字段重读；两个 Item 均得到 2 个完整 source。
- 只对应用本机 `127.0.0.1:18080` 发起媒体详情、字幕清单和 D2 Search/Fetch/Preview 请求；没有调用 Save、Refresh、Add、Replace、Delete、Upload、Playback 或任何媒体写接口。
- D2 只临时启用服务端 Item allowlist；Canary 完成后立即恢复版本化 closed Compose/config。

## 2. 版本化材料与预检

本次没有覆盖现有 closed 文件，新增了带 `784ad32-multisource` 后缀的临时材料：

- `shared/compose.784ad32-multisource.yaml`
- `shared/compose.784ad32-multisource-d2.yaml`
- `shared/config.784ad32-multisource.yaml`
- `secrets/d2_canary_items.784ad32-multisource`
- `d2-preview-cache.784ad32-multisource`

预检结果：

- allowlist 含 2 个精确 Item ID，宿主权限为 `10001:10001`、`0400`
- 专用 cache 位于媒体映射之外，权限为 `10001:10001`、`0700`
- `docker compose config --quiet` 通过
- 以应用 UID 10001 的临时容器检查 allowlist 可读、配置可读、cache 可写，均通过
- Compose 对 file-source 的 `uid`、`gid`、`mode` 给出 warning 并忽略；验收以宿主实际权限和容器内测试为准

## 3. Canary 运行结果

启用配置为 `remote_search_enabled=true`、`d2.canary.enabled=true`，`write_enabled=false` 保持不变。容器仍使用 `emby-strm-subtitle-manager:d2.5-784ad32`，没有重新构建镜像。

### 3.1 运行边界

- `/livez=200`、`/readyz=200`
- 未认证 `/v1/health=401`
- Bearer 健康响应显示 `remote_search_enabled=true`、`write_enabled=false`
- app 为 `running/healthy`，UID `10001:10001`，只读 rootfs，host network，restart count `0`

### 3.2 真实 source 对应

- 两个真实 Item 在未提交 source 时均返回 `409 media_source_required`
- 每个 Item 均返回 2 个 source 选项
- 4 次显式 source 详情请求均返回 `200`，响应 `media_source_id` 与请求逐项一致
- 4 次显式 source 字幕清单请求均返回 `200`，嵌套媒体上下文中的 `media_source_id` 与请求逐项一致

这证明当前真实 Emby 详情、应用 MediaContext、详情响应和字幕清单响应在 source 选择上保持对应；它不等于浏览器点击流程或多源正向搜索能力已开放。

### 3.3 D2 多源安全拒绝

对两个真实多源 Item 分别调用 Search、Fetch、Preview，并在 Search 中提交了一个真实 source ID：

- 共 6 次请求全部返回 `409 d2_multisource_unsupported`
- 显式提交 source ID 也没有绕过多源门禁
- 专用 Canary cache 文件数为 `0`
- 没有进入候选成功、Fetch Artifact 或 Preview 成功路径

## 4. Canary 关闭后的状态

Canary 结束后立即使用原有版本化 closed 材料重建 app：

- `shared/compose.784ad32-closed.yaml` + `shared/compose.784ad32-d2.yaml`
- `shared/config.784ad32-closed.yaml`

关闭后实时核对：

- app `running/healthy`，`/livez=200`、`/readyz=200`
- 未认证 `/v1/health=401`
- `remote_search_enabled=false`、`write_enabled=false`
- Search 返回 `403 remote_search_disabled`
- UID、只读 rootfs、host network、restart count `0` 保持不变
- `frpc-sh` 容器 ID、启动时间和 restart count 与 Canary 前一致
- SH、FRP、OpenResty、公网 18080 未修改

临时 allowlist、cache、enabled config 和 enabled Compose 保留在 C92 的版本化路径中，作为可追溯材料；它们没有进入 Git，也没有被 closed 运行态引用。

## 5. 支持范围与残余门禁

本次新增的真实证据是：

1. 两个真实多源 Movie Item 的完整 source 集合能被应用读取。
2. API 详情和字幕清单的显式 source 选择可以逐项对应。
3. D2 Search、Fetch、Preview 对真实多源 Item 统一安全返回 409，不能猜测或绕过 source。

仍不能宣称：

- 多源 Search、Fetch、Preview 的正向支持；当前实现仍是单 source fail closed
- 真实浏览器 UI 的点击、刷新和 source 选择流程已在 C92 通过；本次是应用 API 对应验收，未把浏览器自动化或公网路径冒充真实证据
- D3 Add、Replace、Delete、Upload、Refresh 或任何媒体写入能力

因此 `remote_search_enabled=false`、`d2.canary.enabled=false` 和 `write_enabled=false` 继续作为默认运行边界。若要实现真正的多源搜索支持，需要另立设计/实现任务和新的正向 Canary，不能由本报告直接放开。

## Knowledge Review

任务或阶段：C92 真实多源 API/source 对应和 D2 fail-closed Canary。

### Knowledge Findings

- 新增运行事实：真实多源 Item 的应用详情和字幕清单在显式 source 选择下可以逐项保持 source 对应。
- 新增验收边界：真实多源 Search、Fetch、Preview 在当前单源契约下统一返回 `409 d2_multisource_unsupported`；这证明安全拒绝，不证明多源正向支持。
- Canary 开启后必须立即恢复 closed 配置；临时 allowlist/cache/config/Compose 不进入 Git 或普通运行态。

### 分流判断

- `docs/architecture.md`、`docs/phase2-readonly-canary.md` 和 `docs/README.md`：更新真实 API 对应与 409 安全拒绝状态，但保留浏览器 UI 和多源正向支持未完成边界。
- `docs/adr/005-conditional-d2-entry-without-live-multisource.md`：不改变 fail-closed 决策，只补充本报告证据链接。
- `docs/lessons-learned.md`：已有 `AlternateMediaSources` 经验足够，本次不重复添加相同规则。
- `LOCAL_OPERATIONS.md`：记录本机长期有用的 C92 临时材料、当前 closed 状态和回滚顺序，不记录真实 ID 或凭据。

