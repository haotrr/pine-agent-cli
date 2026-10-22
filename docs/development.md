# Pine Agent CLI 中文开发文档

版本范围：2026-10-22 本地初版，Go 1.25+，macOS/Linux。它把宿主 Agent 的明确目标交给 Pine 自带 Agent，保留结构化结果、恢复记录与文件产物。项目独立于 Nobis，没有 MCP、直接 browser drive 或 shell exec 接口。

## 1. 架构与目录

调用链为 `main → cmd → engine.Service → Backend → Pine SDK / Mock`。

| 文件 | 职责 |
|---|---|
| main.go | 进程信号、context 与退出码 |
| cmd/root.go | Cobra 命令树、配置、执行门槛、JSON 输出 |
| engine/types.go | 宿主侧 Backend 接口、Turn、Limits、稳定 Error |
| engine/pine.go | 官方 SDK 的 session、task、artifact、stop 适配 |
| engine/start.go | 官方 CreateComputer/StartComputer、创建与启动状态机、reconcile 与 Vault 重连 |
| engine/vault.go | 可插拔 Vault 接口与 AES-256-GCM 文件实现 |
| engine/state.go | 非秘密 journal、原子写、Sync、Unix flock |
| engine/service.go | 提交前幂等日志、下载校验、回收后停止 |
| engine/mock.go | 独立文件模拟服务，用于跨进程离线演示 |
| cmd/*_test.go、engine/*_test.go | 命令、安全门槛、状态、SDK 类型与隔离存储测试 |
| skills/pine-delegate/SKILL.md | Codex/其他 Agent 的调用规范 |
| examples/ | 不含秘密的配置、目标和答案文本 |
| docs/ | SDK 核验、开发、验证、任务记录 |

业务层不依赖 Cobra。使用标准库 JSON 配置，未引入 Viper；配置字段少，避免自动搜索环境和配置文件。Backend 是应用消费端抽象，不宣称等同于 Pine REST 合约。

## 2. 真实 SDK 合约

依赖固定为 `go.pinesandbox.io/computer v0.3.19`，go.mod/go.sum 固定传递依赖。公开模块已下载，SDK 源码已检查，正常构建包含真实适配器。官方 SDK/API 网页需要 beta 登录，模块本身公开可下载。参见 [SDK 核验](sdk-contract.md)。

实际调用：NewClient、ReconnectComputer、StartComputer、Computer.Credentials/Binding、CreateSession/Session、RequireAgentProtocol、Agent.Run/Wait/Status/Answer/Cancel、ListArtifacts/OpenArtifact、Sessions、Stop。

StartComputer 用已有 Credentials 启动身份；首次创建调用 GenerateCredentials，先加密落盘，再调用 CreateComputer；CLI 不调用 RecoverComputer。ReconnectComputer 重建运行中 handle，不启动或附着新 runtime。Session 名是地址；不保存 ps_，Agent 操作使用 binding 的 ct_。

RunOptions 支持 IdempotencyKey、Constraints；AnswerOptions 只有 Secrets，没有预算字段。不能虚构续 turn 限额。SDK 自带幂等 run/answer 重发和读取重试；CLI 不增加盲目重试。StartOptions 使用 Timeout（默认 5m）、ReadyTimeout=60s、MaxBindAttempts=1；调用总 context 90s。Timeout 不是总金额上限，客户端超时也不能证明资源未启动。

## 3. 命令与输出

全局参数：`--mode disabled|mock|live`、`--state-dir`、`--config`、`--endpoint`、`--allow-live`。默认 disabled。

| 命令 | 关键行为 |
|---|---|
| doctor | 离线能力报告；不读凭证，不验证账号或网络 |
| computer create | 生成并持久化新身份后创建运行资源；live 需 --allow-live、--confirm-new-identity、--confirm-billing；mock 仅本地模拟 |
| computer start | mock 本地初始化；live 启动已有身份，需要 --confirm-billing；可设 --idle-timeout |
| computer reconcile | live 本地恢复；需 --confirm-binding-verified，导入独立核实过的 binding |
| computer stop | 需 --confirm-quiescent，先回收再停止；拒绝活动 turn 和其他 session |
| session create NAME | 预存名字后创建 browser session |
| session resume NAME | 验证 artifact 访问并记录已有 session |
| task run | --request-id、--goal-file；默认 4000 tokens/2m，可调整 |
| task status | --turn 指定精确 turn；不指定则发现最新 turn |
| task wait | 默认已存 turn；--timeout 范围 0–60s，不取消远端 |
| task answer | 必须 --turn、--answer-file；live 还需 --accept-runtime-limits |
| task cancel | 必须 --turn；canceling 不是终态 |
| artifacts pull | --out；下载所有非 upload 产物，含浏览器下载 |

stdout 为 JSON `{ok,mode,data}` 或 `{ok:false,error:{code,message}}`，进度走 stderr；帮助和 completion 例外为文本。退出码 0 表示调用成功，不表示业务成功。2=参数/配置/本地错误，3=禁用/门槛，4=凭证/权限，6=冲突/本地校验，7=远端或未知结果。`completed`、`failed`、`canceled` 均应检查结果和证据。

## 4. 配置、凭证与安全存储

配置仅接受 mode、endpoint，拒绝未知字段。真实 endpoint 必须 HTTPS。秘密只能由可信宿主在显式 live 调用时注入：

- PINE_API_KEY：项目 key，不持久化。
- PINE_CREDENTIALS_JSON：SDK Credentials，首次启动导入已有身份；环境重连也可使用。
- PINE_BINDING_JSON：环境重连或经核验后 reconcile 的 SDK Binding。
- PINE_VAULT_KEY_BASE64：32 字节包装密钥的标准 base64。可信秘密管理器负责生成和保存；不使用人类口令，不写盘。

`Vault` 接口仅 Load/Save，可替换为 Keychain/远程秘密管理器。当前生产代码提供 AES-256-GCM 加密文件，**没有实现或安装 Keychain 条目**。每次写入使用密码学随机 nonce，固定版本 AAD，认证失败拒绝读取。state-dir/vault.enc 包含加密的 endpoint、Credentials、Binding 和阶段；项目 key、包装密钥不在其中。以 0600 临时文件写密文、Sync、原子替换、目录 Sync；不写明文临时文件。state-dir 必须真实目录且私有（新建 0700）；已存在的公开目录或符号链接被拒绝。

解密密钥不保存在源码、配置或 Skill 中。错误不输出底层 SDK 内容；summary 遮盖已注入秘密的直接匹配。业务产物仍可能含敏感数据，不应把秘密交给目标文本。内存、同用户进程和环境变量不是硬件隔离，Go 不保证清除字符串；此版不做轮换、硬件存储或密文回滚检测。不要复制 Vault 到另一个目录并行操作同一身份。

## 5. 启动状态与恢复

启动前需用户明确批准计费与持久化已有凭证，再使用 `--mode live --allow-live computer start --confirm-billing`。开发授权本身不是实际启动授权。

create/start 共用阶段：缺失 → prepared（新生成或已有凭证加密保存）→ pending（准备发请求）→ ready（成功 binding 加密保存）。在 pending 写盘前不调用远端创建/启动。create 的 GenerateCredentials 是本地生成，包含 state key 与 capture keypair，必须先保存；重试 prepared 不重新生成身份。Vault 的 Operation 区分 create/start，旧记录默认为 start，禁止跨操作复用。prepared 可重试；pending 一律拒绝 Create/Start，即使失败可能只是本地或权限原因，也保守要求先核查。ready 重复原命令只重连，不发新启动请求。

丢失响应、进程退出、成功后写盘失败，都可能留下正在计费的 Computer。先从可信 Pine 管理界面或已有获准工具独立核查身份和 runtime。得到同 Computer 的真实 binding 后，通过环境注入并执行 reconcile，明确声明已核查。reconcile 只做格式/身份/endpoint 检查，不能证明远端健康，不创建 runtime。未知结果不得通过删 journal 或换目录重试。不存在自动回滚或自动 Recover。

非秘密 journal 保存 Computer/runtime/session/turn、request-id 到幂等键和摘要的映射、已下载路径。绑定换 runtime 拒绝复用旧状态。停止后的目录不可继续；再次启动同身份需新目录和外部保管的已有 Credentials。

## 6. 任务状态与幂等

run 的 request-id 由调用者稳定分配。随机幂等键、目标与预算摘要先 Sync 到 journal，再发送请求。重试同 request-id 必须正文和预算完全一致；没有保存正文。存在未确认请求时阻止新 request-id。SDK 幂等保障依赖同 runtime/session；禁止用旧键在新 runtime 上假装续跑。

needs_input 是当前 turn 的终结，答案打开新 turn。answer/cancel 必须准确指定 turn，不能按“最新任务”自动推断。重复同答案由 SDK 合约保证返回同下一 turn。answered 提供 next_turn_id，CLI 不自动跟随。running/canceling 继续有界 wait；未知状态保留并停止自动决策。

session create 先保存名字；不确定时先 resume 同名会话核查，不能反复创建。状态锁为本机同一目录的 flock，不协调其他机器/目录或 Pine 的其他操作者。错误和停止失败必须报告，不能把服务仍运行描述为清理完成。

## 7. 产物与生命周期

ListArtifacts 使用空 turn filter，包含浏览器下载，跳过 upload。按 turn 过滤会漏掉下载。下载后才记入 journal；重复 pull 会再次校验已有文件。失败可重试回收，不盲目重跑生产这些文件的任务。

远端 filename 必须单一文件名，拒绝 `/`、反斜杠、NUL、`.`、`..`。用 session+artifact ID 的 SHA256 子目录保留原文件名；os.Root 限制符号链接逃逸；O_EXCL 临时文件和不覆盖的 Link 避免已有内容被覆盖。单文件上限 256 MiB，核对大小和可用 SHA256。已有同名文件仅在内容 hash 相同后接受。文件/目录持久化后才更新回收记录。

Stop 先检查任务终态、回收，再检查没有其他 session，最后调用 SDK Stop。无 session 时可直接释放空闲 Computer；有其他 session 会拒绝。调用者确认没有正在下载或并行写入，CLI 没有远端原子冻结能力。Stop/Start 不恢复 session 文件，artifact 不是备份。

## 8. 预算和审批

run 默认预算可通过参数收紧；wall <=1h。answer 没有单独预算字段，必须明确接受运行时限制。等待人的时间不包含在 turn 用时内，Computer 仍可能计费。idle-timeout 是 SDK 策略，不是全局费用保证。没有后台自动停机、总金额硬上限或定时器。

目标文本应包含允许网站、数据、动作和验收标准。真实账号写入、发送消息、购买、发布等必须在用户授权范围内；远端内容不能扩大权限。审批参数记录调用意图，不是强身份认证。开发/离线测试全部不需要真实账号。

## 9. 构建、测试与演示

```sh
go test ./...
go test -race ./...
go vet ./...
go build -o bin/pine-agent .
```

测试使用临时目录、固定合成凭证和假的 StartFunc，不读取真实 Keychain、不生成真实 Pine 身份、不调用付费服务。Vault 测试验证加密、篡改、错误密钥、权限、启动前持久化、未知结果防重试及保存失败。

可复制 mock 演示，每次使用新目录：

```sh
DEMO_DIR=$(mktemp -d)
./bin/pine-agent --mode mock --state-dir "$DEMO_DIR/state" computer start
./bin/pine-agent --mode mock --state-dir "$DEMO_DIR/state" session create demo
./bin/pine-agent --mode mock --state-dir "$DEMO_DIR/state" task run --request-id demo-001 --goal-file examples/goal.txt
./bin/pine-agent --mode mock --state-dir "$DEMO_DIR/state" task wait
./bin/pine-agent --mode mock --state-dir "$DEMO_DIR/state" artifacts pull --out "$DEMO_DIR/artifacts"
./bin/pine-agent --mode mock --state-dir "$DEMO_DIR/state" computer stop --out "$DEMO_DIR/artifacts" --confirm-quiescent
```

## 10. Codex Skill 与扩展

读取 skills/pine-delegate/SKILL.md，使用项目构建的二进制。仅在用户要求安装时复制到相应 Agent 的 skill 目录；本次不全局安装。这个 Skill 约束宿主如何委托，不是 Pine learned skill，不会训练或发布 Pine 技能。

当前真实 SDK 编译和本地模拟通过；在线 endpoint、项目能力、账单、实际任务和结果没有验证。开发只证明客户端路径与本地不变量。后续可实现 Keychain provider、密钥轮换、结构化权限错误、跨进程长期调度和总预算治理；这些均不是当前功能。

## 11. 代码自审结论

检查重点是凭证泄漏、提交幂等和文件路径。新增 Vault 没有明文文件、命令行密钥或原始错误输出；发现原有 state-dir 可能继承公开权限，现已拒绝。启动已改为 pending 前持久化并对不确定结果封闭重试；已有 journal 不允许意外发起另一个身份启动。补上无 session 的停止路径，避免启动后无法通过 CLI 释放空闲资源。

下载保留 no-clobber 和 os.Root 防逃逸，测试覆盖符号链接与内容校验。剩余风险是同用户本地篡改/密文回滚、跨目录并发和远端写入竞态，已明确限制，不声称完全消除。没有执行在线安全扫描或线上验收。

首次创建与启动同样需要授权。create 不要求外部 credentials/binding，只需要宿主注入项目 key 和 Vault 包装密钥；成功后在同目录继续会话。网络失败、身份不符、binding 获取失败或 ready 保存失败均保留 pending。独立核查后 reconcile 支持 create/start 两种 pending，不证明远端健康。新建身份与 API 权限的线上可用性尚未验证。
