# Core A/B C92 综合验收现场清单

- 准备日期：2026-08-26（中国标准时间）
- 候选：`5deaf519f69ba1226840836516c07124965a4afc`
- 范围：仅 C92 上的 SubBridge app-only 受控窗口；不修改 Emby、SH、FRP、OpenResty 或媒体库全局权限。
- 本清单不是部署授权。开始 daily 窗口、临时目录授权和真实写入仍须由操作者在现场明确确认。

## 已完成的准备

- C92 当前运行候选已只读核对为 healthy、UID 10001、根文件系统只读、`/media:ro`；服务保持 closed。
- 本地受控上传文件已存在：[upload-1](../scripts/testdata/core-ab-upload-1.srt) 与 [upload-2](../scripts/testdata/core-ab-upload-2.srt)。它们只在 UI/API Upload 时生成短期 PreviewArtifact，不直接落盘。
- 2026-08-26 单源 STRM 的服务端闭环已独立通过；本次综合窗口应复用其受控样本选择方式，但开始前仍须重新读取并校验当前 Item/source。
- 对整个 Emby 库的只读样本计数预检在有限超时内未完成，因此不会根据旧扫描结果猜测普通本地媒体或多源 STRM 样本。两类样本必须在现场以实时详情确认。

## 样本门禁

不得在本文件、聊天记录或公开报告中记录 Item ID、MediaSource ID、媒体标题、绝对路径、凭据或认证 URL。只在服务器端的短期 allowlist/会话中保存现场选中的精确标识。

| 类型 | 现场必须确认 | 状态 |
|---|---|---|
| 单源 STRM | `Item.Path` 映射的 `.strm` 为普通文件；恰好一个 source；显式 source 绑定；目录原属主和模式已记录 | 可复用既有受控样本，仍须实时重读 |
| 普通本地 Movie 或 Episode | 非 STRM；选中 `MediaSource.Path` 可映射到 `/media`；锚点为普通文件；目录原属主和模式已记录 | 待现场选择 |
| 多源 STRM | `Item.Path` 为 `.strm`；详情中 source 数大于一；不赋予媒体目录写权限 | 待现场选择 |

任何一类样本缺失、路径不安全、文件非普通文件、source 变化、allowlist 不匹配或目录恢复值未知时，跳过该分段，不用其他 Item 或 source 替代。

## 一次 daily 窗口的执行顺序

### 1. 打开前预检

1. 记录当前 closed 镜像/配置作为 app-only 回滚点。
2. 确认候选 OCI revision、版本、UID、只读根文件系统、`/media:ro`、`/readyz` 和未认证健康边界。
3. 为每个现场样本重新读取 Item/source；只把本次精确 ID 写入私有 allowlist。
4. 记录每个待写入目录的原属主和模式；仅单源 STRM、普通本地样本的**父目录**可临时变为对 UID 10001 可写，禁止递归 chmod/chown。
5. 确认 D2 预览、D3 history/quarantine/archive/trash 私有目录可用，且不与媒体目录重叠。

### 2. 打开 daily

1. 只重建 SubBridge app，启用 daily 所需 feature flags、allowlist、私有目录和可写 overlay。
2. 再次检查健康、认证、UID、只读根文件系统和允许写入的精确媒体挂载。
3. 在 UI 以管理员会话登录；不读取或记录密码、Cookie、Bearer 或 CSRF 值。

### 3. 单源 STRM：UI、Provider、Upload 与播放

1. 在 UI 选择现场单源 STRM 和显式 source。
2. 执行真实 Provider `Search → Fetch → Preview`；若候选/内容校验失败，按候选失败记录，不能把失败候选强行写入。
3. 对成功 PreviewArtifact 执行 UI Add；核对 Hash、Refresh、MediaStreams 和官方字幕流。
4. 用 Emby Web 和至少一个实际客户端播放，确认字幕可见且可选择。
5. 通过 UI 选择 `upload-1`，执行 Upload→Preview→Replace；核对 Hash、Refresh、MediaStreams 和官方字幕流。
6. 通过 UI Restore Replace，核对恢复后的字幕流语义内容；再执行 Delete→Restore→最终 Delete。
7. 用新鲜服务器端 Inventory 确认测试 sidecar 不存在；不要只依据 UI 历史或流展示判断清理结果。

若真实 Provider 的 Add 未完成，本段只能记录为 Upload 路径通过，不能称为 Provider 闭环通过。

### 4. 普通本地媒体：回归边界

1. 仅在现场找到通过样本门禁的普通本地 Item 后执行。
2. 使用 `upload-1` 完成 Upload→Preview→Add；确认写入锚点仍是选中 `MediaSource.Path`，不是 Item fallback。
3. 使用 `upload-2` 完成 Replace→Restore，再执行 Delete→Restore→最终 Delete。
4. 每个成功写入或恢复后检查 Hash、Refresh、MediaStreams 和官方字幕流；最终用 Inventory 确认测试 sidecar 不存在。

### 5. 多源 STRM：只验证拒绝

1. 不为该 Item 开放媒体目录写权限，也不准备写入 Artifact。
2. UI 必须保留 Search、Fetch、Preview/Upload 的只读入口，并隐藏或禁用 Add、Replace、Delete、Restore，显示稳定中文原因。
3. 以已认证 API 对四类 D3 操作分别确认 `409 strm_multisource_write_unsupported`；每次响应后重读 Inventory，确认没有媒体或 recovery 材料变更。

### 6. 清理与关闭

1. 完成每个可写分段的最终 Delete，并以服务器端 Inventory 核对 sidecar 已消失。
2. 恢复每个曾临时授权目录的原属主和模式；再次确认没有递归修改。
3. 仅重建 SubBridge app 回到 closed：`write_enabled=false`、`remote_search_enabled=false`、`/media:ro`。
4. 最后核对容器 healthy、UID 10001、根文件系统只读、`/readyz` 成功、未认证健康请求被拒绝；不得顺带重启或修改其他服务。

## 通过口径与记录

- 单源 STRM Provider/UI/播放、普通本地媒体、以及多源 STRM 拒绝必须分别报告，不能互相替代。
- 每段记录为“通过 / 失败 / 因样本门禁跳过”，并仅记录脱敏的证据类别、时间和稳定错误码。
- 任一写入后 Hash、Refresh、MediaStreams、官方字幕流或清理核验失败时，立即停止扩大范围，保留 recovery 材料供人工恢复，并按 closed 流程收尾。
- 本次结束后新建阶段报告；不得回写或删除 [2026-08-25 旧阻断报告](core-ab-c92-acceptance.md) 或 [2026-08-26 单源 STRM 报告](core-ab-c92-acceptance-20260826.md)。
