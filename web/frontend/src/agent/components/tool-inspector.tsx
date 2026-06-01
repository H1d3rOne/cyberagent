import { Copy, FileText, Layers3, RotateCcw, Wrench } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { text, type Language } from '@/lib/console'
import { cn } from '@/lib/utils'
import type { AgentToolCall } from '@/lib/api'
import { commandFromToolCall, formatTime, safeJson, statusLabel, statusPillClass, toolDotClass } from '@/agent/utils'
import { InspectorBlock, KeyValue, OutputBlock } from './shared'

export function ToolInspector({
  tool,
  language,
  onRefresh,
  onClear,
}: {
  tool?: AgentToolCall
  language: Language
  onRefresh?: () => void
  onClear?: () => void
}) {
  return (
    <div className="flex min-h-0 flex-col gap-3">
      <div className="flex items-center justify-between gap-2">
        <div className="min-w-0">
          <div className="text-sm font-medium">{text(language, 'Tool Inspector', 'Tool Inspector')}</div>
          <div className="mt-0.5 text-[10px] text-muted-foreground">{text(language, '查看当前选中的工具调用、结果与元数据。', 'Inspect the selected tool call, result and metadata.')}</div>
        </div>
        <div className="flex items-center gap-1.5">
          {onRefresh && (
            <Button type="button" variant="outline" size="sm" onClick={onRefresh} className="h-8 rounded-xl bg-transparent px-2">
              <RotateCcw className="h-3.5 w-3.5" />
            </Button>
          )}
          {onClear && (
            <Button type="button" variant="ghost" size="sm" onClick={onClear} className="h-8 rounded-xl px-2">
              ×
            </Button>
          )}
        </div>
      </div>

      {tool ? (
        <div className="space-y-3">
          <InspectorBlock icon={Wrench} title={text(language, '调用摘要', 'Call summary')}>
            <div className="flex items-start gap-2">
              <span className={cn('mt-1 h-2.5 w-2.5 shrink-0 rounded-full', toolDotClass(tool.status))} />
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm font-medium">{tool.name}</div>
                <div className="mt-1 flex flex-wrap gap-1.5 font-mono text-[10px] text-muted-foreground">
                  <span className={cn('rounded-full border px-1.5 py-0.5', statusPillClass(tool.status))}>{statusLabel(tool.status, language)}</span>
                  <span>{formatTime(tool.started_at)}</span>
                  {tool.finished_at && <span>{formatTime(tool.finished_at)}</span>}
                </div>
              </div>
            </div>
            {tool.detail && <KeyValue label={text(language, '详情', 'Detail')} value={tool.detail} />}
          </InspectorBlock>

          {commandFromToolCall(tool) ? (
            <InspectorBlock icon={FileText} title={text(language, '命令结果', 'Command result')}>
              {(() => {
                const command = commandFromToolCall(tool)!
                return (
                  <>
                    <KeyValue label="command" value={command.command} />
                    {command.exitCode !== undefined && <KeyValue label="exit_code" value={command.exitCode} />}
                    {command.workdir && <KeyValue label="workdir" value={command.workdir} />}
                    {command.duration && <KeyValue label="duration" value={command.duration} />}
                    {command.stdout && <OutputBlock label="stdout" value={command.stdout} maxHeight="max-h-44" />}
                    {command.stderr && <OutputBlock label="stderr" value={command.stderr} maxHeight="max-h-44" />}
                  </>
                )
              })()}
            </InspectorBlock>
          ) : null}

          {tool.error?.message && (
            <InspectorBlock icon={Layers3} title={text(language, '错误', 'Error')}>
              <KeyValue label="error" value={tool.error.message} />
              {tool.error.code && <KeyValue label="code" value={tool.error.code} />}
            </InspectorBlock>
          )}

          <InspectorBlock icon={Copy} title={text(language, '元数据', 'Metadata')}>
            <OutputBlock label="metadata" value={safeJson(tool.metadata || {})} maxHeight="max-h-72" />
            {tool.result && <OutputBlock label="result" value={safeJson(tool.result)} maxHeight="max-h-72" />}
          </InspectorBlock>
        </div>
      ) : (
        <div className="rounded-2xl border border-dashed border-white/10 p-4 text-sm text-muted-foreground">
          {text(language, '未选择工具调用。点击时间线中的工具卡片后，这里会显示完整结果。', 'No tool selected. Click a tool card in the timeline to inspect it here.')}
        </div>
      )}
    </div>
  )
}
