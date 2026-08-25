# D3 专用样本 Add 契约

状态：本文件保留 D3.1 专用样本 Add 的实现与 C92 真实验收记录，Emby Web 与手机端实际客户端读取均已确认；验收记录见 [D3 C92 Canary 验收](d3-c92-canary-acceptance-20260825.md)。Core A/B 已在本地实现日常 gate、正向多 source、Replace、Upload、可恢复 Delete 和 Restore；当前实现契约见 [ADR-008](adr/008-core-ab-daily-source-bound-recovery.md) 与 [Core A/B 实现评审](core-ab-implementation-review.md)，不代表已部署或完成新的真实 C92 综合验收。

## 历史 D3.1 Canary 边界

以下章节记录 D3.1 当时的专用 allowlist、单 source Add 和 C92 证据。Core A/B 继续复用其中的认证、Artifact、Item 锁、原子写入、Refresh、Hash、history 和 quarantine，但不再以“多 source 一律 409”或“只提供 Add”为当前源码行为。默认 `write_enabled=false`、最小可写 overlay、目录权限预检和部署授权仍保持有效。

## 入口与门禁

写入口为：

```text
POST /v1/media/{item_id}/subtitles/add
```

请求体固定为 JSON 对象：

```json
{
  "artifact_token": "短期服务端 Artifact Token",
  "media_source_id": "服务端重新读取到的唯一 source ID",
  "operation_id": "调用方生成的幂等操作 ID"
}
```

服务端在每次 Add 前重新读取 Item 和 MediaSource，不信任前端路径、文件名或候选原始 ID。当前版本只接受单一 Movie/Episode，Item 必须同时位于 D3 专用 allowlist，Artifact 必须与 Item、source、allowlist generation 和服务端 D2 上下文绑定。多源返回 `409 d3_multisource_unsupported`，不会猜测 source。

默认配置仍为：

```yaml
features:
  write_enabled: false
```

开启写入必须同时满足 D2 搜索开关、D3 Canary allowlist、独立 `subtitle:write` Bearer scope、D3 history/quarantine 私有目录和 D3 Compose overlay。基础 Compose 不提供可写媒体挂载。

D3 overlay 只负责把应用的 `/media` 挂载临时切换为可写；目标媒体目录本身还必须允许容器运行 UID `10001:10001` 创建 sidecar。这个权限只应在专用验收窗口对明确样本目录临时授予，不能用全库递归 `chown` 替代路径核验。窗口结束后恢复默认目录属主和 `/media:ro`。

管理员浏览器会话必须带登录时签发的短期内存 CSRF Token，写请求使用 `X-CSRF-Token`。带 `subtitle:write` 的 Bearer 自动化请求不使用浏览器 CSRF，但仍受 Item allowlist、单 source、Artifact 绑定和路径门禁约束。跨站 Origin 和 `Sec-Fetch-Site: cross-site` 被拒绝。CSRF Token 不写入 URL、Cookie、浏览器存储、日志或 Git。

## 写入和恢复

1. 重新读取 Item、选择明确 source，使用 PathMapper 和 PathGuard 验证目录在媒体根内。
2. 读取并校验短期 Artifact 内容及 SHA-256。
3. 在目标目录创建临时文件，`fsync` 后使用同目录硬链接完成不覆盖的原子提交。已有文件不会被修改，冲突按确定规则生成 `.v2` 到 `.v100` 版本名。
4. 立即重新读取文件并核对字节数和 Hash。
5. 调用官方 Emby `POST /Items/{Id}/Refresh`，随后有界轮询 `GetItem`，直到所选 source 的字幕 MediaStream 映射到新文件。
6. 写入 history JSON。Refresh、可见性核验或 history 失败时，新文件移动到独立 quarantine 目录；旧字幕不受影响。

`operation_id` 在内存和 history 中以 SHA-256 文件名保存。相同操作重放会返回同一结果，不创建额外版本；若历史记录对应文件已不存在，服务端会重新进入安全写入流程。响应只公开安全 basename、格式、字节数和内容 Hash，不公开本地绝对路径、候选原始 ID 或凭据。

## 运行示例

D3 需要在明确授权的临时窗口组合：

```text
docker compose \
  -f compose.example.yaml \
  -f compose.d2-canary.example.yaml \
  -f compose.d3-canary.example.yaml \
  up -d --build app
```

`compose.d3-canary.example.yaml` 仅将 `/media` 替换为可写 bind，并增加 history、quarantine 和 D3 allowlist；rootfs 仍然只读。验收结束后恢复 closed 配置和 `/media:ro` 基础边界，并恢复样本目录原有权限。

## 明确未承诺

- 不提供 Replace、Delete、Upload、批量任务或用户级权限。
- 不把文件存在、Refresh 返回 2xx 或应用 API 200 单独视为成功。
- 本阶段的真实闭环已在 C92 完成：文件 Hash、Emby Refresh/MediaStreams、官方字幕流和 Emby Web 客户端读取均已记录；后续能力仍不得越过本契约边界。
