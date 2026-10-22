# 在线验证摘要

## 最新：零 token 上限的最小闭环

2026-10-22 北京时间 19:18:36–19:21:01，客户端提交 `0a96358`、Go SDK v0.3.19，获授权安全保存新身份并执行一次只读任务。身份以 AES-256-GCM 加密保存，包装密钥由联调宿主存入 Keychain；CLI 本身没有内置 Keychain provider。

| 项目 | 实际结果 |
|---|---|
| 约束 | max-tokens=0（SDK 省略 token 上限）、max-wall=60s、idle-timeout=5m |
| 生命周期 | 创建 Computer、创建 session、Run/Wait、回收产物、Stop 均成功 |
| 任务终态 | state=completed，terminal_reason=completed |
| Agent 耗时 | 29.382 秒，低于请求的 60 秒 |
| 产物 | readonly-check.txt，259 字节；注册 artifact 与 workdir 引用各回收一份，内容和 SHA256 一致；saved=2 不代表两份不同业务产物 |
| 事件 | 21 条非帧元数据，NextAfter=LatestEventID=44 |
| 停止确认 | SDK Stop 返回 true,nil；停止后只读 SandboxDetails 返回 NotFoundError |
| 错误 | 本次 CLI 调用均成功，无 APIError 或 stop_unconfirmed |

Agent 与文本产物报告标题为 “Example Domain”，并称未发现 h1-h6 元素。**没有保存 DOM 或截图，主标题是否确实不存在尚未独立核实。** 本次验证的是生命周期、任务执行与非空产物回收，不能将 completed 等同于所有业务字段正确。未执行登录、外部账号写入或发布，也未追加任务、answer、恢复或重启。

### 本次用量与费用

- Agent cost.total / cost.llm：**USD 0.011489736**；cost.compute=null。
- total_tokens=118888：input=21、output=2522、cache_read=98009、cache_write=18336。
- max-tokens=0 表示没有设置该 token 上限，该总量不是超限证据；本次也没有验证时间限制触发或总金额硬上限。
- **Computer/egress 精确金额及完整费用尚未取得**；compute=null 不等于免费。Console 当时项目累计 $0.46，并提示最近用量仍在入账；项目汇总包含其他活动，不能归为本次费用，也不能用运行秒数估算冒充账单。

完整证据仅保留在私有报告与状态目录，公开文档不包含账号、资源标识、密钥或本地私有路径。本次文档同步复用既有证据，没有再次调用在线接口或启动资源。

## 历史：首次低预算失败

2026-10-22 使用 Go SDK v0.3.19 完成首次获授权的在线验证。

- 创建身份、Computer、会话及 Run/Wait 调用成功。
- 只读目标为访问 example.com 并生成文本产物。任务返回 failed，Agent 当时报告环境在访问前不可用；这一原因已被后续 Pine 回复更正，不能继续作为环境故障证据。
- 产物回收返回 0 个文件；随后停止调用成功。停止后只读查询确认旧 runtime 不存在。
- 未重新启动、恢复或重跑任务，未进行外部账号写入或发布。
- 生命周期调用已验证；网页访问成功和非空产物下载尚未验收。

### 首次预算与费用边界

Run 设置 max_tokens=2000、max_wall_ms=60000；返回 total_tokens=33696，其中 cache_write_tokens=33138、input_tokens=6、output_tokens=552，运行时间 17496ms。

官方 Run 文档按 usage.llm.total_tokens 计算限额，并允许最终报告等内部调用使用独立额度。据用户在 2026-10-22 转述的 Pine 回复，单次调用基础指令约需 3.3 万 token，2000 的预算太低，任务在打开页面前被拦下。Pine 建议 max_tokens=0 或省略该字段，时间限制可保留；将改进文档与限额提示。CLI 现默认 0，SDK 序列化时省略该字段。此回复解释了首次失败原因，但不是每笔 token 计量的完整审计，也不保证未来基础开销固定。AnswerOptions 没有独立预算参数；不能据此承诺续答硬预算。

Agent 返回计价 USD 0.01073772，compute=null。该值不是完整账单；Pine 回复 AI 约 $0.01、Computer 大约运行 1 分钟；仍无 Computer/流量精确金额与完整总账单，不把运行时长当作费用。

### 首次根因与支持

停止后的历史自动诊断仅确认 runtime_gone，证据不完整。后续用户转述 Pine 的历史排查结论：首次 readonly-check 因 max_tokens=2000 太低而失败，并非环境损坏，被拦下前没有打开页面。Pine 表示只检查了历史，没有重启或重跑资源。此处注明消息来源，未将用户转述伪装成新的 Console/API 核验。

第二次在线测试的环境不可用摘要仍只是 Agent 自述；本次转述未明确确认第二次是否同因，不直接外推。前两次测试均未通过业务成功与非空产物下载验收。默认值更正当轮仅做离线开发；随后用户另行授权的第三次测试结果见上文，不能用第三次成功倒推第二次失败根因。

完整原始报告、账号和资源标识、诊断及反馈编号、截图均仅保存在本地 reports/，不纳入公开仓库。SDK 文档见 [Computer API](https://console.pinecomputer.io/spec/computer) 和 [计费说明](https://console.pinecomputer.io/docs/overview)。
