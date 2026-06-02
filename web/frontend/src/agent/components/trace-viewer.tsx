import { Code2, Download, Loader2, RefreshCcw, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { text, type Language } from '@/lib/console'
import type { AgentTraceResponse } from '@/lib/api'
import { formatDateTime, safeJson, statusLabel } from '@/agent/utils'

export function TraceViewerDrawer({
  trace,
  loading,
  language,
  onClose,
  onRefresh,
  onDownload,
}: {
  trace: AgentTraceResponse | null
  loading: boolean
  language: Language
  onClose: () => void
  onRefresh: () => void
  onDownload: () => void
}) {
  const items = (trace?.timeline || []).slice(0, 120)
  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/55 backdrop-blur-sm">
      <aside
        role="dialog"
        aria-modal="true"
        aria-label="Agent Trace"
        className="flex h-full w-full max-w-2xl flex-col border-l border-white/10 bg-[#050816] shadow-2xl"
      >
        <header className="flex h-14 shrink-0 items-center justify-between border-b border-white/10 px-4">
          <div className="min-w-0">
            <div className="flex items-center gap-2 text-sm font-semibold"><Code2 className="h-4 w-4 text-primary" />Agent Trace</div>
            <div className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground">{trace?.session_id || text(language, '加载中', 'Loading')}</div>
          </div>
          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={onRefresh}
              disabled={loading}
              className="h-8 rounded-xl px-2"
              aria-label={text(language, '刷新 Trace', 'Refresh trace')}
            >
              <RefreshCcw className={loading ? 'h-3.5 w-3.5 animate-spin' : 'h-3.5 w-3.5'} />
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={onDownload}
              disabled={!trace}
              className="h-8 rounded-xl bg-transparent px-2"
              aria-label={text(language, '下载 Trace', 'Download trace')}
            >
              <Download className="h-3.5 w-3.5" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={onClose}
              className="h-8 rounded-xl px-2"
              aria-label={text(language, '关闭 Trace', 'Close trace')}
            >
              <X className="h-3.5 w-3.5" />
            </Button>
          </div>
        </header>

        <div className="agent-scroll-area min-h-0 flex-1 overflow-y-auto p-4">
          {loading && !trace ? (
            <div className="grid h-full place-items-center text-sm text-muted-foreground">
              <span className="inline-flex items-center gap-2"><Loader2 className="h-4 w-4 animate-spin" />{text(language, '正在加载 trace...', 'Loading trace...')}</span>
            </div>
          ) : trace ? (
            <div className="space-y-4">
              <div className="grid gap-2 sm:grid-cols-4">
                <TraceStat label={text(language, '消息', 'Messages')} value={safeCount(trace.summary?.message_count)} />
                <TraceStat label={text(language, '工具', 'Tools')} value={safeCount(trace.summary?.tool_call_count)} />
                <TraceStat label={text(language, '上下文', 'Context')} value={safeCount(trace.summary?.context_ref_count)} />
                <TraceStat label={text(language, '变更', 'Changes')} value={safeCount(trace.summary?.pending_change_count)} />
              </div>

              {trace.active_task && (
                <section className="rounded-2xl border border-white/10 bg-white/[0.035] p-3">
                  <div className="text-xs font-medium">{trace.active_task.goal || trace.active_task.summary}</div>
                  <div className="mt-1 font-mono text-[10px] text-muted-foreground">{trace.active_task.kind} · {trace.active_task.status}</div>
                </section>
              )}

              <section className="space-y-2">
                <div className="flex items-center justify-between text-xs text-muted-foreground">
                  <span>{text(language, '时间线', 'Timeline')}</span>
                  <span className="font-mono">{items.length}/{trace.timeline?.length || 0}</span>
                </div>
                <div className="space-y-2">
                  {items.map((item, index) => (
                    <div key={String(item.id || `${item.kind}_${index}`)} className="rounded-2xl border border-white/10 bg-white/[0.035] p-3 text-xs">
                      <div className="flex items-start justify-between gap-2">
                        <div>
                          <div className="font-medium">{String(item.name || item.role || item.kind || 'event')}</div>
                          <div className="mt-1 font-mono text-[10px] text-muted-foreground">{formatDateTime(String(item.timestamp || ''))}</div>
                        </div>
                        {item.status ? <span className="rounded-full border border-white/10 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">{statusLabel(String(item.status), language)}</span> : null}
                      </div>
                      <pre className="mt-2 max-h-36 overflow-auto whitespace-pre-wrap font-mono text-[10px] leading-5 text-muted-foreground">{safeJson(item)}</pre>
                    </div>
                  ))}
                </div>
              </section>
            </div>
          ) : (
            <div className="rounded-2xl border border-dashed border-white/10 p-4 text-sm text-muted-foreground">
              {text(language, '暂无 trace 数据。', 'No trace data.')}
            </div>
          )}
        </div>
      </aside>
    </div>
  )
}

function TraceStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-white/10 bg-white/[0.035] p-3">
      <div className="text-[10px] uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className="mt-1 font-mono text-lg text-foreground">{value}</div>
    </div>
  )
}

function safeCount(value: unknown) {
  return String(typeof value === 'number' ? value : 0)
}
