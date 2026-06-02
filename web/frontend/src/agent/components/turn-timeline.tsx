import { Bot, CheckCircle2, Clock, Loader2, MessageSquare, ShieldCheck, Sparkles, Wrench } from 'lucide-react'
import { GlassAlert, GlassCard, StatusPill } from '@/components/glass'
import { Button } from '@/components/ui/button'
import { text, type Language } from '@/lib/console'
import { cn } from '@/lib/utils'
import type { AgentCockpitState, AgentTurnView, OptimisticMessage } from '@/agent/state/store'
import { commandFromToolCall, formatTime, messageText, parseCommandSummary, roleLabel, safeJson, statusLabel, statusPillClass, toolDotClass } from '@/agent/utils'
import { OutputBlock } from './shared'

export function TurnTimeline({
  state,
  language,
  disabled,
  onSelectTool,
  onFillPrompt,
  onExecutePrompt,
  onCreateSession,
}: {
  state: AgentCockpitState
  language: Language
  disabled?: boolean
  onSelectTool: (toolCallId: string) => void
  onFillPrompt: (prompt: string) => void
  onExecutePrompt: (prompt: string) => void
  onCreateSession: () => void
}) {
  if (!state.session && !state.loadingSession) {
    return <EmptyAgentState language={language} onCreate={onCreateSession} />
  }
  if (state.loadingSession && !state.session) {
    return (
      <div className="grid h-full place-items-center text-sm text-muted-foreground">
        <span className="inline-flex items-center gap-2"><Loader2 className="h-4 w-4 animate-spin" />{text(language, '加载中', 'Loading')}</span>
      </div>
    )
  }

  const turns = state.turns
  return (
    <div className="mx-auto flex min-h-full w-full max-w-5xl flex-col justify-end">
      <div className="space-y-4">
        {turns.map((turn) => (
          <TurnGroup
            key={turn.id}
            turn={turn}
            state={state}
            language={language}
            disabled={Boolean(disabled)}
            onSelectTool={onSelectTool}
            onFillPrompt={onFillPrompt}
            onExecutePrompt={onExecutePrompt}
          />
        ))}
        {state.optimisticMessages.map((message) => (
          <OptimisticBubble key={message.id} message={message} language={language} />
        ))}
        {state.sending && !state.optimisticMessages.some((item) => item.status === 'sending') && (
          <div className="flex justify-start">
            <GlassAlert tone="muted" className="inline-flex items-center gap-2 px-3 py-2 text-xs">
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
              {text(language, 'Agent 正在处理...', 'Agent is working...')}
            </GlassAlert>
          </div>
        )}
      </div>
    </div>
  )
}

function EmptyAgentState({ language, onCreate }: { language: Language; onCreate: () => void }) {
  return (
    <div className="grid h-full place-items-center">
      <GlassCard variant="panel" className="max-w-md p-6 text-center">
        <div className="mx-auto mb-5 grid h-16 w-16 place-items-center rounded-3xl border border-primary/25 bg-primary/10">
          <Bot className="h-8 w-8 text-primary" />
        </div>
        <h1 className="text-xl font-semibold">{text(language, '事件驱动 Agent Cockpit', 'Event-driven Agent Cockpit')}</h1>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">
          {text(language, '按 Turn 聚合模型 token、推理、工具调用、审批与 TODO 更新。创建会话后即可开始。', 'Turns group model tokens, reasoning, tools, approvals and TODO updates. Create a session to start.')}
        </p>
        <Button type="button" onClick={onCreate} className="mt-5 rounded-2xl">
          <Sparkles className="mr-2 h-4 w-4" />
          {text(language, '创建会话', 'Create session')}
        </Button>
      </GlassCard>
    </div>
  )
}

function TurnGroup({
  turn,
  state,
  language,
  disabled,
  onSelectTool,
  onFillPrompt,
  onExecutePrompt,
}: {
  turn: AgentTurnView
  state: AgentCockpitState
  language: Language
  disabled: boolean
  onSelectTool: (toolCallId: string) => void
  onFillPrompt: (prompt: string) => void
  onExecutePrompt: (prompt: string) => void
}) {
  const tools = state.session ? turn.toolCallIds.map((id) => findTool(state, id)).filter(Boolean) : []
  const todos = state.session ? turn.todoIds.map((id) => state.session?.todos?.find((todo) => todo.id === id)).filter(Boolean) : []
  const memoryEvents = state.events.filter((event) => turn.eventIds.includes(event.id) && event.type === 'memory_compacted')
  const approvalEvents = state.events.filter((event) => turn.eventIds.includes(event.id) && (event.type === 'approval_required' || event.type === 'approval_resolved'))

  return (
    <GlassCard as="section" variant="panel" className="relative p-3 shadow-[0_18px_50px_rgba(0,0,0,.16)]">
      <div className="mb-3 flex flex-wrap items-center gap-2 border-b border-white/10 pb-2">
        <span className={cn('grid h-7 w-7 place-items-center rounded-full border', statusPillClass(turn.status))}>
          {turn.status === 'running' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <CheckCircle2 className="h-3.5 w-3.5" />}
        </span>
        <div className="min-w-[160px] flex-1">
          <div className="line-clamp-1 text-sm font-medium">{turn.title || text(language, 'Turn', 'Turn')}</div>
          <div className="font-mono text-[10px] text-muted-foreground">{formatTime(turn.startedAt)} · {statusLabel(turn.status, language)}</div>
        </div>
        <div className="flex flex-wrap gap-1.5 font-mono text-[10px] text-muted-foreground">
          <StatusPill tone="muted">messages {turn.messages.length}</StatusPill>
          <StatusPill tone="muted">tools {tools.length}</StatusPill>
          <StatusPill tone="muted">todos {todos.length}</StatusPill>
        </div>
      </div>

      <div className="space-y-3">
        {turn.messages.map((message) => (
          <MessageBubble
            key={message.id}
            message={message}
            language={language}
            disabled={disabled}
            onFillPrompt={onFillPrompt}
            onExecutePrompt={onExecutePrompt}
            onSelectTool={onSelectTool}
          />
        ))}

        {!turn.messages.length && turn.modelText && (
          <GlassCard variant="subtle" className="p-3 text-sm leading-6">
            <div className="mb-2 flex items-center gap-2 font-mono text-[10px] text-muted-foreground"><Bot className="h-3 w-3" />model token stream</div>
            <div className="whitespace-pre-wrap">{turn.modelText}</div>
          </GlassCard>
        )}

        {turn.reasoning && (
          <details className="rounded-2xl border border-purple-300/15 bg-purple-400/10 px-3 py-2 text-xs">
            <summary className="cursor-pointer text-purple-100">{text(language, '推理流', 'Reasoning stream')}</summary>
            <div className="mt-2 whitespace-pre-wrap leading-5 text-purple-50/80">{turn.reasoning}</div>
          </details>
        )}

        {tools.length ? (
          <div className="grid gap-2 sm:grid-cols-2">
            {tools.map((tool) => tool && (
              <GlassCard
                key={tool.id}
                as="button"
                type="button"
                onClick={() => onSelectTool(tool.id)}
                selected={state.selectedToolCallId === tool.id}
                className="p-3 text-left transition-colors hover:border-primary/25 hover:bg-white/[0.04] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
              >
                <div className="flex items-start gap-2">
                  <span className={cn('mt-1 h-2.5 w-2.5 rounded-full', toolDotClass(tool.status))} />
                  <div className="min-w-0 flex-1">
                    <div className="truncate font-mono text-xs text-foreground">{tool.name}</div>
                    <div className="mt-1 line-clamp-2 text-[11px] leading-4 text-muted-foreground">{tool.detail || tool.error?.message || tool.id}</div>
                  </div>
                  <span className={cn('rounded-full border px-1.5 py-0.5 font-mono text-[10px]', statusPillClass(tool.status))}>{statusLabel(tool.status, language)}</span>
                </div>
              </GlassCard>
            ))}
          </div>
        ) : null}

        {approvalEvents.map((event) => (
          <GlassAlert key={event.id} tone="warning" className="px-3 py-2 text-xs">
            <div className="flex items-center gap-2 font-medium"><ShieldCheck className="h-4 w-4 text-amber-200" />{event.type === 'approval_required' ? text(language, '等待审批', 'Approval required') : text(language, '审批已处理', 'Approval resolved')}</div>
            <div className="mt-1 text-amber-100/75">{event.message || event.detail || safeJson(event.data || {})}</div>
          </GlassAlert>
        ))}

        {memoryEvents.map((event) => (
          <GlassAlert key={event.id} tone="info" className="px-3 py-2 text-xs">
            {text(language, '记忆已压缩：', 'Memory compacted: ')}{event.message || event.detail || formatTime(event.timestamp)}
          </GlassAlert>
        ))}
      </div>
    </GlassCard>
  )
}

function MessageBubble({
  message,
  language,
  disabled,
  onFillPrompt,
  onExecutePrompt,
  onSelectTool,
}: {
  message: import('@/lib/api').AgentMessage
  language: Language
  disabled: boolean
  onFillPrompt: (prompt: string) => void
  onExecutePrompt: (prompt: string) => void
  onSelectTool: (toolCallId: string) => void
}) {
  const isUser = message.role === 'user'
  const isTool = message.role === 'tool'
  const isSystem = message.role === 'system'
  const isAgent = !isUser && !isTool && !isSystem
  const command = parseCommandSummary(message.content || '')
  const suggestions = (message.suggested_prompts || []).map((value) => String(value || '').trim()).filter(Boolean).slice(0, 4)
  const options = (message.recommendation_options || []).filter((item) => item.title || item.detail || item.prompt).slice(0, 5)

  return (
    <div className={cn('flex', isUser ? 'justify-end' : 'justify-start')}>
      <GlassCard as="article" variant="subtle" className={cn(
        'max-w-[88%] rounded-2xl px-3.5 py-2.5 text-sm leading-6 shadow-sm',
        isUser && 'bg-primary/20 text-foreground',
        isAgent && 'border border-white/10 bg-white/[0.055] text-foreground/90',
        isTool && 'border border-amber-400/20 bg-amber-500/10 text-amber-100',
        isSystem && 'border border-white/10 bg-white/[0.04] text-muted-foreground',
      )}>
        <div className="mb-1 flex items-center gap-2 font-mono text-[10px] text-muted-foreground">
          {isUser ? <MessageSquare className="h-3 w-3" /> : isTool ? <Wrench className="h-3 w-3" /> : <Bot className="h-3 w-3" />}
          <span>{roleLabel(message.role, language)}</span>
          <span>·</span>
          <span>{formatTime(message.timestamp)}</span>
          {message.phase && <span className={cn('ml-auto rounded-full border px-1.5 py-0.5', statusPillClass(message.phase))}>{message.phase}</span>}
        </div>

        {command ? <CommandResultView command={command} language={language} /> : <div className="whitespace-pre-wrap break-words">{messageText(message) || '—'}</div>}

        {message.reasoning && (
          <details className="mt-2 rounded-xl border border-white/10 bg-black/15 px-3 py-2 text-xs">
            <summary className="cursor-pointer text-muted-foreground">{text(language, '推理摘要', 'Reasoning')}</summary>
            <div className="mt-2 whitespace-pre-wrap text-muted-foreground">{message.reasoning}</div>
          </details>
        )}

        {message.tool_calls?.length ? (
          <div className="mt-2 flex flex-wrap gap-1.5">
            {message.tool_calls.map((tool) => (
              <button key={tool.id} type="button" onClick={() => onSelectTool(tool.id)} className="inline-flex items-center gap-1.5 rounded-xl border border-white/10 bg-black/15 px-2 py-1 font-mono text-[10px] text-muted-foreground hover:border-primary/25 hover:text-foreground">
                <span className={cn('h-2 w-2 rounded-full', toolDotClass(tool.status))} />
                {tool.name}
              </button>
            ))}
          </div>
        ) : null}

        {suggestions.length ? (
          <div className="mt-2 rounded-2xl border border-primary/20 bg-primary/10 p-2 text-xs">
            <div className="mb-1.5 flex items-center gap-2 text-primary"><Sparkles className="h-3.5 w-3.5" />{text(language, '建议执行', 'Suggested prompts')}</div>
            <div className="space-y-1.5">
              {suggestions.map((prompt, index) => (
                <div key={`${prompt}_${index}`} className="group rounded-xl border border-white/10 bg-black/15 p-2">
                  <button type="button" disabled={disabled} onClick={() => onFillPrompt(prompt)} className="w-full text-left text-[11px] leading-5 text-foreground/90 hover:text-primary disabled:opacity-60">{prompt}</button>
                  <div className="mt-1 flex justify-end gap-1.5">
                    <Button type="button" size="sm" variant="ghost" disabled={disabled} onClick={() => onFillPrompt(prompt)} className="h-7 rounded-lg px-2 text-[11px]">{text(language, '填入', 'Fill')}</Button>
                    <Button type="button" size="sm" disabled={disabled} onClick={() => onExecutePrompt(prompt)} className="h-7 rounded-lg px-2 text-[11px]">{text(language, '执行', 'Run')}</Button>
                  </div>
                </div>
              ))}
            </div>
          </div>
        ) : null}

        {options.length ? (
          <div className="mt-2 rounded-2xl border border-cyan-300/20 bg-cyan-400/10 p-2 text-xs">
            <div className="mb-1.5 text-cyan-100">{text(language, '可选建议', 'Recommendation options')}</div>
            <div className="space-y-1.5">
              {options.map((item, index) => (
                <button key={item.id || index} type="button" disabled={disabled} onClick={() => onFillPrompt(String(index + 1))} className="w-full rounded-xl border border-white/10 bg-black/15 px-2.5 py-2 text-left hover:border-cyan-300/30 disabled:opacity-60">
                  <span className="font-medium text-foreground/90">{index + 1}. {item.title || item.detail}</span>
                  {item.detail && <span className="mt-0.5 line-clamp-2 block text-[10px] text-muted-foreground">{item.detail}</span>}
                </button>
              ))}
            </div>
          </div>
        ) : null}
      </GlassCard>
    </div>
  )
}

function OptimisticBubble({ message, language }: { message: OptimisticMessage; language: Language }) {
  return (
    <div className="flex justify-end">
      <GlassCard as="article" variant="subtle" className={cn('max-w-[88%] rounded-2xl px-3.5 py-2.5 text-sm leading-6', message.status === 'failed' ? 'border border-red-300/25 bg-red-400/10' : 'bg-primary/20')}>
        <div className="mb-1 flex items-center gap-2 font-mono text-[10px] text-muted-foreground">
          <MessageSquare className="h-3 w-3" />{text(language, '你', 'You')} · {formatTime(message.timestamp)}
          {message.status === 'sending' && <Loader2 className="ml-auto h-3 w-3 animate-spin" />}
        </div>
        <div className="whitespace-pre-wrap break-words">{message.content}</div>
      </GlassCard>
    </div>
  )
}

function CommandResultView({ command, language }: { command: import('@/agent/utils').CommandSummary; language: Language }) {
  const ok = command.exitCode === undefined || command.exitCode === '0'
  return (
    <div>
      <div className="flex flex-wrap items-center gap-2">
        {ok ? <CheckCircle2 className="h-4 w-4 text-emerald-300" /> : <Clock className="h-4 w-4 text-amber-300" />}
        <span className="font-medium">{text(language, '命令已执行', 'Command executed')}</span>
        <code className="rounded-lg bg-black/25 px-2 py-0.5 text-xs">{command.command}</code>
        {command.exitCode !== undefined && <span className="font-mono text-xs text-muted-foreground">exit={command.exitCode}</span>}
      </div>
      <details className="mt-2 rounded-xl border border-white/10 bg-black/15 px-3 py-2 text-xs">
        <summary className="cursor-pointer text-muted-foreground">{text(language, '执行详情', 'Execution details')}</summary>
        <div className="mt-2 grid gap-2">
          {command.stdout && <OutputBlock label="stdout" value={command.stdout} maxHeight="max-h-40" />}
          {command.stderr && <OutputBlock label="stderr" value={command.stderr} maxHeight="max-h-40" />}
        </div>
      </details>
    </div>
  )
}

function findTool(state: AgentCockpitState, id: string) {
  for (const tool of state.session?.tool_calls || []) if (tool.id === id) return tool
  for (const message of state.session?.messages || []) {
    for (const tool of message.tool_calls || []) if (tool.id === id) return tool
  }
  return undefined
}

export { commandFromToolCall }
