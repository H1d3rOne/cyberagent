import { useMemo } from 'react'
import { ChevronDown, Play, Plus, Send, Sparkles, X } from 'lucide-react'
import { GlassAlert, GlassCard, GlassToolbar, StatusPill } from '@/components/glass'
import { Button } from '@/components/ui/button'
import { text, type Language } from '@/lib/console'
import { cn } from '@/lib/utils'
import { type CockpitStreamStatus } from '@/agent/state/store'
import { operationLevelLabel, statusLabel, statusPillClass } from '@/agent/utils'

const QUICK_PROMPTS = {
  zh: [
    '检查 daemon 健康状态',
    '列出最近运行并总结状态',
    '读取当前任务上下文并给下一步',
    '列出可用插件和内置工具',
    '扫描 <目标> scope=<范围> profile=safe',
  ],
  en: [
    'Check daemon health',
    'List recent runs and summarize status',
    'Read current task context and suggest the next step',
    'List available plugins and built-in tools',
    'scan <target> scope=<scope> profile=safe',
  ],
} as const

export function Composer({
  value,
  onChange,
  onSubmit,
  onQuickPrompt,
  onClear,
  onContinue,
  onPause,
  onCancel,
  onNewTask,
  language,
  streamStatus,
  operationLevel,
  suggestedPrompt,
  sending,
  disabled,
}: {
  value: string
  onChange: (value: string) => void
  onSubmit: () => void
  onQuickPrompt: (prompt: string) => void
  onClear: () => void
  onContinue: () => void
  onPause: () => void
  onCancel: () => void
  onNewTask: () => void
  language: Language
  streamStatus: CockpitStreamStatus
  operationLevel: string
  suggestedPrompt?: string
  sending: boolean
  disabled?: boolean
}) {
  const prompts = useMemo(() => QUICK_PROMPTS[language], [language])
  return (
    <GlassToolbar as="footer" className="shrink-0 border-t p-2">
      {suggestedPrompt && (
        <GlassAlert tone="info" className="mb-2 flex items-center gap-2 px-3 py-2 text-xs">
          <Sparkles className="h-3.5 w-3.5" />
          <span className="min-w-0 flex-1 truncate">{suggestedPrompt}</span>
          <Button type="button" variant="ghost" size="sm" onClick={() => onQuickPrompt(suggestedPrompt)} className="h-7 rounded-xl px-2 text-primary">
            {text(language, '填入', 'Fill')}
          </Button>
        </GlassAlert>
      )}

      <GlassToolbar className="mb-1.5 flex h-8 items-center gap-2 overflow-x-auto rounded-xl border px-2">
        {prompts.map((prompt) => (
          <GlassCard
            key={prompt}
            as="button"
            type="button"
            onClick={() => onQuickPrompt(prompt)}
            interactive
            className="inline-flex shrink-0 items-center gap-1.5 rounded-lg px-2 py-1 text-xs text-muted-foreground hover:text-foreground"
          >
            <Sparkles className="h-3.5 w-3.5" />
            {prompt}
          </GlassCard>
        ))}
      </GlassToolbar>

      <GlassCard variant="subtle" className="mb-1.5 flex flex-wrap items-center justify-between gap-2 rounded-xl px-2.5 py-1.5 text-xs">
        <div className="min-w-0 truncate font-medium text-muted-foreground">
          {text(language, '输入指令继续，事件流实时更新', 'Type a command to continue; events update live')}
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          <StatusPill className={cn('px-2 py-1', statusPillClass(streamStatus))}>
            {statusLabel(streamStatus, language)}
          </StatusPill>
          <StatusPill tone="muted" className="px-2 py-1">{operationLevelLabel(operationLevel, language)}</StatusPill>
          {suggestedPrompt && (
            <Button type="button" variant="ghost" size="sm" onClick={() => onQuickPrompt(suggestedPrompt)} className="h-7 rounded-lg px-2">
              {text(language, '下一步', 'Next')}
            </Button>
          )}
          <Button type="button" variant="ghost" size="sm" onClick={onClear} className="h-7 rounded-lg px-2">
            <X className="h-3.5 w-3.5" />
          </Button>
          <Button type="button" variant="ghost" size="sm" onClick={onNewTask} className="h-7 rounded-lg px-2">
            <Plus className="h-3.5 w-3.5" />
          </Button>
        </div>
      </GlassCard>

      <div className="space-y-1.5">
        <textarea
          value={value}
          onChange={(event) => onChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter' && !event.shiftKey) {
              event.preventDefault()
              onSubmit()
            }
          }}
          rows={1}
          placeholder={text(language, '输入指令，Enter 发送，Shift+Enter 换行', 'Type a command. Enter sends, Shift+Enter inserts a new line')}
          className="command-input min-h-[44px] max-h-28 w-full resize-y rounded-xl px-3 py-2 text-sm leading-6 outline-none placeholder:text-muted-foreground"
          disabled={disabled || sending}
        />
        <div className="flex flex-wrap items-center justify-end gap-1.5">
          <Button type="button" onClick={onSubmit} disabled={disabled || sending || !value.trim()} className="h-8 rounded-lg px-3">
            <Send className="mr-2 h-4 w-4" />
            {text(language, '发送', 'Send')}
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={onContinue}
            disabled={disabled || sending}
            className="h-8 rounded-lg bg-transparent px-2.5"
            title={text(language, '继续', 'Continue')}
            aria-label={text(language, '继续', 'Continue')}
          >
            <Play className="h-3.5 w-3.5 sm:mr-1.5" />
            <span className="hidden sm:inline">{text(language, '继续', 'Continue')}</span>
          </Button>
          <Button
            type="button"
            variant="ghost"
            onClick={onPause}
            disabled={disabled || sending}
            className="h-8 rounded-lg px-2.5"
            title={text(language, '暂停', 'Pause')}
            aria-label={text(language, '暂停', 'Pause')}
          >
            <ChevronDown className="h-3.5 w-3.5 rotate-90 sm:mr-1.5" />
            <span className="hidden sm:inline">{text(language, '暂停', 'Pause')}</span>
          </Button>
          <Button
            type="button"
            variant="ghost"
            onClick={onCancel}
            disabled={disabled || sending}
            className="h-8 rounded-lg px-2.5 text-red-200 hover:text-red-100"
            title={text(language, '取消', 'Cancel')}
            aria-label={text(language, '取消', 'Cancel')}
          >
            <X className="h-3.5 w-3.5 sm:mr-1.5" />
            <span className="hidden sm:inline">{text(language, '取消', 'Cancel')}</span>
          </Button>
        </div>
      </div>
    </GlassToolbar>
  )
}
