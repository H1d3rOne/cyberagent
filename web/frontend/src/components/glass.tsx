import { forwardRef, type ButtonHTMLAttributes, type HTMLAttributes, type Ref } from 'react'
import { cn } from '@/lib/utils'

export type GlassVariant = 'subtle' | 'panel' | 'elevated'
export type GlassTone = 'muted' | 'info' | 'success' | 'warning' | 'danger'
type GlassElement = 'div' | 'section' | 'aside' | 'article' | 'header' | 'footer' | 'button'

const variantClass: Record<GlassVariant, string> = {
  subtle: 'glass-surface-subtle',
  panel: 'glass-surface-panel',
  elevated: 'glass-surface-elevated',
}

const toneClass: Record<GlassTone, string> = {
  muted: '',
  info: 'glass-alert-info',
  success: 'glass-alert-success',
  warning: 'glass-alert-warning',
  danger: 'glass-alert-danger',
}

const statusToneClass: Record<GlassTone, string> = {
  muted: '',
  info: 'glass-status-pill-info',
  success: 'glass-status-pill-success',
  warning: 'glass-status-pill-warning',
  danger: 'glass-status-pill-danger',
}

export interface GlassSurfaceProps extends HTMLAttributes<HTMLElement> {
  as?: GlassElement
  variant?: GlassVariant
  interactive?: boolean
  selected?: boolean
  type?: ButtonHTMLAttributes<HTMLButtonElement>['type']
  disabled?: ButtonHTMLAttributes<HTMLButtonElement>['disabled']
}

export const GlassPanel = forwardRef<HTMLElement, GlassSurfaceProps>(function GlassPanel(
  { as: Component = 'div', className, variant = 'panel', interactive, selected, ...props },
  ref,
) {
  return (
    <Component
      ref={ref as Ref<never>}
      className={cn(
        'glass-surface rounded-3xl',
        variantClass[variant],
        interactive && 'glass-surface-interactive focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
        selected && 'glass-surface-selected',
        className,
      )}
      {...props}
    />
  )
})

export const GlassCard = forwardRef<HTMLElement, GlassSurfaceProps>(function GlassCard(
  { as: Component = 'div', className, variant = 'subtle', interactive, selected, ...props },
  ref,
) {
  return (
    <Component
      ref={ref as Ref<never>}
      className={cn(
        'glass-surface rounded-2xl',
        variantClass[variant],
        interactive && 'glass-surface-interactive focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
        selected && 'glass-surface-selected',
        className,
      )}
      {...props}
    />
  )
})

export interface GlassToolbarProps extends HTMLAttributes<HTMLElement> {
  as?: GlassElement
}

export const GlassToolbar = forwardRef<HTMLElement, GlassToolbarProps>(function GlassToolbar(
  { as: Component = 'div', className, ...props },
  ref,
) {
  return <Component ref={ref as Ref<never>} className={cn('glass-toolbar', className)} {...props} />
})

export interface GlassAlertProps extends HTMLAttributes<HTMLElement> {
  as?: GlassElement
  tone?: GlassTone
}

export const GlassAlert = forwardRef<HTMLElement, GlassAlertProps>(function GlassAlert(
  { as: Component = 'div', className, tone = 'muted', ...props },
  ref,
) {
  return <Component ref={ref as Ref<never>} className={cn('glass-alert', toneClass[tone], className)} {...props} />
})

export interface GlassDialogProps extends HTMLAttributes<HTMLDivElement> {
  modal?: boolean
}

export const GlassDialog = forwardRef<HTMLDivElement, GlassDialogProps>(function GlassDialog(
  { className, modal = true, role = 'dialog', ...props },
  ref,
) {
  return (
    <div
      ref={ref}
      role={role}
      aria-modal={modal || undefined}
      className={cn('glass-surface glass-surface-elevated rounded-3xl', className)}
      {...props}
    />
  )
})

export interface GlassDrawerProps extends HTMLAttributes<HTMLElement> {
  modal?: boolean
}

export const GlassDrawer = forwardRef<HTMLElement, GlassDrawerProps>(function GlassDrawer(
  { className, modal = true, role = 'dialog', ...props },
  ref,
) {
  return (
    <aside
      ref={ref}
      role={role}
      aria-modal={modal || undefined}
      className={cn('glass-surface glass-surface-elevated', className)}
      {...props}
    />
  )
})

export interface StatusPillProps extends HTMLAttributes<HTMLSpanElement> {
  tone?: GlassTone
}

export const StatusPill = forwardRef<HTMLSpanElement, StatusPillProps>(function StatusPill(
  { className, tone = 'muted', ...props },
  ref,
) {
  return (
    <span
      ref={ref}
      className={cn('glass-status-pill inline-flex items-center gap-1.5 rounded-full px-2 py-1 font-mono text-[10px]', statusToneClass[tone], className)}
      {...props}
    />
  )
})
