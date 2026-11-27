# 阶段 02 · 账号接口（开发预览）

在 stdio v1 基础上新增 `account`, `login`, `logout`，capabilities 宣告四个方法；仍不提供导入或提交。每个进程持有一个内存会话。

- `login {username,password}`：显式登录，先清除旧身份；失败保持注销态。凭据仅经 stdin，不在响应、日志、argv 或持久文件出现。成功返回 `{"state":"signed_in","id":"123","nickname":"..."}`。
- `account {refresh:false}`（默认 false）：仅返回本地会话状态，不联网。`refresh:true` 仅已登录时查询服务端身份；过期、权限拒绝、身份变化清除本地会话。临时网络/服务异常返回稳定错误，不宣称注销或自动重新登录。
- `logout {}`：丢弃本地 Cookie 和身份，不声称撤销平台服务器端所有会话。退出进程也清理内存会话。注销后返回 `{"state":"signed_out"}`。
- 错误码 `auth`, `forbidden`, `network`, `rate`, `server`, `cancelled`, `protocol` 或 `invalid_params`；消息为固定文本，不传播服务端响应、密码或 Cookie。输入长度限制 username 1024 字节/password 4096 字节。协议帧限制不变。
- 凭据持久化由 Companion 账号宿主在用户显式选择时使用系统凭据库；Adapter 自身不保存密码，不在启动时联网或读凭据。正式安装路径/租约待后续集成。

本地 Linux：`go test -race ./...`、`go vet ./...` 通过，新增模拟登录/换号、错误密码、仅显式刷新、过期/网络异常测试；Windows amd64/macOS arm64 无 CGO 构建见 CI。真实平台登录未验证。
