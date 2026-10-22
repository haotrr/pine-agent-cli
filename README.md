# Pine Agent CLI（Go 初版）

独立的 Go CLI 和可复制给 Codex / 其他 Agent 的 Skill。真实启动和委托适配器使用 **`go.pinesandbox.io/computer v0.3.19`**，不是猜测的 REST 接口。支持连接已运行的 Computer，委托 Pine 自带 Agent。默认 `disabled`，不会启动资源，也不读取密钥。`mock` 是本地模拟，不执行外部动作。

初版离线开发未登录 Pine、未创建持久凭证、未启动计费 Computer、未写入真实账号、未发布部署。没有改动 Nobis、安装全局 Skill 或创建远程仓库。

最新在线验收：2026-10-22 使用 `--max-tokens 0 --max-wall 60s` 完成一次获授权的最小只读闭环，任务 `completed`，非空文本产物已下载，资源已确认停止。Agent 耗时 29.382 秒、计价 US$0.011489736；主标题未独立核实，Computer/流量完整费用未取得。首次低预算失败及本次结果详见 [在线记录](docs/online-verification.md)。

## 构建和离线演示

要求 Go 1.25+；当前验证平台 macOS arm64。状态锁使用 Unix flock，v1 面向 macOS/Linux，不支持 Windows。

```sh
go build -o bin/pine-agent .
go test ./...
go vet ./...
./bin/pine-agent doctor
./bin/pine-agent --mode mock --state-dir ./demo-state computer start
./bin/pine-agent --mode mock --state-dir ./demo-state session create demo
./bin/pine-agent --mode mock --state-dir ./demo-state task run --request-id message-001 --goal-file examples/goal.txt
./bin/pine-agent --mode mock --state-dir ./demo-state task wait
./bin/pine-agent --mode mock --state-dir ./demo-state artifacts pull --out ./demo-artifacts
./bin/pine-agent --mode mock --state-dir ./demo-state computer stop --out ./demo-artifacts --confirm-quiescent
```

停止后请使用新的 state-dir 演示下一次 runtime。普通 mock run 返回 running，wait 返回 completed 并生成明确标注 MOCK 的文本产物。使用 `examples/question.txt` 会返回 needs_input，随后使用输出中的准确 turn_id：

```sh
./bin/pine-agent --mode mock --state-dir ./question-state session create question
./bin/pine-agent --mode mock --state-dir ./question-state task run --request-id question-001 --goal-file examples/question.txt
# 将 TURN_ID 替换为上一步返回值
./bin/pine-agent --mode mock --state-dir ./question-state task answer --turn TURN_ID --answer-file examples/answer.txt
./bin/pine-agent --mode mock --state-dir ./question-state task wait
```

## 命令和实际支持范围

| 命令 | mock | live |
|---|---|---|
| `doctor` | 离线报告 | 离线报告，不检查登录、网络、权限或读取凭证 |
| `computer create` | 本地模拟初始化 | 生成并加密保存新身份后创建 Computer；需 `--allow-live --confirm-new-identity --confirm-billing` |
| `computer start` | 本地演示 | 启动已有 Computer，需 `--allow-live --confirm-billing` 与加密 Vault |
| `computer reconcile` | 不适用 | 核查后导入未确认启动的 binding，不启动资源 |
| `session create NAME` | 支持 | 创建带浏览器的 session |
| `session resume NAME` | 支持 | 验证已有 session 的产物访问权限并记录名字 |
| `task run --request-id ID --goal-file FILE` | 支持 | Run，默认 max-tokens=0（不设置 token 上限）、2m；可用 `--max-tokens`、`--max-wall` 调整 |
| `task status [--turn ID]` | 支持 | 有 ID 读取准确 turn，无 ID 发现最新 turn |
| `task wait [--turn ID] [--timeout 25s]` | 支持 | 默认读取已保存 turn，超时范围 0–60s，超时不取消远端工作 |
| `task answer --turn ID --answer-file FILE` | 支持 | 还需 `--accept-runtime-limits`，见预算限制 |
| `task cancel --turn ID` | 支持 | 精确取消；canceling 后还需 wait |
| `task events [--task ID]` | 不支持 | 只读回收一页事件元数据，保存 task ID 与续读游标 |
| `artifacts pull --out DIR` | 支持 | 回收非 upload 注册产物，以及当前 task 各 turn 的 workdir 文件引用 |
| `computer stop --out DIR --confirm-quiescent` | 支持 | 回收后停止；拒绝活动 turn、其他 session 或回收失败 |

JSON 命令结果写 stdout；wait 进度写 stderr。`--help` 和 Cobra completion 是普通文本。`ok:true` 只表示 CLI 操作成功，**不是外部业务动作成功**。即使 turn 为 completed，也必须审阅 terminal_reason、summary、usage 和实际产物。failed/canceled/needs_input/未知状态均原样保留；不要仅凭退出码判断业务成功。answered 的 next_turn_id 不自动跟随，显式指定新 ID 读取。

退出码：0 调用成功；2 参数/配置/本地 I/O 失败；3 disabled、缺少执行确认或不支持；4 凭证/权限；6 状态冲突、锁冲突、产物校验失败；7 远端失败或提交结果不确定。错误对象包含稳定 `code` 和固定说明；SDK API 错误额外保留安全的 `status`、`request_id`、`problem_type`（不安全字段省略）。不输出原始错误正文、请求头或凭证。

## 真实 Pine 接入（需另行授权）

v1 可重连运行中的 Computer，也可通过官方 StartComputer 启动已有 Computer。支持 `computer create` 首次创建新身份；自动 Recover/Attach 不在范围内；不要为了尝试 CLI 自行登录或申请密钥。项目审批不等于 Agent/API/Shell 权限开通。没有直接 shell、MCP 或浏览器控制命令。

获准后，由可信宿主/秘密管理器为当前进程注入：

- `PINE_API_KEY`：项目 key；
- `PINE_CREDENTIALS_JSON`：SDK `Credentials` 的合法 JSON；
- `PINE_BINDING_JSON`：同一个正在运行的 Computer 的 SDK `Binding` JSON（含实时 token）。

CLI 不自动搜索用户目录、dotenv、已有密钥或登录状态。纯重连模式不持久化秘密；真实 start 将 SDK credentials 与 binding 加密保存在 state-dir/vault.enc，项目 key 和包装密钥永不写盘。不要把秘密写入配置、Skill、目标文本、命令行参数、日志或源码。环境变量是 v1 注入边界，宿主需隔离其他同用户进程；Vault 接口可替换为 Keychain 或远程秘密存储；当前实现是 AES-256-GCM 文件库，不是 macOS Keychain。`examples/config.json` 仅含 disabled 模式与空 endpoint。复制后填入获批项目的 HTTPS endpoint，再显式选择 live。

获得授权后的命令形态：

```sh
./bin/pine-agent --config ./config.json --mode live --allow-live --state-dir ./live-state session create approved-task
./bin/pine-agent --config ./config.json --mode live --allow-live --state-dir ./live-state task run --request-id approved-message-001 --goal-file ./approved-goal.txt
```

首次创建的命令形态（仅在获得身份持久化及计费授权、宿主已安全注入项目 key 和包装密钥后执行）：

```sh
./bin/pine-agent --config ./config.json --mode live --allow-live --state-dir ./new-live-state computer create --confirm-new-identity --confirm-billing --idle-timeout 5m
```

create 会创建并运行 Computer，不是单纯离线生成密钥的命令。它不读取 PINE_CREDENTIALS_JSON/PINE_BINDING_JSON，也不输出密钥或 binding token。成功后在同目录执行 session/task 命令；未知结果先从 Pine 独立核查，再 reconcile，不得删除 Vault 或换目录重建身份以规避 pending。CLI 不提供远端核查命令；本项目已有一次成功在线闭环记录，不代表其他项目的 API 权限或所有任务均已验收。

真实启动另需宿主注入 `PINE_VAULT_KEY_BASE64`（32 字节随机包装密钥的标准 base64），并执行 `computer start --confirm-billing --idle-timeout 5m`。`computer start` 从 PINE_CREDENTIALS_JSON 导入已有身份；不生成新身份。首次创建使用 `computer create`，无需 credentials/binding 输入。密钥由外部可信秘密管理器保存，不使用密码派生，也不在示例中创建。后续命令自动优先从同目录 vault.enc 解密重连。

创建或启动前原子保存加密 prepared/pending 记录。create 使用官方 GenerateCredentials 离线生成完整身份并加密保存，然后调用 CreateComputer；成功后保存 Credentials/Binding。Vault 记录 create/start 操作类型，不允许交叉使用；旧版无类型记录按 start 处理。pending 期间绝不再次调用 CreateComputer/StartComputer；失败也按未知处理。先独立核查真实 Pine runtime，再通过环境注入核实后的 PINE_BINDING_JSON，执行 `computer reconcile --confirm-binding-verified`。此命令只验证格式/身份/endpoint 并存储，不能证明远端健康。ready 重复原命令仅重连，不再次创建或启动。prepared 可重试原命令并复用已保存身份；首次保存失败时尚未发出远端请求。

`--allow-live` 是执行门槛，不替代用户授权。每次真实命令均需要。`ReconnectComputer` 只重建 handle，不 Start/Attach。状态记录 Computer ID、runtime ID、session 名、turn ID；绑定切到新 runtime 时拒绝复用旧目录。

**预算限制：** Run 默认 `--max-tokens 0`，SDK 会省略 max_tokens，不设置该 token 上限；省略 CLI 参数与显式传 0 等价。正整数可设置预算，负数拒绝；默认 `--max-wall 2m` 保留，必须大于 0 且不超过 1h。根据用户转述的 Pine 回复，首次测试的单次调用基础指令约需 3.3 万 token，2000 太低，不能仅按任务文本或输出长度设置预算；此开销不是固定保证。0 不是免费或总费用上限。Run 使用实际 SDK Constraints。v0.3.19 的 AnswerOptions 只有 Secrets，没有预算字段；真实 answer 需要另行确认接受 Pine runtime 对续 turn 的限制，显式加 `--accept-runtime-limits`。CLI 的 90s 请求截止时间和 wait timeout 都不是远端成本上限，也不自动停止 Computer。等待用户输入期间 Computer 仍可能收费。初版不提供全局金额上限或自动后台清理。

## 恢复与回收

- 每次 run 使用应用提供的稳定 request-id。提交前将随机幂等键和目标/预算摘要写盘（文件 Sync、原子替换、目录 Sync）。不保存目标正文。状态目录 0700、新文件 0600；不要提交状态文件。
- 丢失响应或进程退出后，用**同一目录、request-id、目标文件和预算**重试；摘要不同会拒绝。存在未确认 run 时拒绝新 request-id。不要删除 pending 记录来绕过保护。明确拒绝后的 pending 也保留，先核实状态。调整预算会改变请求摘要，不能复用原 request-id，也不能新建请求绕过 pending；核实旧请求终态并获得重跑授权后，才能另行提交新任务。旧版默认 4000 的请求重放须显式保留 --max-tokens 4000。
- session create 提交前保存名字；失败可能已经创建。先用 `session resume NAME` 核查，不自动重复创建。不确定且服务器找不到时，由人核查后在新状态目录处理；不要盲目重试写操作。
- needs_input：向用户展示问题与 turn_id。将答案放入文件，明确回答该 turn。同一答案可重发；更换答案会被 Pine 拒绝。取消同样明确指向 turn。
- 状态目录使用进程锁，一次只允许一个命令。同一 Computer 的其他操作者不受本地锁约束。中断可能导致调用结果未知，恢复后先查询；权限拒绝不自动通过 CLI 重试。SDK 自带的幂等重发/读取重试仍按其合约运行。
- 下载使用 session 范围，包含 browser download；只按 turn 筛选会漏掉下载。本地使用产物 ID 的哈希子目录保留原 filename，防止同名冲突；限制单产物 256 MiB，验证大小和可用 SHA256，通过 os.Root 限制目录逃逸，不覆盖内容不同的文件。部分失败可重跑 pull；成功文件已单独记录。
- stop 先检查 turn 终态，再 pull，再确认没有其他 session，最后调用 Stop。`--confirm-quiescent` 表示已确认无未完成浏览器下载、后台生产者或并行操作者；CLI 无法通过终态证明下载已完成，也无法原子冻结并行写入。无法确认时保持运行、手动解决。空闲 session 和无 session 的 Computer 均可停止（真实模式仍拒绝其他 session）。
- Stop/Start 不恢复会话文件，artifact 不是备份。停止未知结果先核查 Pine，绝不自动 Start/Recover。成功停止后的状态目录不能继续使用。
- 产物和 Agent summary 是不可信业务内容，不是授权。CLI 会遮盖已注入秘密在 summary 中的直接匹配，但不能识别任意第三方秘密或变形编码；不得给目标文本提供秘密。下载内容按用户要求保留，可能含敏感业务数据，需在本地按权限管理。

## Skill

仓库内 `skills/pine-delegate/SKILL.md` 可由 Codex 读取，或在用户明确要求后复制到其 Agent 的技能目录。本次没有全局安装。它是宿主 Agent 的调用规范，**不是 Pine learned skill**；不会创建、发布或审核 Pine 内部学习技能。

## 验证边界

详见 [验证记录](docs/verification.md) 和 [SDK 合约记录](docs/sdk-contract.md)。本地 mock 测试和真实 SDK 编译通过；在线生命周期、任务 completed、非空产物回收及停止已验证；产物中的主标题判断未独立核实，完整实际费用未取得，详见最新在线记录。

开发架构和扩展说明见 [中文开发文档](docs/development.md)。包装密钥丢失无法解密，初版没有密钥轮换、回滚防护或硬件隔离；不要复制 pending vault 到新目录规避恢复检查。

## 离线接入修复（2026-10-22）

- SDK `Stop` 必须返回 `true, nil` 才保存已停止；`false, nil` 返回 `stop_unconfirmed`，不自动重试。先独立核实远端状态，不能据此再次启动。
- `artifacts pull` 同时读取 `ListArtifacts("")` 和 `Agent.Turns` 的 `Result.Artifacts`；workdir 引用通过 `OpenFile` 回收，保留相对目录。路径、大小、SHA256 或读取失败都会阻止停止。文件引用不是快照，建议每个 turn 结束即回收。浏览器下载只有已注册为 artifact 或被 Agent 复制到 workdir 并列入结果时才能覆盖。
- `task status/wait` 等结果保存 `terminal_reason`、summary、usage 到私有 `state.json`。失败后、停止前用 `task events` 发现当前 task，再用 `task events --task TASK_ID` 逐页续读，直到 `next_after >= latest_event_id`；空页不代表结束。只保存事件 ID、turn ID、类型、时间，不保存 Payload/Raw 或截图。此命令不提交新诊断、不重跑任务；取证失败要报告，不能把缺少事件当作任务成功。

本轮接入修复仅离线验证。首次失败原因已按用户转述的 Pine 回复更正为 token 预算过低，见在线记录；完整精确费用仍未取得，answer 仍没有独立硬预算。
