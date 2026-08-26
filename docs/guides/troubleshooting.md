# 故障排查

本页只给出安全、可行动的排查方向。不要在问题单、浏览器地址栏、日志摘录或截图中附上密码、Token、Cookie、原始 Provider ID、带认证参数的 URL 或媒体绝对路径。

## 容器未启动或不健康

先运行 Compose 配置合成检查，确认配置文件和三个服务端凭据文件存在且应用运行 UID 可读。检查镜像引用、配置语法、非 root 用户、只读 rootfs、`/tmp` tmpfs 和只读 `/media` 挂载是否仍保持示例的安全语义。

若 `/livez` 正常而 `/readyz` 失败，应用进程可能已启动但无法完成 Emby 就绪探测。检查 Emby URL、API Key 文件、网络可达性和超时设置；不要为了让 `/readyz` 通过而把它降级成纯进程检查。

## 无法登录或认证被拒绝

管理员登录依赖私有 Compose 的 `APP_ADMIN_USERNAME` 与 `APP_ADMIN_PASSWORD`。确认变量被 app 服务读取，但不要输出其值。浏览器 HTTPS 部署时还应核对 session cookie 的 Secure 设置；会话过期时重新登录。

自动化 Bearer 与管理员会话不同。出现 401/403 时，确认请求没有把 Bearer 放在 query 参数中，并检查调用路由所需 scope。不要把 Token 粘贴到 UI 或日志中。

## 媒体或字幕未显示

确认 Emby Item 是受支持的 Movie 或 Episode，且 Emby 报告路径能够映射到容器只读 `/media`。普通本地媒体使用选定 MediaSource 的本地路径；STRM 使用本地 `.strm` Item.Path 作为 sidecar 锚点。远程播放 URL 不是可写本地路径。

多版本 STRM 必须先在 SB 选择目标版本。Add、Replace、Delete、Restore 仍以 Item.Path 的本地 `.strm` 目录为写入锚点，并将操作绑定到选中 source；不要用远程 URL 推导文件名。

## 搜索或写入入口不可用

远程搜索入口依赖 `remote_search_enabled=true`、当前媒体源选择和服务端 admission gate。写入入口还依赖 `write_enabled=true`、写 scope、允许的 Item、可管理字幕、路径/权限检查和 Artifact 绑定。

首次安装和常规升级应保持两个开关关闭。若功能因开关或 gate 被拒绝，先恢复只读流程并按独立 Canary 计划重新预检，而不是在浏览器端修改请求或直接写媒体目录。

## 恢复、替换或回收区操作失败

恢复不会覆盖同名现有字幕；目标冲突、Hash 不一致、历史不可用或 STRM 历史位置不兼容时都会拒绝。保留 archive、trash 和 quarantine，重新加载当前媒体源的历史摘要后按稳定错误说明处理。不要手工覆盖媒体目录中的同名文件。

## 仍无法定位

收集不含敏感信息的版本、镜像 digest、健康状态、错误码与时间范围。先确认是否属于当前项目已支持的单源边界；普通本地媒体、多源 STRM 写入、批量和自动下载仍有独立限制。必要时按[升级与回滚](upgrade-rollback.md)回到最后已验收的 closed 镜像引用。
