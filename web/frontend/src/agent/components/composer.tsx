import { useMemo } from 'react'
import { ChevronDown, Play, Plus, Send, Sparkles, X } from 'lucide-react'
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
    <footer className="shrink-0 border-t border-white/10 bg-[#050816]/70 p-2.5">
      {suggestedPrompt && (
        <div className="mb-2 flex items-center gap-2 rounded-2xl border border-primary/20 bg-primary/10 px-3 py-2 text-xs text-primary">
          <Sparkles className="h-3.5 w-3.5" />
          <span className="min-w-0 flex-1 truncate">{suggestedPrompt}</span>
          <Button type="button" variant="ghost" size="sm" onClick={() => onQuickPrompt(suggestedPrompt)} className="h-7 rounded-xl px-2 text-primary">
            {text(language, '填入', 'Fill')}
          </Button>
        </div>
      )}

      <div className="mb-2 flex h-9 items-center gap-2 rounded-2xl border border-white/10 bg-white/[0.03] px-2 overflow-x-auto">
        {prompts.map((prompt) => (
          <button
            key={prompt}
            type="button"
            onClick={() => onQuickPrompt(prompt)}
            className="inline-flex shrink-0 items-center gap-1.5 rounded-xl border border-white/10 bg-white/[0.04] px-2.5 py-1.5 text-xs text-muted-foreground hover:border-primary/30 hover:text-foreground"
          >
            <Sparkles className="h-3.5 w-3.5" />
            {prompt}
          </button>
        ))}
      </div>

      <div className="mb-2 grid gap-2 rounded-2xl border border-white/10 bg-white/[0.025] p-2 text-xs sm:grid-cols-[minmax(0,1fr)_auto]">
        <div className="min-w-0">
          <div className="truncate font-medium">
            {text(language, '输入自然语言指令即可继续', 'Type a natural-language instruction to continue')}
          </div>
          <div className="mt-1 truncate text-muted-foreground">
            {text(language, '事件流实时更新，不再轮询 session。', 'The event stream updates live; session polling is disabled.')}
          </div>
        </div>
        <div className="flex items-center gap-1.5">
          <span className={cn('rounded-full border px-2 py-1 font-mono text-[10px]', statusPillClass(streamStatus))}>
            {statusLabel(streamStatus, language)}
          </span>
          <span className="rounded-full border border-white/10 bg-black/15 px-2 py-1 font-mono text-[10px]">{operationLevelLabel(operationLevel, language)}</span>
          {suggestedPrompt && (
            <Button type="button" variant="ghost" size="sm" onClick={() => onQuickPrompt(suggestedPrompt)} className="h-8 rounded-xl px-2">
              {text(language, '下一步', 'Next')}
            </Button>
          )}
          <Button type="button" variant="ghost" size="sm" onClick={onClear} className="h-8 rounded-xl px-2">
            <X className="h-3.5 w-3.5" />
          </Button>
          <Button type="button" variant="ghost" size="sm" onClick={onNewTask} className="h-8 rounded-xl px-2">
            <Plus className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      <div className="grid gap-2 sm:grid-cols-[1fr_auto]">
        <textarea
          value={value}
          onChange={(event) => onChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter' && !event.shiftKey) {
              event.preventDefault()
              onSubmit()
            }
          }}
          rows={3}
          placeholder={text(language, '输入指令，Enter 发送，Shift+Enter 换行', 'Type a command. Enter sends, Shift+Enter inserts a new line')}
          className="command-input min-h-[84px] w-full resize-none rounded-2xl px-3 py-2.5 text-sm outline-none placeholder:text-muted-foreground"
          disabled={disabled || sending}
        />
        <div className="flex flex-row gap-2 sm:flex-col">
          <Button type="button" onClick={onSubmit} disabled={disabled || sending || !value.trim()} className="h-12 rounded-2xl px-4">
            <Send className="mr-2 h-4 w-4" />
            {text(language, '发送', 'Send')}
          </Button>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-1">
            <Button type="button" variant="outline" onClick={onContinue} disabled={disabled || sending} className="h-10 rounded-2xl bg-transparent px-3">
              <Play className="mr-2 h-4 w-4" />
              {text(language, '继续', 'Continue')}
            </Button>
            <Button type="button" variant="ghost" onClick={onPause} disabled={disabled || sending} className="h-10 rounded-2xl px-3">
              <ChevronDown className="mr-2 h-4 w-4 rotate-90" />
              {text(language, '暂停', 'Pause')}
            </Button>
            <Button type="button" variant="ghost" onClick={onCancel} disabled={disabled || sending} className="h-10 rounded-2xl px-3 text-red-200 hover:text-red-100">
              ×
              <span className="ml-2">{text(language, '取消', 'Cancel')}</span>
            </Button>
          </div>
        </div>
      </div>
    </footer>
  )
}
