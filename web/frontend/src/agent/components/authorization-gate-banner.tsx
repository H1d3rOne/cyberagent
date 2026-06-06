import { Loader2, ShieldCheck } from 'lucide-react'
import { GlassAlert } from '@/components/glass'
import { Button } from '@/components/ui/button'
import { text, type Language } from '@/lib/console'
import type { AgentAuthorizationRequest, AgentOperationLevel } from '@/lib/api'
import { operationLevelLabel, safeJson } from '@/agent/utils'

export interface AuthorizationGate {
  id: string
  action: string
  prompt?: string
  target?: string
  scope?: string
  policy?: string
  reason?: string
  arguments?: Record<string, unknown>
  requiredLevel: AgentOperationLevel
  source: 'pending' | 'legacy'
  messageIndex?: number
  stateLabel?: string
  stateDetail?: string
  uiHint?: string
}

export function AuthorizationGateBanner({
  gate,
  language,
  busy,
  running,
  onAuthorize,
  onFill,
  onDeny,
}: {
  gate: AuthorizationGate
  language: Language
  busy: boolean
  running: boolean
  onAuthorize: () => void
  onFill: () => void
  onDeny: () => void
}) {
  return (
    <GlassAlert tone="warning" className="mb-2 px-3 py-2 text-xs shadow-[0_0_24px_rgba(251,191,36,0.08)]">
      <div className="flex flex-wrap items-start gap-2 sm:items-center">
        <ShieldCheck className="mt-0.5 h-4 w-4 text-amber-200 sm:mt-0" />
        <span className="font-medium">{text(language, '需要授权确认', 'Authorization required')}</span>
        <span className="rounded-full border border-amber-200/20 bg-black/15 px-2 py-0.5 font-mono text-[10px] text-amber-100/85">
          {gate.action}
        </span>
        {gate.stateLabel && (
          <span className="rounded-full border border-amber-200/20 bg-amber-200/10 px-2 py-0.5 text-[10px] text-amber-50">
            {gate.stateLabel}
          </span>
        )}
        {gate.target && <span className="min-w-0 truncate font-mono text-[10px] text-amber-100/80">{gate.target}</span>}
        {running && !busy && (
          <span className="rounded-full border border-amber-200/20 bg-black/15 px-2 py-0.5 text-[10px] text-amber-100/70">
            {text(language, '后台仍在收尾，可直接授权', 'Background request is finishing; you can authorize now')}
          </span>
        )}
        <div className="flex w-full shrink-0 flex-wrap items-center gap-1.5 sm:ml-auto sm:w-auto">
          <Button type="button" variant="ghost" size="sm" onClick={onDeny} disabled={busy} className="h-10 flex-1 rounded-xl px-2 text-amber-100/80 hover:bg-red-300/10 hover:text-red-100 sm:h-7 sm:flex-none">
            {text(language, '拒绝', 'Deny')}
          </Button>
          {gate.source === 'legacy' && gate.prompt && (
            <Button type="button" variant="ghost" size="sm" onClick={onFill} disabled={busy} className="h-10 flex-1 rounded-xl px-2 text-amber-100 hover:bg-amber-300/10 sm:h-7 sm:flex-none">
              {text(language, '填入声明', 'Fill')}
            </Button>
          )}
          <Button type="button" size="sm" onClick={onAuthorize} disabled={busy} className="h-10 flex-1 rounded-xl bg-amber-300 px-2 text-black hover:bg-amber-200 sm:h-7 sm:flex-none">
            {busy ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : <ShieldCheck className="mr-1.5 h-3.5 w-3.5" />}
            {text(language, '允许并继续', 'Allow & continue')}
          </Button>
        </div>
      </div>
      <div className="mt-1 line-clamp-3 text-[11px] leading-5 text-amber-100/75">
        {text(
          language,
          `${gate.reason || gate.stateDetail || 'Agent 已暂停执行，等待操作者确认。'} 本次需要「${operationLevelLabel(gate.requiredLevel, language)}」权限。`,
          `${gate.reason || gate.stateDetail || 'The Agent paused execution and is waiting for operator approval.'} Required: ${operationLevelLabel(gate.requiredLevel, language)}.`,
        )}
      </div>
      {gate.uiHint && gate.uiHint !== gate.reason && gate.uiHint !== gate.stateDetail && (
        <div className="mt-1 text-[10px] leading-4 text-amber-100/60">{gate.uiHint}</div>
      )}
      {gate.arguments && (
        <details className="mt-1.5 rounded-xl border border-amber-200/15 bg-black/10 px-2 py-1.5 text-[11px]">
          <summary className="cursor-pointer text-amber-100/75">{text(language, '动作参数', 'Action args')}</summary>
          <pre className="mt-1 max-h-24 overflow-auto whitespace-pre-wrap font-mono text-amber-50/80">{safeJson(gate.arguments)}</pre>
        </details>
      )}
    </GlassAlert>
  )
}

export function buildAuthorizationGateFromPending(auth: AgentAuthorizationRequest | undefined): AuthorizationGate | null {
  if (!auth || String(auth.status || '').toLowerCase() !== 'pending') return null
  const args = recordFromUnknown(auth.arguments)
  const action = String(auth.action_name || 'agent.action')
  const target = agentAuthorizationTargetFromArgs(args)
  const scope = String(args.scope || args.scope_url || '').trim()
  return {
    id: auth.id,
    action,
    target,
    scope,
    policy: auth.policy,
    reason: auth.reason,
    arguments: args,
    requiredLevel: auth.required_level && typeof auth.required_level === 'string' ? (auth.required_level as AgentOperationLevel) : 'read_only',
    prompt: auth.original_prompt,
    source: 'pending',
  }
}

function agentAuthorizationTargetFromArgs(args: Record<string, unknown>) {
  const values = [args.target, args.url, args.input, args.path, args.file, args.command, args.run_id]
  for (const value of values) {
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  return ''
}

function recordFromUnknown(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}
