# beta.2

修复缩进代码块和明确输出标签。新增只读 `contests`、`contest_status`、`import_document`、`status_detail`、`result_detail`。旧方法响应保持兼容，按能力协商。仅当前账号、比赛和请求的题目；不扩大读取权限，不增加提交操作。原始 HAR 不进入仓库，使用合成最小回归夹具。