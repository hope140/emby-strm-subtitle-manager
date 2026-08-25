# D3 C92 Canary 验收

日期：2026-08-25（Asia/Hong_Kong）
范围：C92 应用容器的 D3 专用单源 Add；SH、FRP、OpenResty、公网反代和 Emby 其他配置不在本轮范围。

## 结论

D3 专用样本 Add 已通过 C92 真实闭环验收。验证链为：单源 allowlist → Search → Fetch → Preview → Artifact 绑定 → 原子 sidecar 写入 → Emby Refresh/MediaStreams → Hash 和字幕流回读 → Emby Web 客户端读取 → 幂等重放。验收窗口结束后已恢复 closed 配置，写入、远程搜索和媒体可写挂载均关闭。

这不等于开放全库写入。Replace、Delete、Upload、批量写入以及多源正向写入仍未实现或未开放。

## 发布与本地验证

- 公开提交：`a184cf1`（D2/D3 共享已核验 allowlist generation）
- C92 镜像：`emby-strm-subtitle-manager:d3-a184cf1`
- 镜像 SHA-256：`sha256:7be12350fea3bcff6695c004b34e5cc6e49d500759be178816546e42d9b234ff`
- OCI revision：`a184cf19b81b338ba2feb1e050fdca9b51af6f03`
- OCI version：`0.3.0-d3.1.1`

本地已通过：

- `go test ./...`
- `go vet ./...`
- `go build -trimpath ./cmd/server`
- `scripts/verify.ps1 -GoPath .tools\\go1.26.7\\go\\bin\\go.exe`
- D2 UI Playwright E2E、前端语法检查、敏感信息扫描和 `git diff --check`

`go test -race ./...` 未通过本机验证，原因是当前环境未启用 CGO 且没有可用 GCC；这不影响本轮普通测试、vet、构建和真实 Canary 结果。

## C92 部署前后边界

D3 使用独立版本化 Compose/config、D2/D3 allowlist、Preview cache、history 和 quarantine。Compose 合并预检通过，容器运行 UID 为 `10001:10001`，rootfs 保持只读；D3 窗口内 `/media` 临时为 `RW=true`，history/quarantine 为私有可写目录，Secret 在容器内可读且宿主权限已核对。

真实样本目录原先为 `root:root 0755`，因此首次 Add 在原子提交阶段得到权限错误。随后只对 allowlist 指定的精确目录临时改为容器 UID 可写，完成验收后恢复为 `root:root 0755`。没有对媒体库递归改属主，也没有修改其他目录。

窗口结束后使用 `shared/compose.a184cf1-closed.yaml` 重建应用并核对：

- 容器 `running/healthy`，restart count 为 0，镜像仍为 `d3-a184cf1`
- UID `10001:10001`，rootfs `ReadonlyRootfs=true`
- `/media` `RW=false`
- `write_enabled=false`、`remote_search_enabled=false`
- 认证管理员会话调用 Add 返回 `403 write_disabled`
- closed 配置调用 Search 返回 `403 remote_search_disabled`
- `frpc-sh` 仍为 running、restart count 为 0，容器 ID 未变化

SH 的 `frps`、OpenResty、FRP 配置和公网 18080 没有修改。

## 真实 D3 证据链

目标是服务端 allowlist 中的单源 Episode，服务端重新读取 Item、source、路径映射和安全 containment；没有猜测 MediaSource，也没有把前端路径或 Provider 原始 ID交给写入层。

1. Search 返回 HTTP 200，共 20 个候选；第一个候选失败被隔离，未使整次搜索失败。
2. 第二个候选 Fetch 返回 HTTP 200，格式为 SRT，内容 41,072 字节，校验通过。
3. Preview 返回 HTTP 200，Artifact 与 Item、source、allowlist generation 和 D2 上下文绑定。
4. Add 返回 HTTP 200，生成一个版本化 `.subbridge` sidecar；文件为 41,072 字节，内容 SHA-256 为 `3c82277505113476c3c0c8448ddaccb53fc7769940a77f5d7e262827d7071f68`。
5. 文件属主为 `10001:10001`、权限为 `0644`；临时文件为 0，sidecar 数量为 1，history 记录为 1，quarantine 文件为 0。
6. Emby Refresh 成功，随后轮询到选中 source 的新字幕 MediaStream。官方 Refresh 接口为 [ItemRefreshService](https://dev.emby.media/reference/RestAPI/ItemRefreshService.html)。
7. 通过官方字幕流接口读取 41,072 字节，Hash 与写入内容一致；接口形态见 [Emby Subtitles](https://dev.emby.media/doc/restapi/Subtitles.html)。
8. 应用只读字幕清单返回 HTTP 200，包含与新 sidecar 对应的外部 SRT 条目。
9. 使用同一 operation ID 重放返回 HTTP 200，Hash 和文件结果一致；sidecar 数量仍为 1，history 仍为 1，没有生成第二个副本。

第一次真实 Add 还发现了两个可复用问题：旧构建中 D2 与 D3 各自读取 allowlist 会产生不同 generation，已由 `a184cf1` 改为共享同一个已核验 allowlist；宿主目录权限不能由 Compose 的 `RW=true` 自动修复，已通过精确目录预检和恢复步骤固定下来。两次失败尝试均未留下媒体文件或 history/quarantine 记录。

## 实际客户端读取

通过 C92 的 Emby Web 入口打开同一单源样本详情页，页面显示外部 `Chinese Simplified (SRT)` 字幕条目，并标记为可用的外部 SRT。选择该 SRT 轨道后启动播放，HTML5 video 已进入播放状态（`readyState=4`、播放时间持续前进），播放器加载了 `zh-CN` 字幕轨道；本次浏览器控制台没有字幕加载错误。

本次使用的是浏览器 DOM/播放器状态证据，没有把 Cookie、Token、凭据或媒体标识写入报告；未把截图作为验收依据。

## 回滚与后续边界

- 当前 C92 已回到 closed 运行状态，D3 versioned Compose/config、history 和 quarantine 作为可回滚/审计材料保留在服务器私有目录。
- 重新开启 D3 必须重新核对镜像摘要、allowlist generation、精确样本目录权限、Secret 可读性、`/media` 可写窗口和回滚点，并获得独立授权。
- 下一阶段可以讨论 Replace 或更通用的批处理设计，但不能把本次单源 Add 直接扩展为全库写入。
