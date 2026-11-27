# 阶段 05 · 评测查询

新增 `status {target}`、`result {target,id}`，只读且需要已登录会话。返回 `{accountId,target,accepted,latest?:{id,result,language}}`。查询账号和每条记录的账号、题目、提交 ID 均需匹配；未知/缺失身份拒绝，不返回原始评测详情。status 从当前接口可见记录中取最大数字 ID 作为最近可见提交；accepted 是可见 AC 证据，不是完整历史证明。result 只返回指定 ID，accepted 仅指该次结果。保留原始状态码，不将未知状态映射为 AC。无轮询后台线程、无自动提交、无源码传输。真实平台记录字段、分页与状态码尚待验证。

Linux race/vet 和单元测试通过，Windows amd64/macOS arm64 CGO=0 交叉构建通过。模拟 AC 后 WA、错误账号、指定 ID 验证。真实平台无操作。
