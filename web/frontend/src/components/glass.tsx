import { forwardRef, type HTMLAttributes } from 'react'
import { cn } from '@/lib/utils'

export type GlassVariant = 'subtle' | 'panel' | 'elevated'
export type GlassTone = 'muted' | 'info' | 'success' | 'warning' | 'danger'

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

export interface GlassSurfaceProps extends HTMLAttributes<HTMLDivElement> {
  variant?: GlassVariant
  interactive?: boolean
  selected?: boolean
}

export const GlassPanel = forwardRef<HTMLDivElement, GlassSurfaceProps>(function GlassPanel(
  { className, variant = 'panel', interactive, selected, ...props },
  ref,
) {
  return (
    <div
      ref={ref}
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

export const GlassCard = forwardRef<HTMLDivElement, GlassSurfaceProps>(function GlassCard(
  { className, variant = 'subtle', interactive, selected, ...props },
  ref,
) {
  return (
    <div
      ref={ref}
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

export const GlassToolbar = forwardRef<HTMLDivElement, HTMLAttributes<HTMLDivElement>>(function GlassToolbar(
  { className, ...props },
  ref,
) {
  return <div ref={ref} className={cn('glass-toolbar', className)} {...props} />
})

export interface GlassAlertProps extends HTMLAttributes<HTMLDivElement> {
  tone?: GlassTone
}

export const GlassAlert = forwardRef<HTMLDivElement, GlassAlertProps>(function GlassAlert(
  { className, tone = 'muted', ...props },
  ref,
) {
  return <div ref={ref} className={cn('glass-alert', toneClass[tone], className)} {...props} />
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
