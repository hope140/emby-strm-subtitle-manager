# 维护经验

本文件只保存经过官方接口、当前源码、测试、日志或真实运行证明，并且后续开发容易再次踩到的结论。完整过程和样本见 [Gate 0 实测报告](../GATE0_REPORT.md)。

## 1. 后端凭据必须脱离浏览器会话验证

登录后的 Emby Web 请求只能证明当前用户会话可用。正式后端采用 API Key 时，需要用不带 Cookie 的独立请求验证 Search 和 Fetch。

Gate 0.1 已证明当前 API Key 能独立完成两项调用。后续兼容性验收仍要保留 API Key 路径，不能只在浏览器中点通。

## 2. Emby Bridge 不提供请求级 Provider 和关键词控制

远程字幕搜索接口接受 Item、MediaSource、语言和 forced 等条件。Gate 0 添加 `ProviderName` 与 `SearchTerm` 后，响应候选数量和可见字段没有变化。

UI 中的 Thunder、ASSRT 标签只能筛选和排序返回结果。需要自定义关键词或精确 Provider 调度时，再评估 Native Provider。

## 3. Search 返回候选不能证明候选可下载

Thunder 会返回上游地址已经失效的候选。Gate 0.1 中一个候选的上游地址返回 HTTP 404，随后被 Meiam 和 Emby 包装为对外 HTTP 500，同一结果集中的其他候选可以正常 Fetch。

候选失败必须隔离。上游 4xx、格式无效和内容校验失败不重试，临时网络错误最多重试一次。自动模式按排序尝试的候选数量必须有上限。

## 4. STRM 网络安全需要观察真实目标

代码检查能够说明预期，抓包或目标端日志才能证明受控操作期间有没有访问 STRM 内部地址。

Gate 0 对已知媒体代理端口的受控监测没有捕获流量。这个结果只覆盖当时的样本、操作和目标端口，不能扩大成所有 Provider 和所有部署环境的永久证明。

## 5. 文件读取行为不能用 atime 单独证明

Linux `relatime` 可能让文件被读取后仍不更新 atime。需要证明进程是否打开 STRM stub 时，应使用允许的进程级文件打开监测或等价证据。

当前 Meiam 的 CID 行为由源码确认。Gate 0 没有完成 live open trace，报告必须保留这条证据边界。

## 6. Refresh 返回成功后仍要读取字幕内容

文件存在和 Emby Refresh 成功都不能证明客户端会读到正确字幕。最低验收需要确认 Emby MediaStreams 出现新流，并通过字幕流接口读取内容。

涉及反向代理和客户端缓存时，还要验证对应真实访问路径。同路径测试带 `no-cache` 只能证明绕过缓存后的结果。

## 7. 备份和旧字幕不能留在媒体目录中等待清理

Emby 可能把媒体目录中的 `.ass`、`.ssa` 和 `.srt` 备份再次识别为字幕。Installer 的回收、历史和失败隔离文件应放在媒体库外的项目数据目录。

替换默认先写新版本文件，确认 Emby 和实际访问路径可读取后再归档旧文件。

## 8. 自动化验证不能代替真实客户端验收

单元测试证明局部逻辑，Fake Emby 证明接口协作，直连请求证明服务端响应。实际客户端的字幕发现、缓存和播放仍需要客户端验收。

交付报告必须分别列出静态检查、集成测试、Emby 直连和真实客户端结果，不能合并成一句全部通过。

## 9. 有界多源扫描不能证明全库无多源

按最新排序页检查真实 Movie/Episode 只能证明该样本范围未命中。若门禁要求真实多 MediaSource Item，应记录媒体库数、取样策略、Item/请求上限和实际覆盖量；首段、中段、尾段取样仍然只是有界证据，不能把未命中扩大成全库不存在。

当缺少这种边缘样本会无限期阻断已有独立证据支撑的单源工作时，应通过 ADR 明确拆分门禁，不能在阶段报告里悄悄放宽：允许单源能力继续推进，同时让未经真实验收的多源路径 fail closed，并保留多源支持声明与启用的独立门禁。本项目的具体决策见 [ADR-005](adr/005-conditional-d2-entry-without-live-multisource.md)。

## 10. HttpOnly 会话的浏览器验收不能只看 JavaScript 存储

管理员登录会把短期会话放在 `HttpOnly` Cookie 中。页面脚本读取不到它，所以 `document.cookie` 为空是预期结果，不能据此断言服务端没有会话。验收必须同时检查 Set-Cookie 的 `HttpOnly`、`SameSite`、`Secure` 和 TTL 属性，并用带 Cookie 的只读 API 请求证明会话确实生效。密码输入框提交后应立即清空；刷新页面回到登录界面不代表 Cookie 被脚本持久化。

## 11. 目标环境必须以实时容器证据为准

本地操作手册、阶段报告和另一条会话留下的部署快照都可能落后于目标环境。迁移或回滚前先用 `docker inspect` 核对当前镜像、OCI revision、Compose `config_files`、working directory、挂载、rootfs、网络模式和 health 状态，再决定从哪个 Compose 文件继续。容器标签是发现实际版本化 Compose 文件的可靠入口；不能只按文档中的默认路径猜测。

## 12. Compose 路径错误不能当成服务配置错误

`docker compose config --quiet` 在错误工作目录下会报告没有配置文件。这个结果只证明调用位置或 `-f` 参数不对，不证明 YAML 无效。应先从容器标签取得实际 `config_files`，再用相同文件组合执行 `config --quiet` 和 `ps`，并把两类结果分开记录。

## 13. 远程运维优先使用专用非交互通道

XTerminal 已提供带连接状态的 SSH MCP。先列出服务器并按明确的 C92 条目选择，再执行有界、可审计的非交互命令；不要通过桌面点击猜测当前连接，也不要把上海 SH 与 C92 混为一个目标。部署前仍需独立保留回滚点和用户授权，MCP 可用不等于可以跳过安全预检。

## 14. 管理员认证迁移必须先识别现有功能窗口

同一 C92 可能已经运行 D2 closed Canary、allowlist 和预览缓存。切换管理员 environment 前先确认当前镜像、配置代际和已有服务端凭据集合，避免把 D2.5 environment 写入错误的 Compose 组合。迁移失败时只回滚 app 服务，并保持 `remote_search_enabled=false`、`write_enabled=false`，不联动重启 SH/FRP/OpenResty。

## 15. Compose 文件权限必须用容器内实际读取验证

Docker Compose 对 file-source Secret/config 的 `uid`、`gid`、`mode` 字段可能只给出 warning 并忽略。部署前应让目标镜像以实际运行 UID 执行 `test -r`，并把非凭据 config 的宿主权限设为该 UID 可读；管理员 environment 所在私有 Compose 可以保持 root-only。C92 的 b9916d1 迁移用 `root:root 0600` 私有 Compose、`root:root 0644` config 和 UID 10001 容器读取测试通过，避免把 YAML 权限假象当成运行时证据。

## 16. Emby 4.9.x 版本组详情必须请求 AlternateMediaSources

Emby 的版本组在列表查询中可能表现为多个关联 Item，并且默认只带一个 `MediaSource`。对选中的 Item 做详情读取时，`Fields` 必须显式包含 `AlternateMediaSources`，服务端才会返回完整的版本 source 列表。2026-08-25 在 C92 的已知 Movie 版本组上复核：省略该字段时每个结果只有一个 source；加入该字段后每个详情 Item 返回两个 source。D2 客户端不能把列表结果或默认 source 当成完整事实，必须固定请求字段并在服务端按完整 source 列表执行多源 fail-closed 检查。

## 17. 自动化 Bearer 的只读 scope 应在路由边界执行

只有在配置层写一个“只读 Token”还不够，HTTP 层必须按路由检查 `media:read`、`subtitle:search` 和 `subtitle:preview`，缺少权限稳定返回 403；未来 `subtitle:write` 在写能力关闭时直接拒绝。这样可以保持单 Token 和简单 Compose 配置，同时避免未来新增写路由时意外继承当前的全路由 Bearer 放行。管理员 HttpOnly 会话与自动化 Bearer 是两种不同认证主体，写入能力仍需单独的 CSRF、scope 和真实验收门禁。

## 18. D3 Add 的成功必须是一条完整证据链

D3 首次写入不能把“文件存在”或“Refresh 返回 2xx”当作完成。服务端必须先用 Artifact 和 Item/source 重新绑定，再以临时文件、同目录非覆盖原子提交生成版本化 sidecar；随后 Refresh、轮询 MediaStreams、直接 Hash 读取和实际字幕流/客户端读取分别记录。Refresh、轮询或 history 失败时，新文件移动到媒体库外 quarantine，避免失败副本再次被 Emby 识别。操作 ID 需要同时保存在内存和 history，重放不能创建额外副本。

## 19. D3 的可写 overlay 不等于媒体目录可写

容器 `/media` 切换为 `RW=true` 后，宿主媒体目录仍可能是 `root:root 0755`，此时运行 UID `10001` 会在原子提交阶段得到 `permission denied`。真实 C92 Canary 只对 allowlist 指定的一个样本目录临时授予容器 UID 写权限，完成 Hash、Refresh、MediaStreams、字幕流和客户端核对后恢复原属主与 `0755`，再关闭 D3 overlay。后续部署必须把“精确目录权限预检”和“关闭后 `/media:ro`”作为独立证据，不能递归修改整个媒体库。

## 20. 可恢复多 source 操作必须保存目录类别，不能保存或猜测路径

STRM 的 Item.Path 和选中 MediaSource.Path 可以拥有不同的 basename 或目录。Inventory 因此只能在两个受 PathGuard 约束的范围内扫描；若同一个 sidecar basename 出现在不同安全位置，必须标记冲突并拒绝修改。

Replace/Delete 的 history 若只在 Restore 时使用当前 source 的写入目录，会把原先位于 Item 目录的 sidecar 恢复到错误位置。Core A/B 的恢复记录只保存 `item` 或 `source` 目录类别，不保存媒体路径；Restore 重新读取当前 Item/source、核对 Hash 和无覆盖目标后才恢复。该规则由 `internal/inventory`、`internal/d3` 的回归测试和本地 Fake Emby 流程覆盖，真实多 source 客户端验收仍需单独完成。

## 21. 可恢复事务的补偿失败必须成为公开的人工恢复边界

Replace、Delete、Restore 或 Add 的失败分支不能把恢复旧文件、移除新文件、quarantine、Refresh 或 history 补偿写成 best-effort 后继续返回原错误。补偿步骤必须统一执行，恢复后重新读取并核对文件 Hash，再通过 Refresh/MediaStreams 验证最终可见性；任一步无法验证时返回稳定 `subtitle_rollback_failed`，并保留 archive、trash 或 quarantine，不删除可用于人工恢复的副本。该边界由 `internal/d3` 的 restore、remove、quarantine、rollback Refresh 和 history 失败注入测试覆盖。

## 22. 成功补偿不能让同一 operation ID 变成不可重试

成功 rollback 会按设计保留 archive、trash 和 quarantine；这些文件再次遇到时不能一律视为冲突。`moveToRecovery` 必须先重新核对已有恢复文件的 Hash：只有与当前已核验源文件完全一致时才可复用并移除当前媒体副本，Hash 不同或文件不安全时继续拒绝。这样 Replace/Delete 的同一 `operation_id` 可在成功补偿后安全重试，同时不会把不同内容误当成同一恢复材料。

## 23. daily write 配置必须先通过四目录和挂载双重门禁

`write_enabled=true` 时，`d3.history_dir`、`d3.quarantine_dir`、`d3.archive_dir` 和 `d3.trash_dir` 都是启动必需项。Compose 中存在 RW 挂载不能替代 config 字段校验；缺字段时应用应在启动阶段 fail closed。本次 C92 Core A/B 尝试第一次 daily 启动正是因为复制配置缺少四个字段而被拒绝，补齐版本化 config 后才通过启动预检。

## 24. Item.Path 可映射不代表选中 MediaSource.Path 可写

Core A/B 的写入目标必须从当前选中的 MediaSource.Path 重新解析，并通过 PathMapper、PathGuard 和目录权限检查。C92 有界真实查询中，Movie/Episode 的 Item.Path 可以映射到 `/media`，但样本的选中 source path 是远程播放 URL；没有任何候选满足本地 source path 门禁。不能把 Item.Path、默认 source 或 source 顺序当作 fallback，否则会把多 source 写入或恢复导向错误版本。重新进入真实写窗口前，应把“至少一个可映射 source path”作为独立前置门禁。
