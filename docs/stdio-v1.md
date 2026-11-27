# Adapter stdio v1（阶段 01 预览）

本阶段只验证传输和生命周期；不冻结业务方法或安装发现接口。Adapter 运行 `accoding-adapter --stdio`，无端口、无 shell、无启动联网。原只读探针独立保留。

- 双向 UTF-8 JSON Lines，每帧必须以 LF 结束，最大 8 MiB（不计 LF）；stdout 仅协议，stderr 不含请求、响应或凭据。请求 ID 为正整数且在同一进程严格递增；每次只允许一个在途请求，响应 ID 对应请求。
- 请求 `{"id":1,"method":"capabilities","params":{"protocol":"routilet-oj-adapter/1"}}`，成功响应 `{"id":1,"result":{"protocol":"routilet-oj-adapter/1","adapter":"accoding","version":"0.0.0-dev","methods":["capabilities"]}}`。`0.0.0-dev` 是开发标记，不是发行版。后续发行注入真实版本。
- 失败响应 `{"id":1,"error":{"code":"incompatible_protocol","message":"unsupported protocol"}}`；result 与 error 二选一。未知方法 method_not_found；无效参数 invalid_params；不兼容协议 incompatible_protocol。结构错误、非法 UTF-8、截断、超长帧关闭流。不得把未知响应当作成功。客户端先完成握手，再使用协商能力。
- 宿主启动时握手默认 5 秒；调用上限 60 秒，可由调用方缩短。排队时取消不影响当前请求；在途取消、超时、断管或协议错误均终止当前子进程并回收，不透明重启、不重发。重新建立会话须调用方主动决定；未来有副作用的请求必须结合持久化尝试记录，将断线视为可能已发送（阶段 04）。本阶段不存在提交接口。
- 安装器提供可信且经过校验的绝对路径是后续集成前提；测试显式传绝对路径。此协议不负责发现/安装/更新/运行租约或沙箱，原生插件必须可信。当前 Adapter 不产生后代进程；不承诺强制退出宿主后回收任意派生进程。

## 阶段验收

Go 1.23.2 Linux：`go test -race ./...`、`go vet ./...` 通过；Adapter Windows amd64/macOS arm64 CGO=0 交叉编译通过。Companion 宿主用真实 Adapter 二进制及模拟进程验证握手、请求/结果、错误、错误 ID、截断、超长、取消、崩溃、关闭、并发串行化；宿主单独包测试/race/vet 及两平台测试二进制交叉编译通过。未执行完整 Wails 构建或真实 OJ；远程原生 CI 另行记录。

未实施 login/logout/account/resolve/import/prepare/submit/result/status 业务方法，未接 GUI、安装器、凭据持久化、正式发行。阶段 02 起按实际业务逐项扩充方法，不需要额外 SDK 仓库。
