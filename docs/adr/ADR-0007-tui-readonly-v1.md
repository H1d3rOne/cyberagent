# ADR-0007: TUI v1 只读已有 run

状态：Accepted  
日期：2026-05-23

## 决策

TUI 第一版只作为 run viewer，读取已有 runs/{job_id}。

## 原因

```text
先稳定 Core 和 CLI 闭环。
实时控制、审批、创建任务会增加复杂度。
只读 TUI 已能提供 Plan/Events/Evidence/Findings/Report 浏览价值。
```

## 影响

TUI v1 不负责：

```text
创建任务
修改参数
暂停/恢复
HITL approval
实时控制
```
