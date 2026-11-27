# Adapter 0.1.0-beta.1

仅测试通道 next；不代表真实 Accoding 联合验收通过。独立 npm 包 @routilet/accoding-adapter，无安装脚本或运行时 Node 依赖。包中 bin/win32-x64/accoding-adapter.exe、bin/darwin-arm64/accoding-adapter 与 manifest.json（schemaVersion 1，SHA256）提供候选发现约定；不等于 Routilet 正式插件发现/租约协议。运行 --stdio，能力握手报告实际版本。macOS ad-hoc 签名。仅发布分支 release/adapter-v0.1.0-beta.1 的固定版本允许 next 发布，CI 原生测试、实际 tarball 安装和离线握手均通过才发布。npm 授权使用可信发布 release.yml 或已授权的 NPM_TOKEN（首次发布需要账号所有者授权）。不自动发布 latest、不改 Companion/Routilet，不做真实 OJ 写操作。失败时可从 Actions 下载 adapter-npm-candidate 后由授权维护者发布原 tarball。
