# CyberAgent v0.1.4 Agent-Native Polish Plan

版本：v0.1.4  
日期：2026-05-25  
状态：第十八轮实现已完成；计划、代码与验证已同步

---

## 1. 目标

v0.1.4 不扩面，只做三类打磨：

```text
1. Planner 异常路径 UI 更细粒度
2. approval 更明确绑定到 activeRunId / step_id
3. agent 会话恢复后自动补齐 run context snapshot
4. active_run context refs 自动提炼 report / evidence / artifact 关键摘要
```

---

## 2. 非目标

```text
不新增全新 Agent 子系统
不做多用户平台
不做长期 RAG / 自动 exploit
```

---

## 3. 第一轮改动（已完成）

### 3.1 Planner 异常路径

已完成：

```text
- Planner 结构解析成功但 answer 为空时，前端提示为空答案兜底
- Planner 失败时前端展示 planner_parse_error 原因
- 后端 plan.Answer 为空时回写原始模型内容，避免 planner 成功但 answer 丢失
```

涉及文件：

```text
internal/interfaces/daemon/agent.go:612
web/frontend/src/pages/agent-cockpit.tsx:843
```

### 3.2 approval 绑定 activeRunId

已完成：

```text
- checkApprovals 优先检查 activeRunId
- 若当前 run 有 pending approvals，直接同步 activeRunApprovals 和 nextAction
- 若当前 run 无 pending，再 fallback 扫描最近 runs
- 后端读取/列出 Agent session 时会同步 active_run pending approvals，并把 next_action 标记为 approval_required
- approval queue 工具结果沉淀 run / approval / approval_queue context_refs
- RunService counts 增加 pending_approvals，前端 Active Run 卡片展示待审批计数
- 后端读取/列出 Agent session 时会补齐 active_run context snapshot：run / report_json / artifact_index / finding / approval context_refs 与 ActiveTask summary
- context refs 不只显示 counts，还会从 report.json / artifacts index 提炼关键 finding / evidence / artifact 摘要
```

涉及文件：

```text
web/frontend/src/pages/agent-cockpit.tsx:1609
```

### 3.3 会话恢复补齐 snapshot

已完成：

```text
- applyRestoredAgentSession 改为 async
- 恢复 session 且存在 activeRunId 时，自动触发 checkApprovals
- 保证恢复后前端立刻拿到 run approval 状态
- Cockpit Runbook 合并 session context_refs、active run、finding、approval gate，展示上下文引用面板
- Active Run 卡片展示 findings / artifacts / events / pending approvals，并提供“让 Agent 总结证据”入口
- session resume / read / list 会自动触发 run context snapshot hydrate，避免刷新后丢失 run/report/artifact/finding 上下文
- hydrate 时优先读取 report.json，补充 lead finding、lead evidence；artifact_index ref 还能 fallback 读取 artifacts/index.json
```

涉及文件：

```text
web/frontend/src/pages/agent-cockpit.tsx:802
```

---

## 4. 第二轮改动（已完成）

### 4.1 approval 绑定 step_id + 定位功能

已完成：

```text
- AgentApprovalPanel 中 step_id 显示为可点击链接
- 点击 step_id 跳转到对应 run 详情页 (/scan/{runId})
- 支持无 activeRunId 时降级为纯文本显示
- Cockpit approval queue 展示 step_id、policy_decision_id、risk_summary、resolved_by/comment，并保留 approve/deny/resume 操作
```

涉及文件：

```text
web/frontend/src/pages/agent-cockpit.tsx:2832
```

### 4.2 Planner 失败原因在 ToolCallRow 折叠态显示

已完成：

```text
- ToolCallRow 折叠态新增 planner err 警告标记
- 仅当 tool.result.planner_parse_error 为 string 时显示
- 使用 amber 色调与 warning 图标风格一致
```

涉及文件：

```text
web/frontend/src/pages/agent-cockpit.tsx:3039
```

### 4.3 会话恢复显式加载 events/artifacts/findings

已完成：

```text
- applyRestoredAgentSession 中 checkApprovals 后显式加载事件/证据/报告
- 使用 Promise.all 并行加载 getEvents/getArtifacts/getReport
- 加载失败静默处理，不阻断会话恢复流程
- 依赖数组已补充 client/setActiveRunEvents/setActiveRunArtifacts/setActiveRunReport
```

涉及文件：

```text
web/frontend/src/pages/agent-cockpit.tsx:814
```

---

## 5. 第三轮改动（已完成）

### 5.1 Workspace staged edit / diff / apply

已完成：

```text
- Agent API 版本提升到 v0.1.4
- 新增 session.pending_changes 持久化状态
- 新增 workspace.files.diff：生成单文件 exact old/new staged diff，不修改文件
- 新增 workspace.files.diff_many：生成多文件 staged change set，共享 group_id，不修改文件
- 新增 workspace.files.rebase：冲突后基于当前文件 old 文本重新生成 staged change，并 supersede 旧 change
- 新增 workspace.files.apply：按 change_id 应用 pending change，落盘前重新校验 old 文本仍唯一匹配；不匹配时返回 409 并标记 conflict
- 新增 workspace.files.apply-group / discard-group：按 group_id 批量应用或丢弃同一个多文件修改集；apply-group 先全量预检，任一文件冲突则不写入任何文件
- 新增 workspace.files.discard：按 change_id 丢弃 pending/conflict change，不修改文件
- 自然语言支持：“预览修改文件 notes.md old: ... new: ...”、“应用 change_id: ...”、“丢弃 change_id: ...”、“应用所有待应用修改”与“丢弃所有待应用修改”
- HTTP API 支持：POST /workspace/files/diff、/diff-many、/rebase、/apply、/discard、/apply-group 与 /discard-group
- 前端 API client 新增 AgentWorkspaceChange / WorkspaceDiffResult / WorkspaceApplyResult 类型与调用方法
- Agent Cockpit 新增 Staged Changes 面板：展示 pending_changes、group_id、diff preview、Open、Apply、Discard、Apply all、Discard all、Apply group、Discard group；冲突时展示 error，且冲突项可通过内联 Rebase 表单重新生成或 Discard 清理；Rebase 支持工作区选区填充 old 文本、片段 diff 预览、完整文件级 diff 预览，以及 missing/empty/not_found/ambiguous 状态提示
```

涉及文件：

```text
internal/interfaces/daemon/agent.go
internal/interfaces/daemon/router.go
internal/interfaces/daemon/server_test.go
web/frontend/src/lib/api.ts
web/frontend/src/pages/agent-cockpit.tsx
```

---

## 6. 第四轮改动（已完成）

### 6.1 Run report / evidence 摘要自动提炼

已完成：

```text
- 新增 activeRunReportSnapshot：读取当前 active_run 的 report.json，并在需要时 fallback 读取 artifacts/index.json
- 自动解析 lead finding / lead evidence / lead artifact，并写入 context_refs summary
- run context summary 现在包含 findings/artifacts/pending_approvals 以及关键 finding/evidence 文本
- report_json context ref 现在包含关键 finding 与 evidence 摘要
- finding context ref 现在包含关键 finding、category 与 remediation
- artifact_index context ref 现在包含关键 artifact path 与关联 evidence 摘要
- ActiveTask summary 在可安全覆盖时刷新为包含 run counts + 关键证据的摘要
- 测试覆盖 Missing CSP / HTTP status / headers.json / evidence_001，验证读取 session 后自动 hydrate context refs
```

涉及文件：

```text
internal/interfaces/daemon/agent.go
internal/interfaces/daemon/server_test.go
```

---

## 7. 第五轮改动（已完成）

### 7.1 Agent Session List Run Summary

已完成：

```text
- Agent session list response 新增 run_summary，包含 run_id/status/target/findings/artifacts/events/approvals/pending_approvals/report_json
- run_summary 会复用 active_run report snapshot，返回 lead_finding / lead_evidence / lead_artifact / summary
- 前端 Agent Cockpit 左侧会话列表改为任务工作台卡片：显示 session next_action、active run 状态、findings/artifacts/pending approvals/events 指标
- 会话列表可直接展示关键上下文摘要，减少“点进去才知道这个 Agent 在干什么”的割裂感
- 无 active_run 的会话仍展示 message/tool/context 计数，保持轻量可扫读
- 左侧会话列表新增筛选器：全部 / 运行 / 审批 / 发现 / 阻塞，并显示每类数量
- 后端测试断言 list API 的 run_summary 包含 counts 与 lead_finding / lead_evidence / lead_artifact，避免只验证前端展示
- 左侧会话列表新增跨会话搜索：可按 session id、run id、active task、last message、target、lead finding/evidence/artifact 搜索
- 左侧会话卡片新增置顶/收藏，使用 localStorage 保存，置顶会话优先显示
```

涉及文件：

```text
internal/interfaces/daemon/agent.go
web/frontend/src/lib/api.ts
web/frontend/src/pages/agent-cockpit.tsx
```

---

## 8. 验证

```text
go test ./internal/interfaces/daemon -run 'TestHTTPAgent(WorkspaceDiffApplyAPI|NaturalLanguageInvokesWorkspaceTools)' -count=1     ✅ PASS
go test ./internal/interfaces/daemon -run 'TestHTTPAgentActiveRunApprovalGateAndContextRefs' -count=1                              ✅ PASS
go test ./internal/interfaces/daemon -run 'TestHTTPAgentActiveRunApprovalGateAndContextRefs|TestHTTPAgentTerminalActiveRunSyncsOnReadAndList|TestHTTPAgentSessionAndMessageAPIs' -count=1  ✅ PASS
cd web/frontend && npm run build                                                                                                    ✅ OK (tsc + vite)
go test ./...                                                                                                                       ✅ PASS
```

---

## 9. 第六轮改动（已完成）

### 9.1 Step timeline deep-link / focus

已完成：

```text
- 运行详情页支持 /scan/{run_id}?step={step_id}
- ScanDetail 加载 events 时保留 event id / job_id / step_id / data，便于按 step 精确匹配
- ScanDetail 同步加载 approvals，右侧展示审批列表
- 命中 step 后自动滚动到对应事件，并高亮事件行
- 顶部显示 Focused step 摘要，展示匹配事件数和审批数
- findings 区按 finding.source.step_id 高亮相关发现
- Agent Cockpit 审批队列中的 step_id 改为深链，可直接进入运行详情并定位 step
- Context refs 链接会尝试从 ref 信息中识别 step_id 并附加到 /scan 链接
```

涉及文件：

```text
web/frontend/src/lib/api.ts
web/frontend/src/pages/agent-cockpit.tsx
web/frontend/src/pages/scan-detail.tsx
```

---

## 10. 验证

```text
cd web/frontend && npm run build  ✅ OK (tsc + vite)
go test ./...                    ✅ PASS
```

---

## 11. 第七轮改动（已完成）

### 11.1 Step execution chain aggregation

已完成：

```text
- ScanDetail 现在加载 artifacts/index.json，并把 artifacts 带入页面状态
- 运行详情新增“执行链 / Execution Chain”视图，按 step_id 聚合 events / approvals / findings / artifacts
- 每个 step 卡片展示事件数、审批数、发现数、artifact 数，并显示最近事件和首个 finding 摘要
- step 卡片可点击，跳转到 ?step={step_id} 并复用上一轮的自动定位/高亮能力
- 聚合逻辑支持 event.step_id、event.tool、event.data 中的 step 字段、finding.source.step_id、artifact.step_id、approval.step_id
```

涉及文件：

```text
web/frontend/src/lib/api.ts
web/frontend/src/pages/scan-detail.tsx
```

---

## 12. 验证

```text
cd web/frontend && npm run build  ✅ OK (tsc + vite)
go test ./...                    ✅ PASS
```

---

## 13. 第八轮改动（已完成）

### 13.1 Run audit search / filters

已完成：

```text
- 运行详情新增审计搜索栏，可跨 step_id、event type/message/tool/data、finding、artifact、approval 关键字段检索
- 审计过滤支持全部 / 错误 / 警告 / 发现 / 证据 / 审批
- 执行链与事件列表复用同一搜索/过滤条件，计数展示匹配 steps/events 与总数
- 搜索/过滤后无匹配时提供明确空状态，避免误判为运行无数据
- 保留 ?step 深链高亮能力；过滤只影响当前可见列表，不破坏已有定位上下文
```

涉及文件：

```text
web/frontend/src/pages/scan-detail.tsx
```

---

## 14. 验证

```text
cd web/frontend && npm run build  ✅ OK (tsc + vite)
go test ./...                    ✅ PASS
```

---

## 15. 第九轮改动（已完成）

### 15.1 Step detail drawer

已完成：

```text
- 运行详情点击执行链 step 或打开 ?step=step_id 后，会自动展示单 step 详情抽屉
- 抽屉集中展示该 step 的 events / approvals / findings / artifacts
- 顶部给出错误、待审批、普通执行三类状态提示，并展示四类计数
- 关闭抽屉会清除 step 深链，回到当前 run 详情
- 该能力让 Agent 的“做了什么、在哪一步需要审批、产出了什么证据”可以按执行步骤复盘
```

涉及文件：

```text
web/frontend/src/pages/scan-detail.tsx
```

---

## 16. 验证

```text
cd web/frontend && npm run build  ✅ OK (tsc + vite)
go test ./...                    ✅ PASS
```

---

## 17. 第十轮改动（已完成）

### 17.1 Run Evidence Workbench

已完成：

```text
- 运行详情新增 Run Evidence 工作台，放在基础指标与 step/audit 区域之前
- 工作台统一聚合 findings / artifacts / approvals / report summary
- 顶部展示发现数、证据数、待审批数，作为运行证据态势入口
- 支持 Overview / Findings / Artifacts / Approvals / Report 五个视图
- Overview 自动提炼 lead finding、latest artifact、pending approval 与关键 evidence steps
- finding / artifact / approval 卡片可跳转到对应 step 深链，衔接 step detail drawer
- Overview 快捷按钮可同步设置运行审计过滤器，联动 execution chain 与 event list
```

涉及文件：

```text
web/frontend/src/pages/scan-detail.tsx
```

---

## 18. 验证

```text
cd web/frontend && npm run build  ✅ OK (tsc + vite)
go test ./...                    ✅ PASS
```

---

## 19. 第十一轮改动（已完成）

### 19.1 Agent Evidence Explain

已完成：

```text
- Agent Cockpit 的“让 Agent 总结证据”升级为“让 Agent 解释证据”
- 证据解释 prompt 会自动携带 active_run、target/status/counts、context_refs 摘要与 pending approvals
- Agent Runbook 的 Context refs 区新增 Explain 快捷按钮，可基于当前上下文直接发起解释
- 输出要求明确为：证据说明、关键 finding、支撑 step/artifact/report、审批 gate、下一步安全动作
- 如果细节不足，prompt 明确要求 Agent 先调用可用只读 run/report/artifact/approval 工具再回答
```

涉及文件：

```text
web/frontend/src/pages/agent-cockpit.tsx
```

---

## 20. 验证

```text
cd web/frontend && npm run build  ✅ OK (tsc + vite)
go test ./...                    ✅ PASS
```

---

## 21. 第十二轮改动（已完成）

### 21.1 Run Evidence Export

已完成：

```text
- Run Evidence 工作台新增 Markdown / JSON 导出按钮
- Markdown 导出包含 run 基本信息、lead finding、latest artifact、pending approvals、evidence steps、findings、artifacts、step groups 与 report
- JSON 导出使用 schema_version=cyberagent.run_evidence.v1，包含结构化 run/counts/findings/artifacts/approvals/evidence_steps/step_groups/report
- 导出文件名使用 cyberagent-evidence-{run_id}.md/json，并对 run_id 做安全文件名归一化
- 该能力用于审计复盘、报告提交与外部留档
```

涉及文件：

```text
web/frontend/src/pages/scan-detail.tsx
```

---

## 22. 验证

```text
cd web/frontend && npm run build  ✅ OK (tsc + vite)
go test ./...                    ✅ PASS
```

---

## 23. 第十三轮改动（已完成）

### 23.1 Evidence to Action

已完成：

```text
- Agent Cockpit 活跃运行卡片新增“证据生成下一步 / Evidence → next action”
- Agent Runbook 的 Context refs 区新增 Action 快捷按钮
- 该动作会基于 active_run、context_refs、pending approvals 自动构造“证据转行动计划”prompt
- 前端调用 agent control revise，将当前证据上下文转为可继续执行的 next_action / suggested_prompt
- 成功后控制输入框会填充 suggested prompt，并提示用户点击“继续”执行
- prompt 要求 Agent 生成 3-5 个有序 TODO、优先处理审批 gate、缺证据时先调用只读工具、避免无审批高风险动作
- 后端新增回归测试，验证 Evidence → Action 走 control revise / continue 时会保留 active_run、hydrated context_refs、approval gate 与 suggested_prompt
```

涉及文件：

```text
web/frontend/src/pages/agent-cockpit.tsx
internal/interfaces/daemon/server_test.go
```

---

## 24. 验证

```text
go test ./internal/interfaces/daemon -run 'TestHTTPAgentEvidenceToActionControlPreservesRunAndApprovalContext' -count=1 ✅ PASS
cd web/frontend && npm run build  ✅ OK (tsc + vite)
go test ./...                    ✅ PASS
```

---

## 25. 第十四轮改动（已完成）

### 25.1 Evidence Action TODO Plan

已完成：

```text
- 后端 control revise 识别“证据转行动计划 / Evidence → Action”类 prompt
- 命中后不再只保留原有单个 running todo，而是生成有序 TODO 计划
- 若 prompt 中存在 pending approvals / 待审批，第一步自动变为处理审批 gate
- 后续 TODO 覆盖读取 run/report/artifact/approval 证据、关联 finding/artifact/step、形成下一步安全动作与复测计划
- 回归测试断言 revise 后至少生成 4 个 TODO，且包含审批、证据、finding 关联步骤
```

涉及文件：

```text
internal/interfaces/daemon/agent.go
internal/interfaces/daemon/server_test.go
```

---

## 26. 验证

```text
go test ./internal/interfaces/daemon -run 'TestHTTPAgentEvidenceToActionControlPreservesRunAndApprovalContext|TestHTTPAgentSessionAndMessageAPIs' -count=1 ✅ PASS
cd web/frontend && npm run build  ✅ OK (tsc + vite)
go test ./...                    ✅ PASS
```

---

## 27. 第十五轮改动（已完成）

### 27.1 Evidence Action Approval Gate

已完成：

```text
- Evidence → Action prompt 中存在 pending approvals / 待审批 / approval gate 时，后端 next_action 不再保持 continue
- control revise 会将 next_action 设置为 approval_required，并保留 suggested_prompt
- control continue 如果传入的 suggested_prompt 仍包含审批 gate，也会保持 approval_required，避免绕过人工审批语义
- 回归测试断言 revise 和 continue 均保留 approval_required、active_run 和 suggested_prompt
```

涉及文件：

```text
internal/interfaces/daemon/agent.go
internal/interfaces/daemon/server_test.go
```

---

## 28. 验证

```text
go test ./internal/interfaces/daemon -run 'TestHTTPAgentEvidenceToActionControlPreservesRunAndApprovalContext|TestHTTPAgentActiveRunApprovalGateAndContextRefs|TestHTTPAgentSessionAndMessageAPIs' -count=1 ✅ PASS
cd web/frontend && npm run build  ✅ OK (tsc + vite)
go test ./...                    ✅ PASS
```

---

## 29. 第十六轮改动（已完成）

### 29.1 Cockpit approval gate UX

已完成：

```text
- Evidence → Action 返回 approval_required 时，Cockpit 不再提示“点击继续执行”
- 系统消息改为提示先处理审批队列 approve/deny，再恢复运行
- 底部 Continue 按钮在 approval_required 状态下禁用，并显示“需审批 / Approval”
- Next Action 区域新增 amber gate 提示，明确下一步被审批 gate 阻塞
- 前端 UI 与后端 next_action=approval_required 语义保持一致，避免误导用户绕过审批
```

涉及文件：

```text
web/frontend/src/pages/agent-cockpit.tsx
```

---

## 30. 验证

```text
cd web/frontend && npm run build  ✅ OK (tsc + vite)
go test ./...                    ✅ PASS
```

---

## 31. 第十七轮改动（已完成）

### 31.1 Approval resolution state refresh

已完成：

```text
- Cockpit approve / deny 审批请求后不再只刷新 approvals
- approve / deny / resume-approved 后会同步刷新 active scan、live events、approval queue、当前 Agent session 和 session list
- 这样 approval_required 状态、pending approval counts、active run evidence 和左侧会话摘要会立即收敛
- 避免用户处理审批后 Continue 仍因旧 session state 被禁用或显示过期 gate
```

涉及文件：

```text
web/frontend/src/pages/agent-cockpit.tsx
```

---

## 32. 验证

```text
cd web/frontend && npm run build  ✅ OK (tsc + vite)
go test ./...                    ✅ PASS
```

---

## 33. 后续建议

## 33. 第十八轮改动（已完成）

### 33.1 Approval gate resolved state convergence

已完成：

```text
- 后端在读取 active_run session 时会重新检查当前 run 的 pending approvals
- pending approvals 清空后，原 approval_required next_action 自动收敛为 continue
- 被审批 gate 阻塞的 TODO 会清理 blocked_reason，并将明确审批 gate 标记为 done，再恢复后续 pending TODO 为 running
- 普通“跟踪运行事件、证据和审批”类 TODO 不再因 approvals=0 被误判为可继续，避免终态运行被重新拉回 running
- 回归测试覆盖：pending approval → revise/continue 保持 approval_required → approve 后 session read 收敛为 continue，TODO 不再 blocked，session list pending_approvals=0
```

涉及文件：

```text
internal/interfaces/daemon/agent.go
internal/interfaces/daemon/server_test.go
```

---

## 34. 验证

```text
go test ./internal/interfaces/daemon -run 'TestHTTPAgentTerminalActiveRunSyncsOnReadAndList|TestHTTPAgentEvidenceToActionControlPreservesRunAndApprovalContext|TestHTTPAgentActiveRunApprovalGateAndContextRefs' -count=1 ✅ PASS
cd web/frontend && npm run build  ✅ OK (tsc + vite)
go test ./...                    ✅ PASS
```

---

## 35. 后续建议

如果继续推进，可做：

```text
- staged apply UI 增强：完整文件级 rebase diff 对比视图已完成；后续可继续增强底层文件写入失败时的事务化保障和更细粒度 diff 上下文定位
- multi-file diff/apply：已支持一个 change set 包含多个文件；后续可继续做更强 review/rollback 体验
- approval 面板 step deep-link、运行详情高亮定位、按 step 聚合执行链、事件/证据搜索、单 step 详情抽屉、Run Evidence 工作台、Agent Evidence Explain、Evidence Export、Evidence → Action、Evidence Action TODO Plan、Evidence Action Approval Gate、Cockpit approval gate UX 与 Approval resolution state refresh 已完成；后续可继续做虚拟列表和更强多步执行策略
- planner 失败时自动重试一次（带 temperature 调整）
- session 列表紧凑 run 状态摘要、快速筛选、跨会话搜索与置顶收藏已完成；后续可继续支持最近失败自动置顶、跨会话全文搜索索引
- ToolCallRow 展开态增加 planner_parse_error 高亮块（独立于 result JSON）
- 多 session 并行管理（当前为单 session 焦点模式）

```
