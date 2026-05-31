# JS Route Analysis

## Intent

- `js_route_analysis`
- `frontend_routes`
- `api_endpoint_extraction`

## Goal

提取前端 JS bundle 中的路由、API endpoint 和潜在接口。本阶段不依赖 Browser / Playwright。

## Required tools

- `js.routes.analyze`

## Optional tools

- `workspace.search`
- `workspace.files.read`
- `http.request`

## Risk

- `read_only`

## Fallback

- 将 JS 文件放入工作区后，用 workspace 搜索接口路径。

