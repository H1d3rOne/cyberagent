import { useId, useMemo, useState } from 'react'
import { AlertTriangle, Check, Clipboard, Code2, Download, FileJson, Loader2, RefreshCcw, Search, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { text, type Language } from '@/lib/console'
import { cn } from '@/lib/utils'
import type { AgentTraceResponse } from '@/lib/api'
import { formatDateTime, statusLabel } from '@/agent/utils'
import { useDialogA11y } from './dialog-a11y'
import {
  buildTraceErrorMarkers,
  buildTraceJsonSections,
  formatTraceJson,
  matchingJsonLines,
  traceSectionMatchesQuery,
  type TraceErrorMarker,
  type TraceJsonSection,
} from './trace-json'

const TIMELINE_PREVIEW_LIMIT = 120

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
  const [query, setQuery] = useState('')
  const [expandedSections, setExpandedSections] = useState<Record<string, boolean>>({})
  const [copiedKey, setCopiedKey] = useState('')
  const items = (trace?.timeline || []).slice(0, TIMELINE_PREVIEW_LIMIT)
  const jsonSections = useMemo(() => buildTraceJsonSections(trace), [trace])
  const filteredJsonSections = useMemo(
    () => jsonSections.filter((section) => traceSectionMatchesQuery(section, query)),
    [jsonSections, query],
  )
  const errorMarkers = useMemo(() => buildTraceErrorMarkers(jsonSections), [jsonSections])
  const fullTraceJson = useMemo(() => formatTraceJson(trace || {}).json, [trace])
  const titleId = useId()
  const descriptionId = useId()
  const drawerRef = useDialogA11y<HTMLElement>({
    onClose,
    modal: true,
    initialFocusSelector: '[data-dialog-close]',
  })

  const copyText = async (key: string, value: string) => {
    await copyTextToClipboard(value)
    setCopiedKey(key)
    window.setTimeout(() => setCopiedKey((current) => current === key ? '' : current), 1200)
  }

  const downloadSection = (section: TraceJsonSection) => {
    if (!trace) return
    downloadTextFile(
      `cyberagent-trace-${safeFilename(trace.session_id)}-${safeFilename(section.id)}.json`,
      section.json,
      'application/json;charset=utf-8',
    )
  }

  const focusErrorMarker = (marker: TraceErrorMarker) => {
    setExpandedSections((current) => ({ ...current, [marker.sectionId]: true }))
    window.setTimeout(() => {
      document.getElementById(`trace-json-${marker.sectionId}`)?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    }, 60)
  }

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/55 backdrop-blur-sm">
      <aside
        ref={drawerRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={descriptionId}
        tabIndex={-1}
        className="flex h-full w-full max-w-2xl flex-col border-l border-white/10 bg-[#050816] shadow-2xl"
      >
        <header className="flex h-14 shrink-0 items-center justify-between border-b border-white/10 px-4">
          <div className="min-w-0">
            <div id={titleId} className="flex items-center gap-2 text-sm font-semibold"><Code2 className="h-4 w-4 text-primary" />Agent Trace</div>
            <div id={descriptionId} className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground">{trace?.session_id || text(language, '加载中', 'Loading')}</div>
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
              data-dialog-close
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

              {errorMarkers.length > 0 && (
                <TraceErrorLocator markers={errorMarkers} language={language} onFocus={focusErrorMarker} />
              )}

              <section className="space-y-2">
                <div className="flex items-center justify-between text-xs text-muted-foreground">
                  <span>{text(language, '时间线预览', 'Timeline preview')}</span>
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
                      {traceItemDetail(item) && <p className="mt-2 line-clamp-3 text-[11px] leading-5 text-muted-foreground">{traceItemDetail(item)}</p>}
                    </div>
                  ))}
                </div>
              </section>

              <section className="space-y-3">
                <div className="rounded-2xl border border-white/10 bg-white/[0.035] p-3">
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2 text-xs font-medium">
                        <FileJson className="h-4 w-4 text-primary" />
                        {text(language, 'JSON 分段视图', 'Segmented JSON view')}
                      </div>
                      <div className="mt-1 font-mono text-[10px] text-muted-foreground">
                        {filteredJsonSections.length}/{jsonSections.length} sections · {errorMarkers.length} errors
                      </div>
                    </div>
                    <div className="flex shrink-0 flex-wrap gap-2">
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={!trace}
                        onClick={() => { void copyText('full_trace', fullTraceJson) }}
                        className="h-8 rounded-xl bg-transparent px-2 text-[11px]"
                      >
                        {copiedKey === 'full_trace' ? <Check className="mr-1.5 h-3.5 w-3.5" /> : <Clipboard className="mr-1.5 h-3.5 w-3.5" />}
                        {text(language, '复制全部', 'Copy all')}
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={!trace}
                        onClick={onDownload}
                        className="h-8 rounded-xl bg-transparent px-2 text-[11px]"
                      >
                        <Download className="mr-1.5 h-3.5 w-3.5" />
                        {text(language, '下载全部', 'Download all')}
                      </Button>
                    </div>
                  </div>
                  <div className="relative mt-3">
                    <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                    <Input
                      value={query}
                      onChange={(event) => setQuery(event.target.value)}
                      placeholder={text(language, '搜索 JSON path / key / value / error...', 'Search JSON path / key / value / error...')}
                      className="h-9 rounded-xl border-white/10 bg-black/20 pl-9 pr-9 font-mono text-xs"
                    />
                    {query && (
                      <button
                        type="button"
                        aria-label={text(language, '清空搜索', 'Clear search')}
                        onClick={() => setQuery('')}
                        className="absolute right-2 top-1/2 grid h-6 w-6 -translate-y-1/2 place-items-center rounded-full text-muted-foreground transition hover:bg-white/10 hover:text-foreground"
                      >
                        <X className="h-3.5 w-3.5" />
                      </button>
                    )}
                  </div>
                </div>

                <div className="space-y-2">
                  {filteredJsonSections.map((section) => {
                    const defaultExpanded = section.id === 'overview' || section.hasError || Boolean(query.trim())
                    const expanded = expandedSections[section.id] ?? defaultExpanded
                    return (
                      <TraceJsonSectionCard
                        key={section.id}
                        section={section}
                        language={language}
                        query={query}
                        expanded={expanded}
                        copied={copiedKey === section.id}
                        onToggle={() => setExpandedSections((current) => ({ ...current, [section.id]: !expanded }))}
                        onCopy={() => { void copyText(section.id, section.json) }}
                        onDownload={() => downloadSection(section)}
                      />
                    )
                  })}
                  {!filteredJsonSections.length && (
                    <div className="rounded-2xl border border-dashed border-white/10 p-4 text-sm text-muted-foreground">
                      {text(language, '没有匹配的 JSON 分段。', 'No matching JSON sections.')}
                    </div>
                  )}
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

function TraceErrorLocator({ markers, language, onFocus }: { markers: TraceErrorMarker[]; language: Language; onFocus: (marker: TraceErrorMarker) => void }) {
  return (
    <section className="rounded-2xl border border-red-400/25 bg-red-500/10 p-3">
      <div className="mb-2 flex items-center gap-2 text-xs font-medium text-red-100">
        <AlertTriangle className="h-4 w-4" />
        {text(language, '错误定位', 'Error locator')}
      </div>
      <div className="flex flex-wrap gap-2">
        {markers.slice(0, 12).map((marker) => (
          <button
            key={marker.id}
            type="button"
            onClick={() => onFocus(marker)}
            className="rounded-full border border-red-300/25 bg-black/15 px-2.5 py-1 text-left font-mono text-[10px] text-red-50/85 transition hover:border-red-200/50 hover:text-white"
            title={`${marker.path}: ${marker.message}`}
          >
            {marker.label}
          </button>
        ))}
        {markers.length > 12 && (
          <span className="rounded-full border border-red-300/20 px-2.5 py-1 font-mono text-[10px] text-red-50/70">
            +{markers.length - 12}
          </span>
        )}
      </div>
    </section>
  )
}

function TraceJsonSectionCard({
  section,
  language,
  query,
  expanded,
  copied,
  onToggle,
  onCopy,
  onDownload,
}: {
  section: TraceJsonSection
  language: Language
  query: string
  expanded: boolean
  copied: boolean
  onToggle: () => void
  onCopy: () => void
  onDownload: () => void
}) {
  const matches = matchingJsonLines(section.json, query, 4)
  return (
    <article
      id={`trace-json-${section.id}`}
      className={cn(
        'rounded-2xl border p-3 transition',
        section.hasError ? 'border-red-400/25 bg-red-500/10' : 'border-white/10 bg-white/[0.035]',
      )}
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <button type="button" onClick={onToggle} className="min-w-0 flex-1 text-left">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-mono text-xs font-medium text-foreground">{section.title}</span>
            {section.hasError && (
              <span className="rounded-full border border-red-300/25 bg-red-400/10 px-1.5 py-0.5 font-mono text-[10px] text-red-100">
                error
              </span>
            )}
          </div>
          <div className="mt-1 flex flex-wrap gap-2 font-mono text-[10px] text-muted-foreground">
            <span>{section.path}</span>
            <span>{section.lineCount} lines</span>
            <span>{formatBytes(section.byteCount)}</span>
            {section.itemCount !== undefined && <span>{section.itemCount} items</span>}
          </div>
          {section.errorText && <div className="mt-1 line-clamp-2 text-[11px] leading-5 text-red-100/80">{section.errorText}</div>}
        </button>
        <div className="flex shrink-0 gap-1.5">
          <Button type="button" variant="ghost" size="sm" onClick={onCopy} className="h-8 rounded-xl px-2 text-[11px]">
            {copied ? <Check className="mr-1.5 h-3.5 w-3.5" /> : <Clipboard className="mr-1.5 h-3.5 w-3.5" />}
            {text(language, '复制', 'Copy')}
          </Button>
          <Button type="button" variant="ghost" size="sm" onClick={onDownload} className="h-8 rounded-xl px-2 text-[11px]">
            <Download className="mr-1.5 h-3.5 w-3.5" />
            {text(language, '下载', 'Download')}
          </Button>
        </div>
      </div>

      {matches.length > 0 && (
        <div className="mt-2 rounded-xl border border-primary/15 bg-primary/10 p-2 font-mono text-[10px] text-primary/90">
          {matches.map((match) => (
            <div key={`${section.id}_${match.line}`} className="truncate">
              L{match.line}: {match.text.trim()}
            </div>
          ))}
        </div>
      )}

      {expanded && (
        <pre className="mt-2 max-h-80 overflow-auto whitespace-pre-wrap break-words rounded-xl border border-white/[0.07] bg-black/40 p-2.5 font-mono text-[11px] leading-5 text-slate-100/90 shadow-inner">
          {section.json}
        </pre>
      )}
    </article>
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

async function copyTextToClipboard(value: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value)
    return
  }
  const textarea = document.createElement('textarea')
  textarea.value = value
  textarea.setAttribute('readonly', 'true')
  textarea.style.position = 'fixed'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)
  textarea.select()
  document.execCommand('copy')
  textarea.remove()
}

function downloadTextFile(filename: string, content: string, mime: string) {
  const blob = new Blob([content], { type: mime })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 1000)
}

function safeFilename(value: string) {
  return (value || 'trace').replace(/[^a-zA-Z0-9._-]+/g, '-').slice(0, 80)
}

function formatBytes(value: number) {
  if (value < 1024) return `${value}B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)}KB`
  return `${(value / 1024 / 1024).toFixed(1)}MB`
}

function traceItemDetail(item: Record<string, unknown>) {
  return String(item.message || item.detail || item.summary || item.content || item.error || '')
}
