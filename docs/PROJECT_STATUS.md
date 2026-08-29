# SubSteward 项目状态

> 当前状态：暂时搁置
>
> 状态日期：2026-08-29

## 当前结论

SubSteward 暂时停止继续开发、部署和自动化运行。当前代码、测试、文档和历史验证记录保留在仓库中，便于以后恢复；目标服务器上的 SubSteward 插件及部署备份已经清理，媒体文件和字幕文件不在本次清理范围内。

## 已完成内容

- M0：Emby Plugin 加载、数据目录、Item/MediaSource/MediaStream 读取和公开字幕 API 验证。
- M1：人工 Search、Fetch、Preview、Validate、固定偏移对轴、版本化 sidecar、Refresh 和 MediaStream 对账闭环。
- M2：字幕 Health、目标/第二语言检测、双语和特效分析、Preference 排序及 Action 建议。
- M3：定时任务、媒体库白名单、dry-run、单 Source 和安全写入门禁、STRM 标题/年份/集数对应、有限 Fetch 以及候选字幕之间的时间轴共识判断。
- C92：动画电影和华语电影库的历史 dry-run；“妖猫传”历史单样本安装验证；候选互相对照版本在“外语电影”库检测到候选时间漂移并转人工，未写入媒体。

## 暂停时的安全状态

- M3 默认关闭，dry-run 默认开启，媒体库白名单为空。
- C92 上没有保留 SubSteward 插件 DLL，也没有保留该插件的部署备份。
- C92 上的媒体目录没有因本次清理而删除或修改。
- `LOCAL_OPERATIONS.md` 仅保留在本机，不提交到仓库。

## 恢复前需要重新确认

1. 重新阅读本文件、产品说明、架构说明和 M3 文档，确认产品边界没有变化。
2. 重新构建并运行测试，不能直接使用历史 DLL 或历史部署结论。
3. 重新获得目标 Emby 实例的部署、重启、Provider 请求和媒体写入授权。
4. 重新验证 STRM 的候选对应和候选间时间轴共识；当前没有稳定共识候选的正式安装证据。
5. 重新完成客户端播放验收，再决定是否恢复 UI 收口或批量自动化。

## 相关文档

- [产品说明](PRODUCT.md)
- [架构与验证状态](ARCHITECTURE.md)
- [M3 自动补缺方案与进度](M3_AUTOMATION.md)
- [可复用经验](SUBBRIDGE_LESSONS.md)
