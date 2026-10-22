# 在线验证摘要

2026-10-22 使用 Go SDK v0.3.19 完成一次获授权的在线验证。

- 创建身份、Computer、会话及 Run/Wait 调用成功。
- 只读目标为访问 example.com 并生成文本产物。任务返回 failed，Agent 报告环境在访问前不可用。
- 产物回收返回 0 个文件；随后停止调用成功。停止后只读查询确认旧 runtime 不存在。
- 未重新启动、恢复或重跑任务，未进行外部账号写入或发布。
- 生命周期调用已验证；网页访问成功和非空产物下载尚未验收。

## 预算与费用边界

Run 设置 max_tokens=2000、max_wall_ms=60000；返回 total_tokens=33696，其中 cache_write_tokens=33138、input_tokens=6、output_tokens=552，运行时间 17496ms。

官方 Run 文档按 usage.llm.total_tokens 计算限额，并允许最终报告等内部调用使用独立额度。本次较大差额仍待供应商解释，不视为总 token 硬上限已验收。AnswerOptions 没有独立预算参数；不能据此承诺续答硬预算。

Agent 返回计价 USD 0.01073772，compute=null。该值不是完整账单；单次 Computer 时间和流量费用仍待供应商核实。

## 根因与支持

停止后的历史自动诊断仅确认 runtime_gone，证据不完整，尚不能确定原始失败原因。已向 Pine 支持提交失败时间线、token 限额差异及完整费用请求，明确禁止重启资源或重跑任务。

完整原始报告、账号和资源标识、诊断及反馈编号、截图均仅保存在本地 reports/，不纳入公开仓库。SDK 文档见 [Computer API](https://console.pinecomputer.io/spec/computer) 和 [计费说明](https://console.pinecomputer.io/docs/overview)。
