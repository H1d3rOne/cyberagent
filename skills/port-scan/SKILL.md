# Port Scan

## Intent

- `port_scan`
- `open_ports`
- `tcp_connect_probe`

## Goal

使用有界 TCP connect 探测识别开放 TCP 端口；不替代 nmap 全量服务识别、UDP 扫描或超大范围扫描。

## Required tools

- `ports.scan.safe`

## Optional tools

- `plugins.list`
- MCP: `preset-nmap`

## Risk

- `safe_scan`

## Examples

- 扫描 `example.com` 开放端口 `ports=80,443,8080`

