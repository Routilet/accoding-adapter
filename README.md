# Accoding Adapter

Routilet 的独立 Accoding 在线评测适配器，供 Companion 按需调用。正式发行目标 0.1.0，适用于 Windows 11 x64 和 Apple Silicon macOS 12+。

## 安装与使用

通过 [Routilet](https://github.com/Routilet/routilet) 的正式通道安装并启用 `@routilet/accoding-adapter`。Companion 本地编译、样例测试等功能不依赖 Adapter。Adapter 为独立 Go 原生程序，无运行时 Node.js 依赖。

- 账号登录、注销、状态查询；会话仅存于适配器进程内存，保存凭据由宿主负责。
- 近期比赛、题目与公开样例读取，题面与评测状态查询。
- 核验提交目标、账号、语言与源码，明确确认后提交；响应不确定时先核对未决记录，不自动重复 POST。
- 使用 `--stdio` JSON 行协议 `routilet-oj-adapter/1`，能力握手及原生包 manifest 校验版本与身份。

仅使用账号已有权限。不要将真实凭据放入命令行、日志、仓库或公开 CI；遵守课程与平台规则。

## 开发与发行

```sh
go test ./...
go vet ./...
go build ./cmd/accoding-adapter
```

发行使用原生双平台测试、构建、SHA256 清单、实际 npm tarball 离线安装与能力握手；通过后发布到 npm latest。自动测试使用合成数据，不进行真实 OJ 提交，不代表所有平台页面和网络状况已验证。macOS 使用 ad-hoc 签名。

- [正式版说明](docs/release-0.1.0.md)
- [历史 beta 发行说明](docs/beta-release.md)
- [验证记录](TEST_REPORT.md)

MIT · Routilet.
