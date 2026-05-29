# CyberAgent Frontend Guidelines

版本：v0.1  
日期：2026-05-25  
状态：草案

## 1. 设计原则

1. **功能优先**：界面服务于安全运维与 Agent 操作，不做营销化表达。  
2. **克制统一**：优先统一 surface、heading、spacing、interaction，不追求视觉花样。  
3. **可解释**：每个页面区域都要能回答“当前状态是什么 / 下一步做什么”。  
4. **Agent 原生**：聊天、任务、工具调用、审批、workspace 是一等公民，不是附加模块。

## 2. 页面结构规范

### 2.1 标题体系

- 页面主标题：`PageTitle`
- 区块标题：`SectionTitle`
- 小标题/面板标题：`text-sm font-semibold`，不另起新体系
- 不允许同一页面出现 3 种以上标题风格

### 2.2 表面体系

- 外层区块：`surface-panel`
- 内层信息块：`subtle-surface`
- 列表项：`list-item`
- 状态高亮块（active/selected）可保留状态色，但不能替代 surface 体系

### 2.3 间距体系

- 页面主间距：`space-y-6`
- section 内间距：`p-4 sm:p-5`
- section 内子块间距：`space-y-4` 或 `gap-3/4`
- 标题与内容之间：默认 `mb-3`

## 3. 交互规范

- 可点击列表项必须有统一 hover 态
- hover 反馈统一使用 `list-item` 或 `interactive-surface`
- 禁止部分列表有 hover、部分没有
- 动效时间统一在 150~200ms，避免过度动画
- 支持 `prefers-reduced-motion`

## 4. 文案规范

- UI 文案要像运维台，不是营销页
- 用短句说明：状态、范围、风险、下一步
- 标题优先说明“这是什么”，不是“感觉如何”

## 5. Agent Cockpit 规范

Agent 页面遵循“三件事原则”：

1. **当前任务是什么**
2. **Agent 正在做什么**
3. **用户下一步做什么**

### 5.1 Chat 区

- 必须保留用户消息、Agent 消息、系统消息分层
- 工具调用和 reasoning 可折叠，但不能隐藏关键状态

### 5.2 Task / Todo 区

- 当前任务必须可见
- pending/running/done/blocked 状态必须清晰

### 5.3 Workspace 区

- 命令执行结果必须展示：
  - command
  - exit_code
  - stdout/stderr
  - duration
  - workdir

## 6. 可观测性规范

- 错误信息靠近操作区域展示
- 状态流转必须有可读 label，不只靠颜色
- 关键状态建议图标+文字同时存在

## 7. 允许与禁止

### 允许

- 使用统一 token class
- 使用统一 section/header 组件
- 在状态态使用强调色

### 禁止

- 每个页面手搓新 surface 样式
- 同类列表出现不同 hover 行为
- 标题层级随意混搭
- 用装饰效果替代信息层级

## 8. 后续演进

1. 将更多 settings 表单迁移到 `subtle-surface`
2. 为 workspace 增加“只读/编辑”状态规范
3. 增加 report/findings 的视觉层级模板
4. 增加 dark-mode 下的 contrast QA checklist
