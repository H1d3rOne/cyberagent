import type { Language } from '@/lib/console'
import type { ApprovalStatus, JobStatus, RunStatus, Severity } from '@/lib/api'

export function statusLabel(status: RunStatus | JobStatus | ApprovalStatus | undefined, language: Language): string {
  const value = String(status || 'unknown')
  const labels: Record<string, { zh: string; en: string }> = {
    created: { zh: '已创建', en: 'Created' },
    planning: { zh: '规划中', en: 'Planning' },
    queued: { zh: '排队', en: 'Queued' },
    running: { zh: '运行', en: 'Running' },
    succeeded: { zh: '完成', en: 'Done' },
    failed: { zh: '失败', en: 'Failed' },
    cancelled: { zh: '取消', en: 'Cancelled' },
    skipped: { zh: '跳过', en: 'Skipped' },
    pending: { zh: '待审', en: 'Pending' },
    approved: { zh: '通过', en: 'Approved' },
    rejected: { zh: '拒绝', en: 'Rejected' },
    expired: { zh: '过期', en: 'Expired' },
  }
  return labels[value]?.[language] || value
}

export function statusTone(status: RunStatus | JobStatus | ApprovalStatus | undefined): string {
  switch (status) {
    case 'running':
    case 'queued':
    case 'planning':
      return 'border-cyan-400/25 bg-cyan-400/10 text-cyan-200'
    case 'succeeded':
    case 'approved':
      return 'border-emerald-400/25 bg-emerald-400/10 text-emerald-200'
    case 'failed':
    case 'rejected':
      return 'border-red-400/25 bg-red-400/10 text-red-200'
    case 'pending':
      return 'border-amber-400/25 bg-amber-400/10 text-amber-200'
    case 'cancelled':
    case 'skipped':
      return 'border-zinc-400/25 bg-zinc-400/10 text-zinc-200'
    default:
      return 'border-white/10 bg-white/[0.04] text-muted-foreground'
  }
}

export function severityTone(severity: Severity | undefined): string {
  switch (severity) {
    case 'critical':
      return 'border-red-400/35 bg-red-500/15 text-red-100'
    case 'high':
      return 'border-orange-400/35 bg-orange-500/15 text-orange-100'
    case 'medium':
      return 'border-amber-400/35 bg-amber-500/15 text-amber-100'
    case 'low':
      return 'border-sky-400/35 bg-sky-500/15 text-sky-100'
    case 'info':
      return 'border-zinc-400/25 bg-zinc-500/10 text-zinc-100'
    default:
      return 'border-white/10 bg-white/[0.04] text-muted-foreground'
  }
}

export function formatDate(value: string | undefined, language: Language): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(language === 'zh' ? 'zh-CN' : 'en-US', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

export function formatTime(value: string | undefined, language: Language): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(language === 'zh' ? 'zh-CN' : 'en-US', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(date)
}

export function formatBytes(value: number | undefined): string {
  if (!value) return '0 B'
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}

export function hostLabel(target: string | undefined): string {
  if (!target) return '—'
  try {
    return new URL(target).host
  } catch {
    return target
  }
}

export function shortId(value: string | undefined, size = 12): string {
  if (!value) return '—'
  if (value.length <= size + 4) return value
  return `${value.slice(0, size)}…`
}
