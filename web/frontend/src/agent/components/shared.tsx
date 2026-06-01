import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'

export function InspectorBlock({ icon: Icon, title, children, className }: { icon: LucideIcon; title: string; children: ReactNode; className?: string }) {
  return (
    <section className={cn('rounded-2xl border border-white/10 bg-white/[0.025] p-3', className)}>
      <div className="mb-3 flex items-center gap-2 text-sm font-medium">
        <Icon className="h-4 w-4 text-primary" />
        <span>{title}</span>
      </div>
      {children}
    </section>
  )
}

export function KeyValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="mb-1.5 grid gap-1 rounded-xl bg-white/[0.03] px-2.5 py-2 text-xs">
      <span className="text-[10px] uppercase tracking-wide text-muted-foreground">{label}</span>
      <span className="break-words font-mono text-foreground/90">{value || '—'}</span>
    </div>
  )
}

export function OutputBlock({ label, value, maxHeight = 'max-h-72' }: { label: string; value: string; maxHeight?: string }) {
  return (
    <div>
      <div className="mb-1 font-mono text-[10px] text-muted-foreground">{label}</div>
      <pre className={cn('overflow-auto rounded-xl bg-black/25 p-2 font-mono text-[11px] leading-5 text-foreground/85', maxHeight)}>{value || '—'}</pre>
    </div>
  )
}
