# 阶段 04：在线提交（开发预览）

`prepare {target,sourceHash}`：显式用户操作，只读核验账号、稳定题目 ID、比赛有效期和允许语言。返回 `{token,target,accountId,languages,sourceHash}`。令牌仅在当前进程有效，5 分钟过期，换号、注销、新 prepare 或任何 submit 消耗；不自动切换比赛/题库目标。

`submit {token,language,code}`：一次性令牌、精确语言及 UTF-8 源码 SHA-256 匹配后，核心再次核验远程目标及账号。源码上限 4 MiB，协议帧仍 8 MiB。成功返回 `{id,target,accountId,language,result,sourceHash}`，仅代表已识别的远程提交，不代表 AC。缺少身份、语言或题目字段的 POST JSON 不足以确认；题库仍可进行有限只读源码匹配。没有自动 POST 重试。

提交前写入用户配置目录 `Routilet/accoding-adapter/submissions-v1` 下的元数据（账号 ID、目标、源码 SHA-256、语言、时间）；不写密码、Cookie、源码或评测详情。使用排他目录锁、同步临时文件及无覆盖硬链接发布。非普通文件拒绝。未决记录按账号+题目 ID 阻止跨比赛/题库路由重发。若进程在 POST 前后退出，未决记录或残留锁保守阻止提交。文件系统须支持硬链接；不支持则拒绝发送。目录同步在系统支持时尽力执行，不保证突然断电/外部篡改安全。本阶段提供题库未决只读核对；比赛未决不自动认领，先到 OJ 核对，由维护者协助处理记录，勿直接删除保护记录。`unknown_outcome` 表示不能确认服务器是否收到了请求。

本阶段仅返回首次评测状态及提交 ID，不轮询、不展示题目历史通过状态、不自动刷新。真实 Accoding 接口及实际提交需授权账号和测试题，未执行。测试使用合成平台与假客户端，不产生真实提交。

`reconcile {problemId}`：主动核对当前账号与该题未决记录，只读访问 OJ；`none` 无未决、`resolved` 唯一且有明确源码/身份/语言/时间匹配的题库提交，`unresolved` 未能匹配，`manual_required` 比赛提交暂不具备可靠自动认领条件。只在 `resolved` 时持久确认回执，不存在“查不到即允许重发”的行为；残留锁需要维护者核查。该入口不查询全站历史、不触发新提交。
