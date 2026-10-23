# 验证结果

最新在线记录：2026-10-22 获授权使用 max-tokens=0、max-wall=60s 执行最小只读闭环，任务 completed、非空产物已回收、资源已确认停止；主标题未独立核实，完整费用未取得，详见 [在线记录](online-verification.md)。下文早期记录保留各轮当时的验证边界。

时间：2026-10-22；环境：Go 1.25.5，darwin/arm64。

## 已完成

- `go mod tidy`：成功取得并锁定真实 Pine SDK v0.3.19、Cobra 与传递依赖。使用临时模块缓存 `/tmp/pine-sdk-mod`，未读取用户密钥。
- `go test ./...`：通过。
- `go test -race ./...`：通过，初版 19 个测试函数（含子用例）。覆盖提交前持久化幂等键、丢失响应后同键恢复、不同目标/预算拒绝、pending 阻止新请求、needs_input/answered、重复答案、取消、停止前回收、锁、模式/runtime 隔离、文件路径/符号链接逃逸防护、checksum、不成功回收禁止停止、默认禁用及摘要脱敏。
- `go vet ./...`：通过。
- `go build -o bin/pine-agent .`：通过，真实 SDK 适配器是正常构建的一部分，无 mock-only build tag。
- 构建产物 smoke：独立进程依次执行 mock computer start、session create、task run/status/wait、artifacts pull、computer stop；JSON 解析成功，实际落地产物包含 MOCK 标记。
- Skill 官方 `quick_validate.py skills/pine-delegate`：`Skill is valid!`。初次因缺少 PyYAML 未运行，随后仅在 `/tmp/pine-skill-validation` 安装 PyYAML 6.0.3 并通过；未修改全局环境。

## 边界

上述工作流验证全部使用本地 mock。真实 SDK 的方法和类型已读取源码并编译验证，但没有真实服务响应、权限或端到端操作证据。未登录 Pine、未生成持久凭证、未启动计费 Computer、未读取已有秘密、未执行真实账号写入或发布部署。

真实在线验证待用户授权，并需有效 endpoint、宿主安全注入的项目 key / credentials / running binding，以及对应项目能力。SDK/API 网页有 beta 门槛，公开 Go 模块实际可下载。没有尚未解除的自动审批拒绝，也没有结果未知的真实 Pine 写操作。

live start 已实现官方 StartComputer 和 AES-256-GCM 可插拔 Vault，需显式 live/计费确认，启动已有身份。新增测试使用固定合成凭证验证加密认证、错误密钥、篡改、权限、启动前持久化、保存失败和未知结果防重复。没有访问 Keychain 或执行真实启动。live answer 的 SDK 缺少独立预算字段，必须明确接受运行时限制。金额总限、后台停机、Windows、直接 shell/MCP/浏览器控制未实现。

代码自审：检查凭证、幂等和路径；修复既有 state-dir 权限不足与无 session 停止问题。剩余本地同用户篡改、密文回滚、跨目录并发及远端写入竞态见 development.md。源码 ZIP 使用白名单打包，并在独立临时目录解包运行测试与构建；不含编译产物或状态。

## 首次创建补充验证（2026-10-22）

新增 computer create；本轮共 23 个测试函数（含子用例）。`go test ./...`、`go test -race ./...`、`go vet ./...`、`go build -o bin/pine-agent .` 均通过。

新增覆盖：创建前凭证及 pending 加密落盘；ready 重复调用不创建；丢失响应禁止重复创建及重新生成身份；create/start 操作隔离；prepared 重试复用身份；prepared/pending/ready 三个保存位置故障；远端身份不符；endpoint 不符；核实 binding 后 reconcile 不触发新创建；缺少授权参数时不创建状态目录。

测试只注入固定合成凭证和假的远端函数；没有调用 GenerateCredentials 生成真实身份，没有配置真实凭证、访问 Pine 或启动计费资源。真实 API 权限、首次创建、成本与端到端结果仍未验收。Skill `quick_validate.py` 校验通过；构建产物 `computer create --help` 检查通过。旧源码 ZIP 的解包验证仅覆盖初版，本轮未重新打包。


## 接入修复补充验证（2026-10-22，仅离线）

共 29 个测试函数（含子用例），新增 6 个 SDK 适配回归测试：

- Stop 的 true/false/error 组合、只调用一次、未确认时内存和磁盘均不标记停止。
- 非 upload 注册产物与 workdir 引用合并，保持空 turn filter；Open/OpenFile 分流、相对目录、task 不存在及读取失败。
- workdir 路径/根类型拒绝、checksum 不符阻止停止。
- terminal_reason 落盘、RunUnconfirmedError 内嵌 APIError 元数据保留、敏感字段排除。
- 空事件页推进、跨进程游标恢复、ExpectedTaskID/SkipFrames/Limit、安全元数据存储。
- 错误/无效/倒退游标和不匹配 task 不得作为有效事件页。

`go test ./...`、`go test -race ./...`、`go vet ./...`、构建和 Skill `quick_validate.py` 均通过；`git diff --check` 通过。固定 SDK v0.3.19 未改动。测试使用本地接口替身和合成 SDK 类型，不包含真实网络或 SDK HTTP 线级契约验收，不读取真实凭证或 Keychain，不重启资源、不重跑在线任务。本轮未更新旧源码 ZIP，也不把修复作为远端故障根因已确认的证据。

## 默认 token 预算更正（2026-10-22，仅离线）

共 31 个测试函数（含子用例）。新增 CLI 回归覆盖省略 max-tokens 与显式 0 的同请求重放、正数覆盖、预算变化触发 request_mismatch、负数拒绝以及 wall 非正/超过 1h 拒绝；SDK JSON 测试确认 0 省略 max_tokens、正数原样发送、max_wall_ms=60000 保留。

`go test ./...`、`go test -race ./...`、`go vet ./...`、构建、Skill quick_validate 和 git diff --check 均通过。构建产物的 task run --help 已显示默认 0 及非费用上限说明。没有修改 SDK、读取真实凭证、重启资源或重跑在线任务；本轮没有验证取消 token 上限后的在线业务成功。首次失败原因依据用户转述的 Pine 回复更正，第二次原因不直接外推。


## 零 token 上限在线闭环（2026-10-22）

客户端 0a96358、SDK v0.3.19：新身份安全保存、创建 Computer/session、单次 Run/Wait、非空产物下载、事件元数据读取与停止均成功。任务 state/terminal_reason 均为 completed，耗时 29382ms。注册 artifact 和 workdir 两条回收路径得到同一份 259 字节文本，内容与 SHA256 相同；saved=2 不是两份独立产物。停止返回 true,nil，随后独立只读查询返回 runtime NotFoundError。

Agent 计价 USD 0.011489736，total_tokens=118888（含缓存读写），compute=null。主标题缺失判断仅来自 Agent 和文件，没有 DOM/截图独立验证；Computer/egress 精确费用及完整总额尚未取得。未执行 answer、账号写入、发布、重启或额外任务。本次证明最小执行及回收闭环成功，不证明业务字段全部准确、时间限制触发或总费用硬上限。详见 online-verification.md。

本次文档同步仅检查既有本地证据、Markdown 差异与链接、Skill quick_validate；未改代码，未重跑 Go 测试或在线验收，未启动资源。

## 2026-10-23：auth setup 与 macOS Keychain

新增本地 auth setup、隐藏终端输入、按 endpoint 保存的 API key/包装密钥原子记录，以及 live SDK/Vault 的环境优先与 Keychain 回退。默认 endpoint 为 https://pinesandbox.io。重复 setup 不覆盖记录，未知结果禁止盲目重试的生命周期逻辑保持不变。

新增离线测试覆盖：随机 32 字节密钥生成、已有密钥导入、重复配置不换钥、环境冲突、空/非法输入、存储读取/保存失败、损坏记录、endpoint 隔离、环境覆盖不读取 Keychain、错误密钥拒绝解密且不改变旧 pending Vault、CLI 输出无秘密及帮助/默认值。使用假的 AuthStore 与合成凭证；不访问真实 Keychain。

验证通过：工作区与独立待提交快照的 `go test -race ./...`、`go vet ./...`、正常 macOS+cgo 构建；`CGO_ENABLED=0 go test ./...` 验证环境模式；构建产物 auth setup --help；仓库及已安装 Skill 的 quick_validate；git diff --check。工作区原有网站登录 Secrets 改动未纳入本次提交，独立快照不依赖它们。

未配置真实 API key、未创建或读取真实 Keychain 条目，未访问 Pine、创建身份、启动资源或重跑任务。Security.framework 适配仅编译验证，真实系统授权弹窗/访问控制及跨构建重用尚未实测。历史在线结果的主标题未核实、完整 Computer/egress 费用未取得边界仍保留。本次未重新打包旧源码 ZIP。
