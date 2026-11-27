# Accoding Adapter · 业务核心验证版

独立 Go 工程，目标是迁移旧 Accoding Companion 的平台业务能力。**未冻结插件接口，未接入 Companion/Routilet，未发行。** 当前只依赖 Go 标准库（Go 1.23+）。

## 业务能力

- 学生端 CSRF 表单登录、内存 Cookie 会话、账号查询、注销及会话取消；管理端独立 Cookie 快照、只读权限探测。
- 比赛读取、按题序排序和映射、评测设置解析、题库页面读取、公开样例提取；管理端导入失败时回到学生端读取。只使用账号已有的访问权限。
- 提交前确认远程题目、语言、账号和比赛时间；比赛/题库路线明确指定，比赛结束时不自动切换提交目标。
- 比赛 JSON 提交和题库表单提交，必要时按旧版已识别的站点传输编码处理源码。拒绝跨域请求，不重复 POST，不执行远程脚本。
- 查询提交历史、单次评测结果和有界轮询；题库 POST 丢失响应后只读查询，按题目、账号、语言及源码匹配唯一记录；独立未决尝试恢复接口。
- 提交前强制记录 `Journal.Begin`。验证版提供 `FileJournal`，用追加式文件保留未决记录；只记录哈希和元数据，不保存源码或登录凭据。未决提交阻止相同账号/题目再次发送，包括不同提交路线。

## 使用

```sh
go test -v ./...
go vet ./...
go test -race -cover ./...
go build ./cmd/accoding-probe
```

开发专用 `accoding-probe` 是一次性的只读命令行探针，并非插件协议。支持 `account`、`contest`、`import`、`problem`、`history`、`result`、`prepare`；账号和密码从 stdin 读取 JSON 对象（`username`、`password`），不作为命令行参数，不写日志或磁盘。请通过可信的本地秘密输入机制提供 stdin；不要在 shell 历史、脚本、仓库或公开 CI 中存放真实凭据。

例如，已经安全准备好标准输入时：

```sh
./accoding-probe -op contest -contest 123
./accoding-probe -op problem -problem 456
./accoding-probe -op history -contest 123 -problem 456
```

编号为示例。Windows 使用 `accoding-probe.exe`。探针每次调用重新登录；`-service admin` 仅用于明确选择管理端题库读操作。标准输出可能包含课程题目/私人提交信息，不应公开。

Go 核心 API 位于 `accoding` 包：`New`、`Login`、`Logout`、`Account`、`GetContest`、`ImportContest`、`GetProblem`、`Prepare`、`Submit`、`ListSubmissions`、`GetSubmission`、`Monitor`、`RecoverAttempt`。这些名称和数据模型只是内部验证接口，并非对外兼容承诺。

`Submit` 必须由调用方明确确认源码、题目、语言和账号，并提供 `Journal`；开发探针刻意不提供提交功能，测试只使用模拟 HTTP transport。使用真实提交 API 前需要真实平台验证及用户授权。`FileJournal` 必须放在本用户独占、可信的绝对目录，不应放在课程共享目录；若进程异常留下 `.lock`，需核对服务器状态且确认无进程使用后人工恢复，不自动清理或重发。`Resolve` 仅接受已经验证的提交记录。断电耐久性和 Windows 文件语义尚待真机验证，不应把该实现视为正式存储契约。

## 来源与差异

旧版参考：[Routilet/accoding-companion](https://github.com/Routilet/accoding-companion)，审计时 main 最近提交 `34b5ea34e0111060751ba60cd757a852914bc760`。迁移规则来自 `src/accoding/{client,adapters,import}.ts`、`src/auth/session.ts`、`src/problems/statement.ts`、`src/submissions/{send,store,monitor}.ts`、`src/model.ts`；保留旧 MIT 许可。组织共同决策参考 [ecosystem.md](https://github.com/Routilet/.github/blob/main/docs/ecosystem.md)。

本次采用仅内存会话、固定站点、Go 标准库、独立 `Journal` 与只读探针作为验证默认值，均可在下一轮设计评审改变。尚未实现或未达到旧版完全等价：

- 真实账号及接口验证：所有网络测试为合成 fixture，未验证当前平台返回结构、提交成功率、真实权限和真实评测轮询。
- 系统 Keychain/DPAPI、保存登录及自动登录恢复：暂不实现，不落盘凭据。
- 题库 HTML 使用有限的标准库解析器输出纯文本，Markdown 按常见标题和 fenced code 提取样例；没有完整 HTML5/Markdown 渲染、图片、数学公式转换、缩进代码块与复杂题面兼容。Markdown 内容为不可信远程数据，未来 GUI 必须安全渲染；不能直接插入 DOM。未解析成功时由调用者提示手动检查；样例数量不匹配或 fenced code 未闭合时不导入可能错误的数据。
- GET 网络/服务端临时失败最多重试两次，退避 250/500ms，整个请求序列最多 20 秒；限流、认证、权限及协议错误不自动重试，所有 POST 不重试；题库提交恢复最多三轮，未决时不重发。比赛 POST 丢失响应的自动唯一匹配未实现（保留未决，人工核对）。
- 未实现正式提交记录管理 UI、主动解除未决记录、账户持久化或旧 `.accoding` 数据迁移。FileJournal 是验证用最小实现，不代表后续 Companion 的存储设计。
- 插件 IPC、宿主、GUI、安装/更新/卸载、npm、Routilet 集成均不在本阶段。

## 验证结果

见 [TEST_REPORT.md](TEST_REPORT.md)。CI 工作流只执行检查与构建，不发布。真实平台验证仍待授权账号和可访问网络。
