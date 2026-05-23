# ADR-0003: 外部安全工具通过插件接入

状态：Accepted  
日期：2026-05-23

## 决策

nmap、ffuf、nuclei、Playwright、ZAP、sqlmap 等外部安全工具不写死在 Core 中，统一通过 Tool Plugin / Adapter 接入。

## 原因

```text
工具依赖重、更新快、参数差异大。
插件化便于升级、禁用、降级和风险控制。
Core 可以保持稳定。
```

## 影响

```text
插件必须声明 manifest、依赖、输入输出 schema、风险等级。
插件不能绕过 Runner / Scope Guard / Risk Policy。
```
