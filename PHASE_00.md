# 阶段 00：业务核心入库与验证

依据：Routilet 组织共同决策及维护者确认的 Adapter 实施计划。仅独立 Go 业务核心；不冻结 IPC，不接入 Companion，不改 Routilet、Exam Kit 或 Observer，不发布。

## 本次变更

- 迁入上一轮独立 Go 核心与只读 CLI 探针；旧 TS 代码仅作行为参考。
- 增加有界 GET 重试，POST 不重发；限流、身份与协议错误直接返回。
- 公开样例数量不匹配、未闭合代码块不导入可能错误的数据。
- CI 执行 vet、单测、构建；本地竞态检测和目标交叉编译通过，见 TEST_REPORT.md。

## 实机只读验收

使用维护者授权的普通学生账号，本机运行 `accoding-probe`；账号、密码仅经 stdin 传入，禁止提交到仓库或 CI。检查 account、contest、import、problem、prepare、history、result；比较官方页面的 ID、题序、样例及状态。不要记录密码、Cookie 或私人题目正文。探针无提交功能。

## 尚未完成

真实平台接口尚未测试（无账号、环境不能访问平台），不得宣称可投入生产。比赛 POST 不确定结果仍保守记录、拒绝重发，自动恢复需确认真实平台详情接口；正式日志/凭据存储及 IPC 留后续阶段。样例解析非完整 Markdown/HTML5 实现，复杂题面需人工核验。禁止把辅助工具用于课程禁止的场景。
