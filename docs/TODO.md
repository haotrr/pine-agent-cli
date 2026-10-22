# 初版交付记录

状态：当前开发范围已完成；最新最小在线闭环已验证，主标题与完整费用边界见 online-verification.md。

授权：用户“你直接开发吧”；独立工作区，不修改 Nobis。不授权创建持久凭证、登录 Pine、启动计费 Computer、真实账号写入、发布部署。

- [x] 检查工作区：空目录；祖先未发现 AGENTS.md，本地无 .agents/skills。
- [x] 核验公开 SDK 与真实类型；固定 v0.3.19。
- [x] 命令、真实 reconnect adapter、持久化 mock、提交前幂等日志。
- [x] 产物校验、回收后停止、状态锁及 runtime 保护。
- [x] 中文说明、无秘密配置和仓库内 Skill。
- [x] 测试与构建（详见 verification.md）。
- [ ] 后续授权：可信宿主提供运行中 binding、凭证注入和真实端到端验收。
- [x] 真实 StartComputer、AES-256-GCM 可插拔 Vault、计费门槛及未知启动恢复保护。
- [ ] 后续版本：Keychain 提供方、凭证轮换；更丰富的在线错误分类；金额预算与任务清理调度。

## 首次创建补充（2026-10-22）

授权：补齐首次创建安全流程、离线测试、文档和 Skill；不配置真实凭证或启动计费资源。

- [x] computer create，独立身份持久化/计费门槛。
- [x] 完整身份先加密落盘、pending 防重试、create/start 类型隔离及 reconcile。
- [x] 离线故障测试、race、vet、构建；见 verification.md。
- [x] 更新 README、开发文档、SDK 合约及 pine-delegate Skill。
- [ ] 真实账号 API 权限和最小在线闭环：仍待单独授权。

## 首次在线验收（2026-10-22）

- [x] 获明确授权，Keychain 保存本次宿主包装密钥，CLI 加密保存新身份。
- [x] 创建、会话、Run/Wait、空产物回收及停止；已停止资源。
- [x] 记录 Agent 实际计价和 failed 行为：[在线记录](online-verification.md)。
- [ ] 只读业务成功与非空产物下载：未通过；首次原因已据 Pine 回复更正为 token 预算过低。
- [ ] Computer/egress 完整精确账单仍待核实；首次原因与预算说明见 online-verification.md。


## 接入修复（2026-10-22，离线）

授权：修复三项接入缺口、SDK 适配测试、文档和 Skill，验证后 commit & push；不重启资源、不重跑在线任务。

- [x] 严格确认 Stop 布尔返回值，未知结果不自动重试。
- [x] 回收 turn workdir 引用，保留相对目录和完整性校验。
- [x] 保存 terminal_reason、安全 API 错误标识，新增逐页事件元数据回收。
- [x] 更新文档与 Skill，补 SDK 签名替身的离线回归测试。
- [x] race、vet、构建及 Skill 校验；验证边界见 verification.md。


## 默认预算更正（2026-10-22）

- [x] 根据用户转述的 Pine 回复更正首次失败原因；不外推第二次根因。
- [x] max-tokens 默认 0、允许 0、拒绝负数，保留正数覆盖和 wall 校验。
- [x] 补默认/显式零值、正数、负数、时间限制、幂等摘要及 SDK 序列化离线测试。
- [x] 更新 README、开发文档、在线记录与 Skill；本轮不重启或重跑在线任务。


## 零 token 上限在线验收与文档同步（2026-10-22）

- [x] 另获授权，新身份、max-tokens=0/max-wall=60s、单次只读任务 completed。
- [x] 非空文本产物回收、事件读取、停止成功并独立确认 runtime 不存在。
- [x] 同步 README、在线/离线验证文档和 Skill；早期失败记录保留为历史。
- [ ] 主标题未发现的判断：没有 DOM/截图独立核实。
- [ ] Computer/egress 精确金额与本次完整费用仍未取得。

本次文档同步不启动资源、不重跑在线任务。
