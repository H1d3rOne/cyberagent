# CyberAgent Design Tokens

版本：v0.1  
日期：2026-05-25  
状态：草案

## 1. 目标

将 CyberAgent 前端的表面、标题、间距、列表交互收敛为可复用的 token，避免页面间出现样式漂移。

## 2. 表面层 Token

```text
surface.panel
  border: rgba(255,255,255,0.08)
  background: rgba(255,255,255,0.03)
  radius: 1.75rem

surface.subtle
  border: rgba(255,255,255,0.10)
  background: rgba(0,0,0,0.20)

surface.list-item
  border: rgba(255,255,255,0.10)
  background: rgba(0,0,0,0.20)
  hover.border: rgba(59,130,246,0.30)
  hover.background: rgba(59,130,246,0.05)
  motion: 180ms cubic-bezier(0.2,0,0,1)
```

## 3. 标题层级 Token

```text
heading.page
  size: 2xl -> ~2.15rem
  weight: semibold
  tracking: tight

heading.section
  size: sm
  weight: semibold
  spacing.bottom: 12px (mb-3)

heading.panel
  size: sm
  weight: semibold
```

## 4. 间距 Token

```text
section.padding.default: p-4 sm:p-5
section.gap.default: space-y-4
page.gap.default: space-y-6
grid.gap.default: gap-3 / gap-4
```

## 5. 交互 Token

```text
interactive.surface
  motion: 180ms
  easing: cubic-bezier(0.2,0,0,1)
  hover.transform: translateY(-1px)
  hover.shadow: 0 10px 25px rgba(0,0,0,0.20)

list.item
  motion: 180ms
  hover.border: primary/30
  hover.background: primary/5
```

## 6. 当前实现映射

```text
surface.panel -> .surface-panel
surface.subtle -> .subtle-surface
surface.list-item -> .list-item
heading.section -> SectionTitle
heading.page -> PageTitle
section.padding.default -> Section
```

## 7. 后续建议

1. 把 settings 内层表单块统一到 `subtle-surface`
2. 统一所有页面 `mb-3` 标题间距为 token 化 class
3. 继续减少页面内 inline surface class
## 8. 当前剩余 inline 用法

```text
1. cockpit chat footer: border-t border-white/10 bg-black/20
2. new-scan mode button active state: border-primary/35 bg-primary/10
3. settings language active state: border-primary/35 bg-primary/10
4. dashboard connection badge in header (保留为独立语义组件)
```

这些属于状态驱动样式或 UI 局部强调，不在本轮全量迁移范围内。
