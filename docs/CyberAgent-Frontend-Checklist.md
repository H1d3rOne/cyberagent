# CyberAgent Frontend Checklist

版本：v0.1  
日期：2026-05-25  
状态：草案

## 1. 结构检查

- [ ] 页面使用 `PageTitle` 作为主标题
- [ ] 区块使用 `SectionTitle` / `SectionHeader`
- [ ] 没有出现超过 3 种标题风格
- [ ] 页面主结构清晰：任务区 / 状态区 / 上下文区

## 2. 表面层检查

- [ ] 外层 section 使用 `surface-panel`
- [ ] 内层信息块使用 `subtle-surface`
- [ ] 列表项使用 `list-item`
- [ ] 没有新增随意手搓的 inline surface 样式

## 3. 间距检查

- [ ] section 使用统一 `p-4 sm:p-5`
- [ ] 页面主间距统一 `space-y-6`
- [ ] 标题与内容之间保持 `mb-3` 或统一 spacing token
- [ ] 子块间距一致，不出现明显跳变

## 4. 交互检查

- [ ] 可点击列表项都有 hover 反馈
- [ ] hover 行为与同类元素一致
- [ ] disabled 状态有明确视觉表达
- [ ] loading 状态有提示
- [ ] 动效时长保持 150~200ms
- [ ] 支持 `prefers-reduced-motion`

## 5. Agent 体验检查

- [ ] 用户能快速知道当前任务
- [ ] 用户能快速知道 Agent 正在做什么
- [ ] 用户能快速知道下一步做什么
- [ ] 工具调用 / reasoning / approval 状态可见
- [ ] workspace 命令结果展示完整

## 6. 可读性检查

- [ ] 状态不仅靠颜色表达
- [ ] 关键状态有图标 + 文字
- [ ] 错误信息靠近操作位置
- [ ] 数值/时间/ID 有足够对比度

## 7. 前置提交检查

- [ ] 没有引入新的未定义 surface 样式
- [ ] 没有同类组件出现不同 hover 行为
- [ ] 没有重复实现 section header
- [ ] 没有破坏 token 文档约定
- [ ] 新增页面遵循 `Frontend Guidelines` 和 `Design Tokens`

## 8. QA 检查

- [ ] 小屏布局正常
- [ ] 大屏布局正常
- [ ] 侧栏 / sticky 区域正常
- [ ] 长内容可滚动
- [ ] 关键页面 build 无 TS/样式回归

## 9. 推荐复查顺序

1. 先看页面结构
2. 再看 surface/heading/spacing
3. 再看交互一致性
4. 最后看 agent 可观测性

## 10. 典型反模式

- 每个页面自造 section 样式
- 列表项 hover 不一致
- 标题层级随意混搭
- 用装饰效果替代信息层级
- Agent 页面只有聊天，没有任务与状态上下文
