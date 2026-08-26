# ADR 索引与规则

ADR 用于记录跨模块、长期有效且代码本身无法完整解释的选择。局部实现细节和一次性排错写入测试、代码注释或 [维护经验](../../reference/lessons-learned.md)。

## 当前 ADR

| 编号 | 状态 | 决策 |
|---|---|---|
| [ADR-001](001-v1-uses-emby-remote-subtitle-bridge.md) | accepted | V1 使用 Emby Remote Subtitle Bridge |
| [ADR-002](002-project-codebase-route.md) | accepted | 方案 B：新建轻量 Go 后端，选择性复用 ChineseSubFinder |
| [ADR-003](003-phase2-milestones-and-deployment.md) | accepted | D1 只读 Canary → D2 搜索预览 → D3 专用样本 Add；固定安全部署默认值 |
| [ADR-004](004-item-and-source-path-separation.md) | accepted | STRM/Inventory 使用 Item.Path；MediaSource.Path 仅作为受控播放定位事实 |
| [ADR-005](005-conditional-d2-entry-without-live-multisource.md) | accepted | 缺少真实多源样本时有条件进入单源 D2；多源搜索保持独立门禁和安全拒绝 |
| [ADR-006](006-admin-session-and-automation-credentials.md) | accepted | 发布版管理员会话与自动化 API 凭据分离；D2.5 与 D3 Add 认证门禁已实现，真实 D3 Canary 已验收 |
| [ADR-007](007-subbridge-brand-and-legacy-deployment-identifiers.md) | accepted | SubBridge 新仓库/module/安装标识与 C92 旧部署标识的兼容边界 |
| [ADR-008](008-core-ab-daily-source-bound-recovery.md) | accepted | Core A/B 日常模式、显式 source 绑定与可恢复字幕操作 |
| [ADR-009](009-strm-write-target-and-multisource-boundary.md) | accepted | STRM 写入锚点、多源共享 sidecar 只读边界与旧 history 恢复策略 |
| [ADR-010](010-risk-based-acceptance.md) | accepted | 风险分级验收、真实证据复用与失效条件 |

## 创建条件

公共 API、数据格式、并发模型、安全边界、Provider 路线或兼容性承诺发生长期变化时创建 ADR。

文件名使用三位编号和英文短标题。状态使用 `proposed`、`accepted`、`superseded` 或 `rejected`。已使用的编号不重复。决策被替代时保留原文件，修改状态并链接新 ADR。

## 最小模板

```markdown
# ADR-NNN　标题

- 状态　proposed
- 日期　YYYY-MM-DD
- 相关组件　...

## 背景
## 问题
## 可选方案
## 最终选择
## 选择原因
## 已知代价
## 后续影响
## 验证依据
```

ADR 不得包含服务器地址、明文凭据、Token、候选 ID 或个人绝对路径。
