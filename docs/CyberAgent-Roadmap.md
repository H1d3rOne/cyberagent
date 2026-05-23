# CyberAgent Roadmap

版本：v0.1
日期：2026-05-24
状态：v0.1.1 本地 RC 准备完成，v0.1.2 Local Daemon / Web API 进入计划完成状态

---

## 1. 路线图目标

本文定义 CyberAgent 从规划阶段到 MVP、v1、Desktop/Web 扩展阶段的整体路线图。

路线图原则：

```text
先完成 Agent Core 闭环，再扩展工具。
先 CLI/TUI，再 Desktop/Web。
先本地单机 Runner，再 Remote Runner。
先确定性规则，再 LLM 增强。
先证据链和可复现，再追求自动化深度。
```

---

## 2. 阶段总览

```text
Phase 0：需求与架构规划（完成）
Phase 1：MVP Core Runtime（完成）
Phase 2：内置 Web/API 分析闭环（完成）
Phase 3：插件系统与可选工具（MVP 完成）
Phase 4：TUI Run Viewer（完成）
Phase 5：MVP Stabilization（v0.1.0 本地完成）
Phase 5.1：v0.1.1 Stabilization / HITL / Plugin Hardening（本地 RC 完成）
Phase 5.2：v0.1.2 Local Daemon / Web API Foundation（当前计划完成，待实现）
Phase 6：LLM 增强
Phase 7：Desktop 本地工作台
Phase 8：Local API / Daemon 稳定版
Phase 9：Web / Remote Runner
Phase 10：平台化与生态扩展
```

---

## 3. Phase 0：需求与架构规划

状态：完成。

目标：

```text
明确产品定位
明确用户画像
明确技术路线
明确 MVP 范围
明确核心契约
明确实现蓝图
明确风险边界
```

产出文档：

```text
docs/CyberAgent-Plan.md
docs/CyberAgent-Requirements.md
docs/CyberAgent-Agent-Architecture.md
docs/CyberAgent-Core-Contracts.md
docs/CyberAgent-MVP-Spec.md
docs/CyberAgent-Implementation-Blueprint.md
```

完成标准：

```text
核心方向不再频繁变更。
MVP 输入输出明确。
核心对象明确。
技术选型明确。
默认不继续扩大 MVP 范围。
```

---

## 4. Phase 1：MVP Core Runtime

目标：

```text
完成 Agent 能运行任务的最小 Runtime。
```

范围：

```text
Core Contracts
Run Manager
bbolt State Store
Event JSONL Store
Job / Step 状态机
Scope Guard
Risk Policy
Command Recorder
基础 CLI command wiring
```

不包含：

```text
复杂扫描能力
TUI 实时控制
插件执行
LLM
Desktop
Web
```

验收标准：

```text
可以创建 Job workspace。
可以保存 Job / Plan / Step 状态。
可以写入 events.jsonl。
可以执行策略判断。
没有 scope 时拒绝主动扫描。
```

---

## 5. Phase 2：内置 Web/API 分析闭环

目标：

```text
不依赖外部工具，完成 safe profile 的最小 Web 分析闭环。
```

范围：

```text
HTTP Probe
HTTP Fetcher
Header Checker
Cookie Checker
Robots/Sitemap Fetcher
HTML Extractor
JS Route Extractor Light
API Hint Detector
Content Discovery Small
Evidence Indexer
Finding Normalizer
Markdown/JSON Reporter
```

验收命令：

```bash
cyberagent scan web --target http://localhost:8080 --scope localhost --profile safe
```

应生成：

```text
runs/{job_id}/events.jsonl
runs/{job_id}/commands.log
runs/{job_id}/artifacts/
runs/{job_id}/evidence/index.json
runs/{job_id}/findings.json
runs/{job_id}/reports/report.md
```

---

## 6. Phase 3：插件系统与可选工具

目标：

```text
通过插件接入外部安全工具，但不让插件绕过 Core 规则。
```

范围：

```text
Plugin Manifest
Plugin Manager
Tool Registry
Dependency Doctor
subprocess + JSON/JSONL protocol
Command Executor
nuclei-safe，可选且默认不启用
ffuf-low-rate，可选
python-js-route-analyzer，可选
```

验收标准：

```text
插件可发现。
插件依赖可检查。
插件缺失时可降级。
插件输出能转为 Artifact/Evidence/Finding。
插件不能绕过 Scope Guard / Risk Policy / Command Recorder。
```

---

## 7. Phase 4：TUI Run Viewer

目标：

```text
提供第一版 TUI，只读打开已有 run。
```

范围：

```text
Plan View
Events View
Evidence View
Findings View
Report View
```

不包含：

```text
创建任务
实时控制
审批高风险步骤
修改参数
暂停/恢复任务
```

验收命令：

```bash
cyberagent tui runs/{job_id}
```

---

## 8. Phase 5：MVP Stabilization

目标：

```text
让 MVP 可以被稳定试用和复现。
```

范围：

```text
错误处理
日志清理
测试补齐
文档补齐
示例靶场
报告模板优化
doctor 完善
性能基础限制
```

验收标准：

```text
go test ./... 通过。
核心安全边界测试通过。
示例目标可完整跑通。
报告可被人工复现。
```

状态：v0.1.0 本地完成。

---

## 8.1 Phase 5.1：v0.1.1 Stabilization / HITL / Plugin Hardening

目标：

```text
在不扩大默认扫描强度的前提下，增强工程稳定性、人工审批、插件边界和 run 复现能力。
```

范围：

```text
CI / release hygiene
HITL Approval MVP
Plugin timeout/output/schema/error taxonomy
run metadata / effective config / plugin snapshot
CLI/TUI/report UX polish
smoke fixture expansion
```

非目标：

```text
Desktop/Wails
Web 多用户平台
Remote Runner
默认启用外部扫描插件
高风险自动利用链
```

详细计划：`docs/CyberAgent-v0.1.1-Plan.md`

---

## 8.2 Phase 5.2：v0.1.2 Local Daemon / Web API Foundation

目标：

```text
把 CLI/TUI-first 的 Agent Core 抽象成可复用的本地服务层，并提供 Local HTTP Daemon。
```

范围：

```text
Core Service API
cyberagent daemon
local HTTP JSON API
run list / run detail / report / artifacts / events
scan web async job start/status/cancel
approvals list / approve / deny / resume-approved
plugins doctor / version / health / readiness
local token auth
API contract / smoke test
```

非目标：

```text
Desktop/Wails
完整 Web UI
多用户 SaaS
Remote Runner
公网监听 daemon
SQLite 强制迁移
默认启用外部扫描插件
```

详细计划：`docs/CyberAgent-v0.1.2-Plan.md`

---

## 9. Phase 6：LLM 增强

目标：

```text
在不改变 Core 决策边界的前提下加入 LLM 增强。
```

范围：

```text
LLM Gateway
Context Builder
Reporter Plugin
Analyzer Plugin
Reflection Assistant，可选
```

原则：

```text
LLM 不执行工具。
LLM 不扩大 scope。
LLM 不自动批准高风险动作。
LLM 只读取 ContextPack。
LLM 输出必须 schema validate。
```

---

## 10. Phase 7：Desktop 本地工作台

目标：

```text
降低非命令行用户使用门槛。
```

候选技术：

```text
Wails + Go Core
```

范围：

```text
任务创建
历史 run 浏览
Finding 浏览
Evidence 浏览
报告导出
配置管理
```

---

## 11. Phase 8：Local API / Daemon 稳定版

目标：

```text
在 v0.1.2 foundation 基础上，稳定本地 API、客户端 SDK、事件流和 UI 集成能力。
```

范围：

```text
Job API
Event Stream API
Report API
Evidence API
Plugin API
Local auth token
API client
OpenAPI/generated client
Desktop/Web integration hardening
```

---

## 12. Phase 9：Web / Remote Runner

目标：

```text
支持团队化、多任务、远程执行。
```

范围：

```text
Web Control Plane
Remote Runner
Runner registration
Task dispatch
Object storage
权限系统
审计日志
```

---

## 13. Phase 10：平台化与生态扩展

目标：

```text
形成可扩展的安全 Agent 平台。
```

范围：

```text
插件索引
模板更新
报告模板市场
团队协作
长期 Target Memory
资产图谱
CWE/OWASP 映射
Issue Tracker 集成
```

---

## 14. 阶段推进原则

```text
每个 Phase 只在上一阶段可验收后开始。
任何新功能不得破坏 Scope Guard / Evidence Chain / Event Log。
MVP 期间不引入多用户和云端依赖。
发现需求膨胀时优先推迟到后续 Phase。
```
