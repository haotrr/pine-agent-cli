# 在线验证摘要

2026-10-22 使用 Go SDK v0.3.19 完成一次获授权的在线验证。

- 创建身份、Computer、会话及 Run/Wait 调用成功。
- 只读目标为访问 example.com 并生成文本产物。任务返回 failed，Agent 当时报告环境在访问前不可用；这一原因已被后续 Pine 回复更正，不能继续作为环境故障证据。
- 产物回收返回 0 个文件；随后停止调用成功。停止后只读查询确认旧 runtime 不存在。
- 未重新启动、恢复或重跑任务，未进行外部账号写入或发布。
- 生命周期调用已验证；网页访问成功和非空产物下载尚未验收。

## 预算与费用边界

Run 设置 max_tokens=2000、max_wall_ms=60000；返回 total_tokens=33696，其中 cache_write_tokens=33138、input_tokens=6、output_tokens=552，运行时间 17496ms。

官方 Run 文档按 usage.llm.total_tokens 计算限额，并允许最终报告等内部调用使用独立额度。据用户在 2026-10-22 转述的 Pine 回复，单次调用基础指令约需 3.3 万 token，2000 的预算太低，任务在打开页面前被拦下。Pine 建议 max_tokens=0 或省略该字段，时间限制可保留；将改进文档与限额提示。CLI 现默认 0，SDK 序列化时省略该字段。此回复解释了首次失败原因，但不是每笔 token 计量的完整审计，也不保证未来基础开销固定。AnswerOptions 没有独立预算参数；不能据此承诺续答硬预算。

Agent 返回计价 USD 0.01073772，compute=null。该值不是完整账单；Pine 回复 AI 约 $0.01、Computer 大约运行 1 分钟；仍无 Computer/流量精确金额与完整总账单，不把运行时长当作费用。

## 根因与支持

停止后的历史自动诊断仅确认 runtime_gone，证据不完整。后续用户转述 Pine 的历史排查结论：首次 readonly-check 因 max_tokens=2000 太低而失败，并非环境损坏，被拦下前没有打开页面。Pine 表示只检查了历史，没有重启或重跑资源。此处注明消息来源，未将用户转述伪装成新的 Console/API 核验。

第二次在线测试的环境不可用摘要仍只是 Agent 自述；本次转述未明确确认第二次是否同因，不直接外推。两次业务成功与非空产物下载均未验收。本次仅调整 CLI 默认值、测试、文档与 Skill，没有按建议重跑任务。

完整原始报告、账号和资源标识、诊断及反馈编号、截图均仅保存在本地 reports/，不纳入公开仓库。SDK 文档见 [Computer API](https://console.pinecomputer.io/spec/computer) 和 [计费说明](https://console.pinecomputer.io/docs/overview)。
