# D2-C C92 单源真实 Canary 验收报告

- 日期：2026-08-25（香港时间）
- 范围：仅 C92；未连接、读取或修改 SH/上海服务器、FRP 或 OpenResty
- 代码引用：`eac7b663cfc17c48ccac88aaa10d01e2a22994df`
- 结论：**单源 Search→Fetch→Preview Canary 通过；Canary 窗口已关闭；D2 仍未对外启用**

## 1. 执行边界

本次操作使用已授权的 C92 可回滚部署和单源 Canary 范围：

- 只允许一个服务端 allowlist 中的单源 Movie/Episode STRM Item；Item ID、标题、媒体路径、候选原始 ID、Candidate Token、Artifact Token 和字幕正文均未输出、写入 Git 或进入本报告。
- 只调用应用的 `Search`、`Fetch` 和 `Preview` 三个 D2 POST 路由；没有调用 Save、Refresh、Add、Replace、Delete、Upload 或其他媒体写入接口。
- `write_enabled` 全程为 `false`；真实 Fetch 只生成短期私有 PreviewArtifact，不写媒体目录、不改变 Emby 元数据。
- Search 首轮只发送 `language=zh-CN`、`forced=false`，候选尝试最多两个；第一个候选失败后没有重复尝试，第二个候选成功。
- Canary 完成后立即以版本化 closed Compose/config 重建同一个 app 服务，关闭 `remote_search_enabled` 和 `d2.canary.enabled`；没有重建 Emby、frpc-sh、frps、OpenResty 或其他服务。

## 2. 不可变发布引用和回滚材料

| 项目 | 实时结果 |
|---|---|
| Git commit | `eac7b663cfc17c48ccac88aaa10d01e2a22994df` |
| Docker image | `emby-strm-subtitle-manager:d2-canary-eac7b663cfc1` |
| Image ID | `sha256:f1ac6fd57b4dcbd539a969f279586976d26d094fb2a901b42cfcbb9ffc9c10d8` |
| OCI version | `0.2.0-d2-canary.1` |
| OCI revision | 与 Git commit 完全一致 |
| OCI source | `https://github.com/hope140/emby-strm-subtitle-manager` |
| 构建时间标签 | `2026-08-25T01:17:29Z` |
| 应用用户 | `10001:10001` |
| rootfs / 网络 | `ReadonlyRootfs=true` / `host` |
| `/media` | bind 且只读 |

部署前已保留当前 D1 Compose 和 config 的版本化 `.pre-<commit>.bak` 回滚文件；D2 base Compose、overlay、closed Compose、enabled/closed config 和 allowlist 均按 commit 后缀保存，没有覆盖未带版本的 D1 文件。D2 专用 cache 位于媒体映射之外，宿主实时权限为 `10001:10001`、`0700`；allowlist 为 `10001:10001`、`0400`，SHA-256 为 `46739a2aa116f318c6b1c283cb349d2471a1bcb7528dc315db34bd50c8865917`。

Docker Compose 对 file-source Secret/config 的 `uid`、`gid`、`mode` 给出“不支持并忽略”的警告，因此验收以宿主文件实际 owner/mode 和容器内应用用户可读性为准；四份 Secret 均只读可读，D2 cache 由应用用户可写。

## 3. 真实 Canary 结果

### 3.1 Search

| 检查 | 结果 |
|---|---|
| HTTP 状态 | `200` |
| 服务端 canonical language | `zh-CN` |
| 返回候选数 | `20` |
| 首个候选 Token 存在 | 是（仅内存/服务端变量，未输出） |
| `truncated` | `false` |

### 3.2 Fetch

第一个候选返回 `502 candidate_fetch_failed`，该失败只影响该候选；按照固定的最多两次候选预算尝试第二个候选，未重试第一个。

| 检查 | 结果 |
|---|---|
| 第二候选 HTTP 状态 | `200` |
| 服务端 canonical language | `zh-CN` |
| 格式 | `srt` |
| 规范化字节数 | `41072` |
| 解析 cue 数 | `603` |
| `preview_ready` | `true` |
| Artifact Token 存在 | 是（仅内存/服务端变量，未输出） |

### 3.3 Preview

| 检查 | 结果 |
|---|---|
| HTTP 状态 | `200` |
| language / format | `zh-CN` / `srt` |
| Artifact 总字节数 / cue 数 | `41072` / `603` |
| 请求分页 | `offset=0`、`limit=20` |
| 返回 cue 数 | `20` |
| `truncated` | `true` |
| Provider Fetch 重复调用 | 本次 Preview 请求没有再次调用 Provider Fetch |

Preview 只返回了有界的 cue 元数据和纯文本投影；本报告没有记录任何字幕正文。

## 4. Canary 关闭后的实时状态

Canary 成功后立即使用版本化 closed Compose/config 重建 app：

| 检查 | 结果 |
|---|---|
| app | `running` / `healthy`，restart count `0` |
| `/livez` / `/readyz` | `200` / `200` |
| `write_enabled` | `false` |
| `remote_search_enabled` | `false` |
| `d2.canary.enabled` | `false` |
| 关闭后的 Search | `403 remote_search_disabled` |
| app 用户、rootfs、媒体挂载 | 仍为 `10001:10001`、只读 rootfs、只读 `/media` |
| 唯一可写挂载 | D2 专用 cache；不在媒体目录内 |

D2 镜像代码仍保留在 C92 当前 app 中，但 D2 功能已关闭；后续若要重新开放，必须重新授权一个新的有界 Canary 窗口并复核 allowlist、缓存和回滚点。

## 5. 未通过或尚未覆盖的门禁

- 本次只证明 allowlist 中一个真实单源 Item 的 Search、候选级失败隔离、Fetch、字幕校验和 Preview；**不证明多 MediaSource 支持**。真实多源样本仍未找到，D2 对多源继续 fail closed，D3 仍未授权。
- 未做真实浏览器/公网客户端的 D2 UI 验收；本地 Fake Emby Playwright 流程另有自动化证据，不能替代 C92 真实客户端证据。
- 本机 `go test -race ./...` 仍受 Windows 缺少 gcc 阻断；C92 本次未运行 race 检查。
- C92 构建时 Docker Compose 提示未安装 buildx，镜像仍成功构建；本次引用以 Git commit、Image ID 和 OCI revision 交叉核对。
- 本报告不把一次真实单源 Canary 推断为可执行的字幕保存/替换能力；D3 写入路由和媒体写入仍未实现、未调用。

## Knowledge Review

任务或阶段：D2-C C92 单源真实 Canary、可回滚部署和窗口关闭。

### Knowledge Findings

- 新增运行事实：真实 Provider 可能让首个候选返回 `candidate_fetch_failed`，候选级隔离和第二候选有限尝试可以在不放大预算的情况下取得可预览 Artifact。
- 新增验收边界：D2 Canary 成功后必须立即关闭 `remote_search_enabled` 与 `d2.canary.enabled`；部署保留 D2 镜像但默认不开放搜索。
- 没有新增架构决策；多源、D3 写入和真实客户端验收边界保持原 ADR/契约结论。

### 证据

- 代码与镜像：Git commit、C92 source clone、Image ID、OCI revision/version/source。
- 真实运行：C92 Search 200、首候选 502 隔离、第二候选 Fetch 200、Preview 200；关闭后 health flags 和禁用路由 403。
- 安全边界：容器用户、只读 rootfs、只读媒体挂载、Secret 实际权限、cache 实际权限、版本化 Compose/config 和 D1 回滚备份。

### 分流判断

- `docs/architecture.md`：不更新；本报告记录部署事实，不改变已接受架构。
- `docs/adr/`：不新增；本次结果符合既有 D2 契约和 ADR-005。
- `docs/lessons-learned.md`：本报告保留一次性真实 Canary 证据；候选失败隔离规则已在既有维护经验和契约中，不重复追加。
- `LOCAL_OPERATIONS.md`：不更新；没有新增长期连接、拓扑或凭据位置。

### 去重检查

已搜索 D2 搜索预览契约、D2-B1/B2 评审、D2-C 预检、D2 发布候选审计、Phase 2 Canary、ADR-005、维护经验和架构文档；本报告只新增本次 C92 真实运行和关闭后的证据。
