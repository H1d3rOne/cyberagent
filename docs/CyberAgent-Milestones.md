# CyberAgent Milestones

版本：v0.1  
日期：2026-05-23  
状态：里程碑计划完成

---

## 1. 里程碑定义

本文定义 CyberAgent MVP 前后的可验收里程碑。

每个里程碑必须满足：

```text
目标明确
范围明确
验收标准明确
非目标明确
```

---

## M0：Planning Complete

状态：当前目标。

目标：

```text
完成进入实现前所需的全部规划文档。
```

验收：

```text
需求文档完成
架构文档完成
核心契约完成
MVP Spec 完成
实现蓝图完成
路线图完成
任务拆解完成
ADR 完成
风险计划完成
测试计划完成
发布计划完成
文档索引完成
```

非目标：

```text
不要求实现完整功能。
不要求 CLI scan 可运行。
```

---

## M1：Project Skeleton Ready

目标：

```text
项目骨架可以编译，基础 CLI 可运行。
```

验收：

```text
go test ./... 通过
cyberagent version 可用
cyberagent doctor 可用
目录结构符合 Implementation Blueprint
```

---

## M2：Core Runtime Ready

目标：

```text
Agent Runtime 基础能力可用。
```

验收：

```text
Core Contracts 可序列化
Run workspace 可创建
bbolt State Store 可保存/读取 Job/Plan/Step
Event Store 可 append/read
Job/Step 状态机可运行
Scope Guard 可拒绝越界目标
Risk Policy 可输出 allow/require_approval/deny
```

---

## M3：Safe Profile Plan Ready

目标：

```text
能为 Web URL 生成 safe profile 执行计划。
```

验收：

```text
输入 target/scope/profile 后生成 Plan
Plan 包含 safe profile Step
每个 Step 有 risk/tool/input/dependency
外部插件默认不启用
无 scope 不生成主动网络 Step
```

---

## M4：Builtin Web Analysis Ready

目标：

```text
内置 Web 分析能力形成闭环。
```

验收：

```text
HTTP Probe 可用
HTTP Fetch 可用
Header Checker 可用
Cookie Checker 可用
HTML Extractor 可用
API Hint Detector 可用
Evidence 可生成
Finding 可生成
Report 可生成
```

---

## M5：CLI Safe Scan Ready

目标：

```text
CLI 可以完整执行一次 safe scan。
```

验收命令：

```bash
cyberagent scan web --target http://localhost:8080 --scope localhost --profile safe
```

验收输出：

```text
runs/{job_id}/target.json
runs/{job_id}/scope.json
runs/{job_id}/job.json
runs/{job_id}/plan.json
runs/{job_id}/events.jsonl
runs/{job_id}/evidence/index.json
runs/{job_id}/findings.json
runs/{job_id}/reports/report.md
```

---

## M6：Plugin MVP Ready

目标：

```text
可选插件系统可用。
```

验收：

```text
Plugin Manifest 可加载
Plugin Doctor 可检查依赖
nuclei-safe 默认不启用
用户显式启用时才运行 nuclei-safe
插件输出可成为 Artifact/Evidence/Finding
插件缺失时 scan 不失败
```

---

## M7：TUI Run Viewer Ready

目标：

```text
TUI 可以只读打开已有 run。
```

验收：

```text
cyberagent tui runs/{job_id} 可启动
可查看 Plan
可查看 Events
可查看 Evidence
可查看 Findings
可查看 Report 路径/摘要
```

---

## M8：MVP Release Candidate

目标：

```text
MVP 可试用、可复现、可测试。
```

验收：

```text
全部 MVP 测试通过
示例靶场跑通
README 快速开始可用
报告结构稳定
已知限制写入文档
无破坏性默认行为
```

---

## M9：MVP v0.1 Release

目标：

```text
发布第一个可试用版本。
```

验收：

```text
tag v0.1.0
release notes 完成
二进制构建流程完成
文档索引完成
示例 run 完成
```
