# CyberAgent Agent Cockpit Smoke Test

版本：v0.1  
日期：2026-05-28  
状态：本地通过

---

## 1. 目标

验证 Web 端 Agent Cockpit 符合当前项目定位：以聊天驱动 Agent 执行，同时保留运行簿、执行轨迹、能力目录和工作区操作面板。

---

## 2. 启动方式

```bash
npm run build --prefix web/frontend
go build -o .tmp/cyberagent-dev ./cmd/cyberagent

PATH="$PWD/.tmp/noopbin:$PATH" .tmp/cyberagent-dev daemon --listen 127.0.0.1:8787
PATH="$PWD/.tmp/noopbin:$PATH" .tmp/cyberagent-dev dashboard --addr 127.0.0.1:8080 --daemon-url http://127.0.0.1:8787
```

打开：

```text
http://127.0.0.1:8080/agent
```

---

## 3. 必验点

```text
[x] 页面加载最新构建产物
[x] 左侧展示 Agent 会话列表、搜索、筛选、置顶
[x] 中间为 Agent 聊天主工作区
[x] 聊天输入框支持自然语言任务、快捷指令和 Slash Commands
[x] 右侧 Inspector 采用四个焦点页签：运行簿 / 轨迹 / 能力 / 工作区
[x] 运行簿展示 session 状态、TODO、memory、reasoning 和 next_action
[x] 轨迹展示 timeline / next_action / tool metadata
[x] 能力展示 capabilities、内置工具与 auto/gate 风险标记
[x] 工作区展示命令执行、文件浏览、最近扫描
[x] Inspector 当前页签写入 localStorage，可在刷新后保持
[x] daemon 在线状态可见
```

---

## 4. Playwright 验证

使用 Codex Playwright wrapper：

```bash
export CODEX_HOME="${CODEX_HOME:-$HOME/.codex}"
export PWCLI="$CODEX_HOME/skills/playwright/scripts/playwright_cli.sh"

"$PWCLI" open http://127.0.0.1:8080/agent
"$PWCLI" snapshot
"$PWCLI" click <轨迹按钮 ref>
"$PWCLI" snapshot
"$PWCLI" click <能力按钮 ref>
"$PWCLI" snapshot
"$PWCLI" click <工作区按钮 ref>
"$PWCLI" snapshot
```

最近一次本地验证确认：

```text
Agent Cockpit 页面可访问
页面标题：CyberAgent Dashboard
左侧状态：在线
右侧 Inspector 页签：运行簿 / 轨迹 / 能力 / 工作区
能力页签内置工具数量：17
工作区页签可见 Agent 工作区、工作区文件、最近扫描
```

---

## 5. 失败排查

```text
页面 404/旧资产：
  重新执行 npm run build --prefix web/frontend，并重建 dashboard 二进制。

daemon 离线：
  检查 127.0.0.1:8787 是否监听；bundled dashboard 会通过 /api/local/daemon-config 注入 URL/token。

聊天不能回复：
  先检查设置页 AI provider；未配置模型时仍应支持本地规则工具、continue/stop/new_task、workspace 命令等确定性分支。

Inspector 页签不符合预期：
  清理 localStorage key cyberagent_cockpit_inspector_tab_v1，或点击右侧页签切换。
```
