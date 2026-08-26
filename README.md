# SubBridge（SB，字幕桥）

SubBridge 为 Emby 与 STRM 媒体库提供字幕浏览、搜索、预览和受控管理能力。Emby 继续负责媒体索引与播放；SubBridge 不建立第二套媒体库，也不读取 STRM 内部的远程播放地址。

## 适合谁使用

如果你希望在保留 Emby 现有媒体库和播放链的前提下，查看与管理字幕，SubBridge 可以作为独立服务部署。标准安装默认启用搜索和日常字幕管理；管理员仍需登录，所有操作仍由服务端重新校验 Item、source 与路径。

## 已支持的能力

- 浏览 Emby 中的电影、剧集、季、集、版本和现有字幕。
- 搜索、获取和预览字幕候选。
- 对已验收的单源 STRM 场景执行 Add、Replace、Upload、可恢复 Delete 与 Restore。
- 管理员网页登录使用短期 HttpOnly 会话；自动化使用独立 Bearer Token。

普通本地媒体的真实写入验收、批量任务和自动下载仍未开放。多版本 STRM 必须明确选择版本；SubBridge 以 Item.Path 作为本地写入锚点，并以选中 source 绑定搜索、验证和历史，绝不从远程媒体 URL 推导本地路径。

## 快速开始

1. 阅读[安装指南](docs/guides/install.md)，选择 bridge 或 host-network Compose 拓扑。
2. 准备独立的 Emby API Key、应用 identity key、自动化 Bearer Token，以及私有 Compose 中的管理员用户名和密码。
3. 使用默认启用的搜索和写入配置，完成健康检查、搜索、预览与一个可恢复写入流程的验证。
4. 更新镜像或配置前阅读[升级与回滚](docs/guides/upgrade-rollback.md)；遇到问题使用[故障排查](docs/guides/troubleshooting.md)。

不要把密码、Token、Cookie、媒体绝对路径或带认证参数的 URL 写入配置仓库、浏览器地址栏、日志或问题单。

## 文档

- [用户指南](docs/guides/install.md)：安装、升级回滚、排障和公网反代。
- [当前状态与路线图](docs/planning/current-status-and-roadmap.md)：已完成范围、独立门禁和后续优先级。
- [开发文档索引](docs/index.md)：架构、长期决策、计划、验收与历史评审资料。

## 项目边界

- 不管理 115、CD2 Cookie 或直链。
- 不实现 Native Thunder 或 Native ASSRT Provider。
- 未经明确授权，不部署、不重启 Emby，也不发布外部版本。
