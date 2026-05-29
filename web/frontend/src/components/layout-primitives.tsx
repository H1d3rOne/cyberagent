import { forwardRef, type HTMLAttributes, type ReactNode } from 'react'
import { RefreshCcw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export const PageHeader = forwardRef<HTMLDivElement, HTMLAttributes<HTMLDivElement>>(function PageHeader({ className, ...props }, ref) {
  return <div ref={ref} className={cn('flex flex-wrap items-start justify-between gap-4', className)} {...props} />
})

export const PageTitle = forwardRef<HTMLHeadingElement, HTMLAttributes<HTMLHeadingElement>>(function PageTitle({ className, ...props }, ref) {
  return <h1 ref={ref} className={cn('text-2xl font-semibold tracking-tight sm:text-[2.15rem] sm:leading-[1.15]', className)} {...props} />
})

export const PageDescription = forwardRef<HTMLParagraphElement, HTMLAttributes<HTMLParagraphElement>>(function PageDescription({ className, ...props }, ref) {
  return <p ref={ref} className={cn('max-w-3xl text-sm leading-7 text-muted-foreground', className)} {...props} />
})

export function Section({ className, ...props }: HTMLAttributes<HTMLElement>) {
  return <section className={cn('glass-panel p-4 sm:p-5', className)} {...props} />
}

export function SectionHeader({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('mb-4 flex items-center justify-between gap-3', className)} {...props} />
}

export function SectionTitle({ className, icon: Icon, ...props }: HTMLAttributes<HTMLHeadingElement> & { icon?: React.ElementType }) {
  return (
    <h2 className={cn('flex items-center gap-2 text-sm font-semibold', className)} {...props}>
      {Icon && <Icon className="h-4 w-4 text-primary" />}
      {props.children}
    </h2>
  )
}

export function EmptyState({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        'grid min-h-[7rem] place-items-center rounded-2xl border border-dashed border-white/10 bg-white/[0.02] text-sm text-muted-foreground',
        className,
      )}
      {...props}
    />
  )
}

export function PageActions({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('flex flex-wrap items-center gap-2', className)} {...props} />
}

export function RefreshButton({ loading, label, onClick, className }: { loading?: boolean; label: string; onClick: () => void; className?: string }) {
  return (
    <Button type="button" variant="outline" onClick={onClick} className={cn('rounded-2xl bg-transparent', className)}>
      <RefreshCcw className={cn('mr-2 h-4 w-4', loading && 'animate-spin')} />
      {label}
    </Button>
  )
}

export function MetricCard({
  label,
  value,
  icon: Icon,
  tone,
  className,
}: {
  label: string
  value: ReactNode
  icon: React.ElementType
  tone: 'cyan' | 'emerald' | 'red' | 'amber' | 'blue'
  className?: string
}) {
  const toneMap = {
    cyan: 'text-cyan-300 bg-cyan-400/10 border-cyan-400/20',
    emerald: 'text-emerald-300 bg-emerald-400/10 border-emerald-400/20',
    red: 'text-red-300 bg-red-400/10 border-red-400/20',
    amber: 'text-amber-300 bg-amber-400/10 border-amber-400/20',
    blue: 'text-blue-300 bg-blue-500/10 border-blue-500/20',
  } as const

  return (
    <div className={cn('glass-panel p-4', className)}>
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs text-muted-foreground">{label}</span>
        <span className={cn('grid h-9 w-9 place-items-center rounded-2xl border', toneMap[tone])}>
          <Icon className="h-4 w-4" />
        </span>
      </div>
      <div className="mt-4 font-mono text-2xl font-semibold sm:text-3xl">{value}</div>
    </div>
  )
}
