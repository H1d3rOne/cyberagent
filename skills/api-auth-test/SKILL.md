# API Auth / Authorization Test

## Intent

- `api_auth_test`
- `authorization`
- `authentication`
- `idor`

## Goal

围绕 Auth Profile、角色、token 和 endpoint 做认证/鉴权验证。需要明确 endpoint、method、认证上下文和授权 scope。

## Required tools

- `api.auth.test`

## Optional tools

- `http.request`

## Risk

- `advanced`

## Fallback

- 使用已配置 Auth Profile 对具体 API endpoint 发起单次授权 HTTP 请求。

