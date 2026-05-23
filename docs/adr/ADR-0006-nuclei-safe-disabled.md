# ADR-0006: nuclei-safe 默认不启用

状态：Accepted  
日期：2026-05-23

## 决策

nuclei-safe 作为可选插件进入 MVP，但默认不运行。

## 原因

```text
nuclei 是外部工具，模板行为需要治理。
即使低风险模板也可能增加请求量和误报。
默认内置流程应保持轻量可控。
```

## 影响

用户需显式启用：

```bash
cyberagent scan web --target http://localhost:8080 --scope localhost --use-plugin nuclei-safe
```
