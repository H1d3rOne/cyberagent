import { Loader2, MessageSquare, Plus, RefreshCcw, Trash2 } from 'lucide-react'
import { GlassPanel, GlassToolbar } from '@/components/glass'
import { Button } from '@/components/ui/button'
import { text, type Language } from '@/lib/console'
import { cn } from '@/lib/utils'
import type { AgentSessionSummary } from '@/lib/api'
import { formatTime, originLabel, sessionTitle, statusLabel, statusPillClass } from '@/agent/utils'

export function TaskTree({
  sessions,
  activeSessionId,
  loading,
  collapsed,
  language,
  onToggleCollapsed,
  onCreate,
  onSelect,
  onRefresh,
  onDelete,
}: {
  sessions: AgentSessionSummary[]
  activeSessionId: string
  loading: boolean
  collapsed: boolean
  language: Language
  onToggleCollapsed: () => void
  onCreate: () => void
  onSelect: (sessionId: string) => void
  onRefresh: () => void
  onDelete: (sessionId: string) => void
}) {
  if (collapsed) {
    return (
      <GlassPanel as="aside" variant="panel" className="hidden min-h-0 w-12 shrink-0 flex-col items-center gap-2 overflow-hidden py-2 xl:flex">
        <button
          type="button"
          onClick={onToggleCollapsed}
          className="grid h-9 w-9 place-items-center rounded-xl text-muted-foreground hover:bg-white/[0.06] hover:text-foreground"
          aria-label={text(language, '展开任务列表', 'Expand task list')}
        >
          <MessageSquare className="h-4 w-4" />
        </button>
        <button
          type="button"
          onClick={onCreate}
          className="grid h-9 w-9 place-items-center rounded-xl text-muted-foreground hover:bg-white/[0.06] hover:text-foreground"
          aria-label={text(language, '新任务', 'New task')}
        >
          <Plus className="h-4 w-4" />
        </button>
        <button
          type="button"
          onClick={onRefresh}
          className="grid h-9 w-9 place-items-center rounded-xl text-muted-foreground hover:bg-white/[0.06] hover:text-foreground"
          aria-label={text(language, '刷新', 'Refresh')}
        >
          <RefreshCcw className={cn('h-4 w-4', loading && 'animate-spin')} />
        </button>
      </GlassPanel>
    )
  }

  return (
    <GlassPanel as="aside" variant="panel" className="hidden min-h-0 w-[282px] shrink-0 flex-col overflow-hidden xl:flex">
      <GlassToolbar as="header" className="flex h-12 shrink-0 items-center justify-between border-b px-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-medium">
            <MessageSquare className="h-4 w-4 text-primary" />
            {text(language, 'Task 列表', 'Task list')}
          </div>
          <div className="mt-0.5 font-mono text-[10px] text-muted-foreground">{sessions.length} sessions</div>
        </div>
        <button
          type="button"
          onClick={onToggleCollapsed}
          className="grid h-8 w-8 place-items-center rounded-xl text-muted-foreground hover:bg-white/[0.06] hover:text-foreground"
          aria-label={text(language, '收起任务列表', 'Collapse task list')}
        >
          ×
        </button>
      </GlassToolbar>

      <div className="grid grid-cols-2 gap-2 border-b border-white/10 p-2">
        <Button type="button" onClick={onCreate} disabled={loading} className="h-9 rounded-xl">
          <Plus className="mr-2 h-4 w-4" />
          {text(language, '新建', 'New')}
        </Button>
        <Button type="button" variant="outline" onClick={onRefresh} className="h-9 rounded-xl bg-transparent">
          <RefreshCcw className={cn('mr-2 h-4 w-4', loading && 'animate-spin')} />
          {text(language, '刷新', 'Refresh')}
        </Button>
      </div>

      <div className="agent-scroll-area min-h-0 flex-1 overflow-y-auto p-2">
        {loading && !sessions.length ? (
          <div className="flex items-center gap-2 p-3 text-xs text-muted-foreground">
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
            {text(language, '加载中', 'Loading')}
          </div>
        ) : sessions.length ? (
          <div className="space-y-1.5">
            {sessions.map((item) => {
              const active = item.session_id === activeSessionId
              const origin = item.active_task?.kind === 'schedule' || item.active_task?.status === 'schedule' ? 'schedule' : 'user'
              return (
                <div
                  key={item.session_id}
                  className={cn(
                    'group rounded-2xl border p-2 transition',
                    active
                      ? 'border-primary/35 bg-primary/10 text-foreground shadow-[inset_0_0_0_1px_rgba(96,165,250,.12)]'
                      : 'border-white/5 bg-white/[0.025] text-muted-foreground hover:border-white/12 hover:bg-white/[0.05] hover:text-foreground',
                  )}
                >
                  <div className="flex items-start gap-2">
                    <button type="button" onClick={() => onSelect(item.session_id)} className="min-w-0 flex-1 text-left">
                      <div className="line-clamp-2 text-xs font-medium leading-5">{sessionTitle(item, language)}</div>
                      <div className="mt-1 flex flex-wrap items-center gap-1.5 font-mono text-[10px]">
                        <span className={cn('rounded-full border px-1.5 py-0.5', statusPillClass(item.status))}>{statusLabel(item.status, language)}</span>
                        <span className="rounded-full border border-white/10 bg-black/10 px-1.5 py-0.5">{originLabel(origin, language)}</span>
                        <span>{formatTime(item.updated_at)}</span>
                      </div>
                    </button>
                    <button
                      type="button"
                      onClick={(event) => {
                        event.stopPropagation()
                        onDelete(item.session_id)
                      }}
                      className="grid h-7 w-7 shrink-0 place-items-center rounded-lg opacity-60 hover:bg-red-500/15 hover:text-red-200 group-hover:opacity-100"
                      aria-label={text(language, '删除', 'Delete')}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>
              )
            })}
          </div>
        ) : (
          <div className="rounded-2xl border border-dashed border-white/10 p-3 text-xs leading-5 text-muted-foreground">
            {text(language, '暂无任务。创建会话后，立即任务、定时触发和恢复任务都会出现在这里。', 'No tasks yet. User, scheduled and resumed tasks appear here after creation.')}
          </div>
        )}
      </div>
    </GlassPanel>
  )
}
