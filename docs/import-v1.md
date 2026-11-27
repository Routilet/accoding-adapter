# 阶段 03：只读导入

`resolve {url}` 无网络解析可信 HTTPS 学生端/4000 端链接；`import {url}` 要求已有登录会话，并先核对账号。比赛 URL 导入整个比赛（包括比赛内页面的 hash），题库 URL 导入单题，题序不是远程 ID。拒绝其他域名、凭据、非 HTTPS、查询参数和不识别的路径。管理端使用已有权限，不另行登录或升级权限。

结果 `{accountId,title,problems:[{target,title,label,sourceUrl,samples,warnings,special}]}`。仅传递稳定标识和公开样例，不传完整题面、HTML 或图片；比赛原题入口为原比赛链接。特殊评测/交互题不自动建立比较样例。数量与帧长有限制。未开放提交接口。

本地 `go test -race ./...`、`go vet ./...` 及 Windows amd64/macOS arm64 CGO=0 交叉构建通过。新增链接、无登录拒绝、账号检查、单次比赛读取、题面裁剪、特殊评测测试。真实 Accoding 仍待验证。
