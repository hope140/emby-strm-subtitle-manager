# Knowledge Review 模板

实质性代码修改、Bug、架构或兼容性任务结束时复制下面模板，填入任务报告。没有新增内容时保留“无”，并写明检查范围。

```text
Knowledge Review
任务或阶段
验证范围

Knowledge Findings
- 新增约束　无
- 隐蔽坑　无
- 被证明错误的假设　无
- 建议沉淀项　无

证据
- 代码
- 测试
- 实际运行、日志或可复现结果

去重检查
- 已搜索的文档和关键词
- 是否更新已有结论　否

分流判断
- docs/lessons-learned.md　不需要 / 新增 / 更新
- docs/architecture.md　不需要 / 更新
- docs/adr/　不需要 / 新增或更新 ADR-NNN
- LOCAL_OPERATIONS.md　不需要 / 更新本机长期信息

未验证范围与残余风险
```

## 使用规则

1. 先搜索 `AGENTS.md`、`docs/architecture.md`、`docs/lessons-learned.md` 和 `docs/adr/`。
2. 只有代码、测试、日志、可复现运行结果，或官方文档与当前实现交叉确认的内容可以进入正式知识文档。
3. 服务器拓扑、连接方法和本机路径只进入 `LOCAL_OPERATIONS.md`。
4. 明文凭据、Token、Cookie、候选 ID 和带认证参数的 URL 不进入任何文档。
5. 任务报告可以保存完整过程，正式知识文档只吸收去重后的长期结论。
6. 主任务审核证据和分流结果，Knowledge Findings 本身不构成验收。
