# CyberAgent 需求基线文档

版本：v0.3-confirmed  
日期：2026-05-23  
状态：MVP 需求已确认；已进入 MVP 实现/稳定化阶段

---

## 1. 本文目标

把 `docs/CyberAgent-Plan.md` 中的总体方向收敛成可执行、可验收的 MVP 需求基线。

本阶段已经确定：

```text
目标用户
首期场景
MVP 边界
交互方式
自动化程度
输出物标准
技术路线约束
```

暂不进入代码实现。

---

## 2. 已确认默认决策

为了避免需求过散，本轮已将默认值确认为 MVP 基线。

| 决策项 | 推荐默认值 | 原因 |
|---|---|---|
| 首期用户 | 个人安全研究员 / CTF 玩家 / 授权测试人员 | 最适合 CLI/TUI，反馈快，范围清晰 |
| 首期场景 | 本地靶场 + 明确授权 Web/API 目标分析 | 避免企业级复杂度，先跑通闭环 |
| 首期形态 | CLI + TUI | Agent 原生运行方式，兼顾自动化和交互 |
| Desktop | 后置，但从架构上预留 | 满足易用性，但不拖慢 MVP |
| Web | 后置 | 团队化、多用户、远程 Runner 后续再做 |
| 核心语言 | Go | 适合 CLI/TUI、并发、分发、长期运行 |
| 插件语言 | Python | 复用安全与 AI 生态 |
| 自动化程度 | 自动信息收集 + 低风险验证；高风险动作需确认 | 平衡效率、可控性和可复现性 |
| 输出标准 | Markdown + JSON + evidence 目录 | 人可读、机器可读、方便扩展 |

---

## 3. MVP 用户画像

### 3.1 主用户

```text
个人安全研究员
CTF / 靶场玩家
授权 Web/API 测试人员
```

### 3.2 典型能力水平

```text
会使用命令行
理解 URL、HTTP、端口、请求、响应、漏洞报告
能判断扫描范围是否合法
希望减少重复工具操作和结果整理成本
```

### 3.3 非首期用户

```text
企业安全管理者
普通非技术用户
大型安全团队
仅需要可视化报表的客户
```

这些用户后续通过 Desktop / Web 形态覆盖。

---

## 4. MVP 首期场景

### 4.1 核心场景

```text
用户给定一个授权 Web/API 目标，CyberAgent 自动规划并执行低风险分析，
保存证据，归一化结果，并生成可复现报告。
```

### 4.2 示例目标

```text
http://localhost:8080
http://127.0.0.1:3000
http://10.10.10.5
https://authorized-lab.example
```

### 4.3 首期不覆盖

```text
大规模互联网资产发现
未授权第三方目标扫描
自动爆破
自动横向移动
持久化
破坏性 payload
企业多用户协作
长期资产监控
```

---

## 5. MVP 输入与输出

### 5.1 输入

最小输入：

```text
target: 目标 URL
scope: 授权范围
mode: web
profile: 扫描策略，默认 safe
```

示例：

```bash
cyberagent scan web \
  --target http://localhost:8080 \
  --scope localhost,127.0.0.1 \
  --profile safe
```

### 5.2 输出

```text
runs/{job_id}/
  scope.json
  target.json
  plan.json
  events.jsonl
  commands.log
  evidence/
  findings.json
  report.md
```

### 5.3 报告内容

```text
目标概览
执行范围
执行计划
工具调用摘要
发现列表
每个 Finding 的证据
复现步骤
影响说明
修复建议
风险等级
置信度
```

---

## 6. CLI/TUI 交互边界

### 6.1 CLI 用途

```text
自动化执行
脚本集成
CI/CD
远程服务器运行
快速生成报告
```

### 6.2 TUI 用途

```text
查看执行计划
实时查看日志
浏览证据
查看 Findings
确认高风险动作
暂停 / 取消任务
导出报告
```

### 6.3 推荐首期命令形态

```bash
cyberagent scan web --target http://localhost:8080 --scope localhost
cyberagent tui
cyberagent report runs/run_001
cyberagent replay runs/run_001
cyberagent doctor
```

---

## 7. 自动化边界

### 7.1 默认允许自动执行

```text
目标可达性检查
HTTP 指纹识别
页面抓取
robots.txt / sitemap.xml 检查
链接提取
JS 文件下载与静态分析
低频目录枚举
低风险 nuclei 模板
响应头安全检查
TLS 基础检查
```

### 7.2 默认需要确认

```text
高并发目录枚举
认证绕过验证
写入型请求测试
SQL 注入验证工具
XSS 实际 payload 验证
已知 CVE exploit 验证
可能改变服务状态的请求
```

### 7.3 默认禁止

```text
密码爆破
凭证填充
持久化
横向移动
数据破坏
无范围目标扫描
绕过明确的禁止范围
```

---

## 8. 第一版能力清单

### 8.1 必须有

```text
目标与 scope 输入
scope 校验
任务模型 Job / Step
事件日志 events.jsonl
命令日志 commands.log
证据目录 evidence/
基础 HTTP 探测
链接提取
JS/API 路由提取
低风险扫描入口
Finding 归一化
Markdown 报告生成
JSON 报告数据
```

### 8.2 应该有

```text
TUI 任务视图
任务取消
超时控制
并发限制
工具缺失检查 doctor
失败步骤重试
报告模板
插件 manifest 草案
```

### 8.3 暂不做

```text
多用户登录
云端同步
远程 Runner
Desktop UI
Web Dashboard
PDF 报告
企业权限系统
自动修复代码
```

---

## 9. 已确认问题

本轮已逐项确认：

1. 首期用户锁定为：个人安全研究员 / CTF / 授权测试人员。
2. MVP 只做 Web URL；OpenAPI 深度导入、源码目录分析后置。
3. 首期形态采用 CLI + TUI；CLI 负责自动化执行，TUI v1 负责只读查看已有 run。
4. `nuclei-safe` 作为可选插件进入规划，但默认不启用，用户显式开启后才运行。
5. MVP 先做非 LLM 确定性流程；LLM 只定义 Gateway / ContextPack / 插件扩展点。
6. Desktop 后续默认优先评估 Wails + Go Core，不重写核心 Runtime。
7. 报告优先面向技术人员和授权测试记录，管理层摘要后置。
8. 插件协议第一版采用 subprocess + JSON/JSONL，不采用 Go 动态插件 ABI。

---


## 10. 工具接入方式

需求决策：**调用的外部安全工具以插件 / 适配器形式接入**，Agent Core 不在业务流程中直接写死具体工具命令。

### 10.1 接入原则

```text
所有外部工具调用必须经过 Tool Plugin Interface / Tool Adapter。
Core 负责任务调度、scope 校验、风险确认、Runner 执行和证据落盘。
插件负责任具能力声明、参数映射、输出解析和结果归一化。
```

### 10.2 插件分类

```text
Tool Plugin      包装外部工具，例如 nmap / ffuf / nuclei / httpx
Parser Plugin    把原始输出解析为结构化数据
Analyzer Plugin  基于证据生成 Finding 或风险解释
Reporter Plugin  增强 Markdown / JSON / HTML 报告
```

### 10.3 插件运行方式

MVP 推荐：

```text
内置 Go Adapter：用于核心轻量能力，例如 HTTP probe、fetch、header 检查
外部工具包装：nmap / ffuf / nuclei 等由 Tool Plugin 声明并通过 Runner 执行
外部进程插件：用于 Python 分析器、复杂解析器、LLM 增强
```

暂不优先采用 Go 动态插件 ABI，避免跨平台分发和版本兼容问题。

### 10.4 插件必须声明

```text
插件名称
插件版本
能力类型
支持的输入 schema
输出 schema
依赖的外部二进制和版本
风险等级
是否需要用户确认
产物路径
解析后的 evidence / finding 类型
```

### 10.5 安全边界

```text
插件不能绕过 Scope Guard。
高风险插件执行前必须经过 Risk Policy。
所有命令必须进入 commands.log。
所有输出必须落入 evidence/ 或 events.jsonl。
工具缺失时通过 doctor 检查并降级运行。
```


### 10.6 MVP 内置工具边界

内置工具指由 Agent Core 自带、跨平台、低风险、零外部二进制依赖的基础能力。MVP 内置能力建议只覆盖 Web/API 分析的通用原语，不内置重型扫描器。

```text
Scope Matcher        目标 / URL / IP / Hostname 范围校验
HTTP Probe           GET / HEAD / OPTIONS 基础请求、状态码、跳转链、响应时间
HTTP Fetcher         页面抓取、响应头 / body 采样、MIME 判断、大小限制
TLS Inspector        证书基础信息、过期时间、协议版本摘要
Header Checker       安全响应头检查，例如 CSP / HSTS / X-Frame-Options / CORS
Cookie Checker       Secure / HttpOnly / SameSite 等 Cookie 属性检查
Robots/Sitemap       robots.txt / sitemap.xml 抓取与 URL 提取
HTML Extractor       links / forms / scripts / meta / comments 基础提取
JS Route Extractor   轻量静态路由提取，复杂 AST 分析交给 Python 插件
API Hint Detector    从页面、JS、响应中发现 /api、GraphQL、OpenAPI 线索
TCP Port Check       少量端口连通性检查，不替代 nmap
Content Discovery    小字典、低速率目录探测，不替代 ffuf
Evidence Hasher      证据文件 hash、去重、元数据记录
Report Generator     Markdown / JSON 基础报告生成
Doctor               检查外部工具和插件依赖是否可用
```

### 10.7 不内置而作为插件接入

```text
nmap        专业端口扫描和服务识别
ffuf        高性能目录 / 参数枚举
nuclei      模板化漏洞扫描
httpx       大规模 HTTP 探测
katana      爬虫与链接发现
playwright  浏览器自动化、截图、前端运行态分析
semgrep     源码安全分析
gitleaks    secret 检测
trufflehog  secret 检测
zap         DAST 代理扫描
sqlmap      SQL 注入验证，默认高风险确认
```


### 10.8 Agent 基础工具层

除安全分析工具外，CyberAgent 还需要内置一组 Agent Runtime 基础工具。它们不直接承担漏洞扫描，而是保证计划、执行、状态、证据、插件和多端交互可复用。

MVP 必须内置：

```text
Config Manager       读取项目配置、用户配置、扫描 profile、默认策略
Run Manager          创建 runs/{job_id} 工作区、路径规范、清理策略
State Store          Job / Plan / Step 状态快照，MVP 使用 bbolt 嵌入式 KV，SQLite 后置评估
Event Bus            统一事件流，写入 events.jsonl，并供 TUI / 后续 API 订阅
Logger               普通日志、调试日志、错误日志，避免污染 evidence
Command Executor     受控命令执行封装，统一 timeout / env / cwd / stdout / stderr
Command Recorder     所有外部命令写入 commands.log，支持复现和审计
Policy Engine        Scope Guard、Risk Policy、确认策略、禁止动作策略
Planner Runtime      Plan / Step 生成、更新、跳过、重排，不直接绑定具体 UI
Tool Registry        安全工具、基础工具、插件能力统一注册与查询
Rate Limiter         请求频率、并发数、per-host 限制
Retry/Timeout        步骤级超时、失败重试、取消任务
Plugin Manager       插件发现、manifest 读取、版本检查、启用/禁用
Dependency Doctor    检查外部工具、Python 环境、插件依赖是否可用
Artifact Store       原始输出、截图、HTML、JSONL、工具结果统一落盘
Evidence Indexer     证据元数据、hash、来源 step、关联 finding
Redactor             对 token、cookie、authorization header 等敏感字段做脱敏策略
Template Renderer    Markdown / JSON 报告模板渲染
Cache/Dedup          URL、响应、证据、finding 去重，避免重复请求和重复报告
LLM Gateway          可选模型调用抽象；MVP 可只定义接口，不作为核心依赖
Prompt/Context Store 可选提示词、摘要、上下文包管理；避免 LLM 直接读取全部原始证据
```

后续可内置，但不进入 MVP：

```text
Credential Vault     授权测试凭据、cookie、API token 的本地安全存储
Browser Controller   浏览器会话管理；具体 Playwright 仍建议作为插件/组件接入
Notification         扫描完成、失败、需要确认等通知
Update Manager       插件索引、模板、规则集更新
Metrics/Telemetry    本地性能指标；默认不上传
Remote Sync          Desktop/Web/Remote Runner 场景下的任务同步
```

### 10.9 基础工具边界原则

```text
Agent 基础工具属于 Core Runtime，不属于安全扫描插件。
基础工具必须稳定、跨平台、可测试，尽量零外部依赖。
外部命令只能通过 Command Executor / Runner 执行，不能由插件绕过。
所有基础工具都要服务于可复现、可审计、可取消、可扩展。
```

---

## 11. 需求基线完成状态

需求基线已完成，并已分解到以下规划文档：

```text
docs/CyberAgent-Plan.md
docs/CyberAgent-Agent-Architecture.md
docs/CyberAgent-Core-Contracts.md
docs/CyberAgent-MVP-Spec.md
docs/CyberAgent-Implementation-Blueprint.md
docs/CyberAgent-Roadmap.md
docs/CyberAgent-Milestones.md
docs/CyberAgent-Task-Breakdown.md
docs/CyberAgent-Test-Plan.md
docs/CyberAgent-Release-Plan.md
```

进入实现前的闸门：

```text
只有用户明确说“开始实现”或给出等价指令后，才继续扩展代码。
当前已有代码不回滚，但本轮规划任务不再新增实现。
```
