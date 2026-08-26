# 安装 SubBridge

本指南面向新的、只读默认安装。首次安装不会启用远程字幕搜索或任何媒体写入；它们需要单独授权、预检和验收。

## 安装前准备

- 一台可运行 Docker Compose 的 Linux 主机。
- 可访问的 Emby 实例，以及仅供 SubBridge 使用的 Emby API Key。
- 一个与 Emby 报告路径对应的媒体根目录。首次安装必须以只读方式挂载。
- 三个独立的服务端凭据文件：Emby API Key、应用 identity key、自动化 Bearer token。它们不进入 Git、URL、浏览器存储或日志。
- 管理员用户名和密码，以私有 Compose `environment` 直接提供；不要写入 `.env`、配置 YAML 或 Secret 文件。

选择拓扑：Emby 可通过普通容器网络访问时使用 bridge Compose；Emby 只监听宿主机网络时使用 host-network Compose。两种拓扑只能二选一，不能同时合并。

## 准备部署目录

将选定的 Compose 示例、对应配置示例与 `secrets` 目录复制到受保护的部署目录。配置和凭据文件应只允许部署管理员读取，且部署目录不应位于媒体目录内。

1. 用 `deploy/config.example.yaml` 或 `deploy/config.host-network.example.yaml` 建立本地 `config.yaml`。
2. 将 `emby.url`、`path_mappings` 中的 Emby 路径和宿主媒体挂载源改成真实值。映射本地端固定为 `/media`。
3. 在本地 `secrets` 目录建立三个凭据文件。不要把文件内容复制到 Compose、shell 历史、截图或问题报告。
4. 在私有 Compose 文件中填入 `APP_ADMIN_USERNAME` 和 `APP_ADMIN_PASSWORD`。公开示例必须保留空占位符。
5. 保持 `features.write_enabled=false`、`features.remote_search_enabled=false`，并保持 `/media` 为只读挂载。

首次安装不合并 `compose.write.example.yaml`、`compose.d2-canary.example.yaml` 或 `compose.d3-canary.example.yaml`。它们分别代表受控写入或 Canary 边界，不能作为常规安装选项。

## 选择镜像

开发验证可使用带 `build` 的 Compose 示例。面向用户的安装应使用 release Compose，并把 `IMAGE_REF` 设置为完整的不可变镜像摘要，例如 `<registry>/<repository>@sha256:<digest>`。不要使用 `latest`、无摘要的浮动标签或本机历史镜像名称。

在启动前运行：

```text
docker compose -f compose.yaml config
```

命令必须成功，且输出中应保持只读媒体挂载、只读 rootfs、非 root 用户和 loopback 端口绑定。若配置合成失败、凭据文件不可读或路径映射不确定，应停止而不是尝试启动。

## 启动与首次检查

启动后依次验证：

1. 容器健康检查为 healthy。
2. `/livez` 返回存活，`/readyz` 只在 Emby 可达时返回就绪。
3. 未认证的 `/v1/health` 被拒绝；管理员登录后健康摘要显示远程搜索和写入均关闭。
4. UI 可只读浏览媒体和字幕清单，且页面、浏览器存储和 URL 中没有凭据、路径或 Token。

首次检查只证明默认 closed 安装可用，不证明 Provider、写入、媒体目录权限、字幕流或客户端播放已验收。相关能力必须按[风险分级验收矩阵](acceptance-matrix.md)另行授权。

## 下一步

- 变更镜像或配置前阅读[升级与回滚](upgrade-rollback.md)。
- 出现启动、登录、路径映射或功能开关问题时阅读[故障排查](troubleshooting.md)。
- 管理员会话和 Bearer 的安全职责见[ADR-006](adr/006-admin-session-and-automation-credentials.md)。
