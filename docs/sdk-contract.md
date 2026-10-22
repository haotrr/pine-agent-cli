# SDK 核验记录

2026-10-22，仅公开资料与包源码，无登录。

- 官方 [overview](https://console.pinecomputer.io/docs/overview) 指明 Go 包 `go.pinesandbox.io/computer`。
- [SDK 页面](https://console.pinecomputer.io/sdks) 和 [Computer API](https://console.pinecomputer.io/spec/computer) 网页显示 beta 登录门槛。
- 公开 Go proxy 成功返回并下载 **v0.3.19**，时间 `2026-10-22T00:10:21Z`，GoVersion 1.23，来源仓库 `https://github.com/RunVid/pinesandbox-go`，tag commit `a318b4b330a8b37f035864b1b79fe3834b5aef1d`。
- 模块校验和 `h1:pNuJWTtsyi+38M5JzzFWfWYJSpQtgjEwSibb0+agwwc=`，版本与传递依赖固定在 go.mod/go.sum。

实际查看 client.go、computer_binding.go、agent.go、session.go、files.go 与内部公开类型的定义。适配器使用公开 SDK：NewClient、ReconnectComputer、Session/CreateSession、RequireAgentProtocol、Agent Run/Wait/Status/Answer/Cancel、ListArtifacts/OpenArtifact、Sessions/Stop。不调用猜测路由。

关键约束：ReconnectComputer 通过 AdoptExisting 重建 handle，不分配 runtime；Binding 包含秘密 token，不写入 CLI journal。RunOptions 有 IdempotencyKey 和 Constraints；**AnswerOptions 仅有 Secrets**。Session 名用作 SDK 地址；不保存 ps_，后续读和 Agent 操作使用 ct_。ListArtifacts 空 turnID 包含下载；下载不可仅按 turn 筛选。

公共状态语义参考 [Agent](https://console.pinecomputer.io/docs/agent)；产物保留与 Shell 权限参考 [Files and shell](https://console.pinecomputer.io/docs/files-and-shell)；会话生命周期参考 [Sessions](https://console.pinecomputer.io/docs/sessions)。这些资料不构成真实项目 API 权限证据。

真实 start 已使用官方 StartComputer(ctx, Credentials, StartOptions)，仅启动已有身份；StartOptions 设置 Timeout、ReadyTimeout=60s、MaxBindAttempts=1。调用前加密持久化 credentials 和 pending，成功后保存 Computer.Credentials()/Binding()。未知结果不自动 Start/Recover。SDK 能编译不意味着在线集成已验证。

2026-10-22 首次创建补充：读取本地 v0.3.19 client.go、credentials.go、computer.go。GenerateCredentials 离线生成 UUIDv7、state key、capture keypair；CreateComputer 的 AttachOptions 接受完整 Credentials。CLI 在请求前保存完整凭证及 pending，设置 Timeout、ReadyTimeout=60s、MaxBindAttempts=1；不依赖 SDK 失败后才返回恢复凭证。没有执行真实身份生成或在线请求。


2026-10-22 接入修复：对照官方 [best practices](https://console.pinecomputer.io/docs/best-practices) 与 [files and shell](https://console.pinecomputer.io/docs/files-and-shell)，离线核验 v0.3.19 源码并修复：

| 合约 | CLI 行为 |
|---|---|
| Computer.Stop 返回 (bool,error) | 仅 true,nil 确认停止；false,nil 不得宣称停止或盲目重试 |
| AgentResult.Artifacts 为 FileRef | 读取当前 task 的 Turns，通过 OpenFile 回收 workdir 相对路径；注册 artifact 仍用 OpenArtifact |
| AgentResult.TerminalReason / APIError | 保留终止原因与安全的请求标识、问题类型、HTTP 状态；不导出原始错误正文 |
| EventsPage 的 Limit 是扫描数量 | 逐页持久化 NextAfter，空页也可能尚未结束；ExpectedTaskID 绑定游标；只保留事件元数据 |

离线测试只证明客户端适配和持久化行为，不证明远端环境恢复，也不解释既有 token/费用差异；未重启资源或重跑在线任务。
