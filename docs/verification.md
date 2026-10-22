# 验证结果

最新在线记录：2026-10-22 已获授权执行首次在线测试，生命周期调用完成并停止资源，但只读任务 failed、产物为 0；详见 [在线记录](online-verification.md)。下文初版和首次创建补充为当时的离线验证记录。

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
