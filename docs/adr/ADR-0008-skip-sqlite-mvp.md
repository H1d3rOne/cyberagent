# ADR-0008: MVP 不使用 SQLite

状态：Accepted  
日期：2026-05-23

## 决策

MVP 不引入 SQLite。

## 原因

```text
MVP 单机单任务为主。
状态更适合 KV 快照。
复杂关系查询暂时不是核心需求。
```

## 影响

```text
使用 bbolt + JSONL + filesystem。
Desktop/Web 阶段再评估 SQLite。
```
