# SubSteward 文档索引

本分支只保留 SubSteward 当前产品需要的公开文档。旧 SubBridge 的完整服务部署、Canary、评审和历史记录不复制到这里；需要回查时使用稳定基线 `3acdf27047338f81438fa611aed314533e170371` 或远端回退分支 `legacy/subbridge`。

## 先看哪一份

| 文档 | 适合回答的问题 |
| --- | --- |
| [README](../README.md) | 这个项目是什么、当前主线和核心安全边界是什么 |
| [产品说明](PRODUCT.md) | 产品做什么、不做什么、M0–M4 如何推进 |
| [架构与验证状态](ARCHITECTURE.md) | 当前 Plugin、管理员页面、API、写入链路和实测状态是什么 |
| [M3 自动补缺方案与进度](M3_AUTOMATION.md) | M3 自动化的门禁、配置、运行结果和实施顺序是什么 |
| [项目状态](PROJECT_STATUS.md) | 当前项目是否继续开发、服务器是否保留插件、以后如何恢复 |
| [可复用经验](SUBBRIDGE_LESSONS.md) | STRM、Provider、语言/质量、写入和验收有哪些不能忘的规则 |
| `LOCAL_OPERATIONS.md` | 当前机器的凭据入口、XTerminal/C92、路径、部署和回滚怎么做 |

`LOCAL_OPERATIONS.md` 只在本机存在且保持未跟踪，不把其中的连接细节或凭据复制到公开文档。

## 按问题查找

- 要确认产品边界，先看 [产品说明](PRODUCT.md) 的“明确不属于 SubSteward 主线”和“当前状态”。
- 要调用或检查管理员 API，看 [架构与验证状态](ARCHITECTURE.md) 的“管理员 API 索引”，实际认证和本机连接再看 `LOCAL_OPERATIONS.md`。
- 要处理 STRM 或怀疑写错媒体，看 [可复用经验](SUBBRIDGE_LESSONS.md) 的“Emby 媒体模型与 STRM”，再看本机手册的路径边界。
- 要处理错误 Provider 候选，看 [可复用经验](SUBBRIDGE_LESSONS.md) 的“Provider 候选与错误隔离”，不要直接下载搜索第一条。
- 要判断测试是否足以宣布通过，看 [可复用经验](SUBBRIDGE_LESSONS.md) 的“证据分层与状态写法”和本机手册的“证据等级索引”。
- 要部署、重启或回滚 C92，只按 `LOCAL_OPERATIONS.md` 执行，并先取得当前任务的明确授权。

## 当前文档口径

截至 2026-08-29，M1 人工 Search → Fetch → Preview → Install → Refresh 主链路和实际客户端验收已有历史实测；M2 核心分析、媒体库分页和嵌入式管理页面已经进入主线；M3 后端自动补缺已完成候选门禁和候选间时间轴共识的阶段性实现。项目当前暂时搁置，后续恢复必须以当前源码、测试和重新进行的目标 Emby 复验为准，不能仅凭历史部署记录宣布自动化已发布。

项目暂停前的最新 C92 记录是候选互相对照版 M3 的单条目 dry-run；“外语电影”库的《惊天魔盗团3》检测到候选间时间漂移并转人工，没有媒体写入。随后服务器插件及备份已清理。此前“妖猫传”已完成单样本自动 Install、Refresh 和 MediaStream 对账，UI8 浏览器视觉验收和客户端播放仍待完成。M3 的配置和门禁集中记录在 [M3 自动补缺方案与进度](M3_AUTOMATION.md)，总状态见 [项目状态](PROJECT_STATUS.md)，历史证据和未验证项集中记录在 [架构与验证状态](ARCHITECTURE.md) 和本机手册中。
