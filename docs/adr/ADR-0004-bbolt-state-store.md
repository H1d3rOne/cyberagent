# ADR-0004: MVP State Store 使用 bbolt + JSONL + 文件系统

状态：Accepted  
日期：2026-05-23

## 决策

MVP 使用：

```text
bbolt 保存当前状态快照和轻量索引
events.jsonl 保存 append-only 事件流
文件系统保存 artifacts/evidence/reports
```

## 原因

```text
比纯 JSON 文件更适合频繁状态更新。
比 SQLite 更轻，不需要提前设计关系表。
适合 Go 单机 CLI/TUI MVP。
```

## 影响

```text
SQLite 后置。
未来如果需要复杂查询、Web 多任务过滤、报表检索，再评估 SQLite/Postgres。
```
