import type { TimelineEventView } from '@/agent/state/store'

export interface TimelineWindowOptions {
  initialLimit: number
  step: number
  max?: number
  focusIndex?: number
  anchor?: 'head' | 'tail'
}

export interface TimelineWindowResult<T> {
  items: T[]
  total: number
  hiddenBefore: number
  hiddenAfter: number
  visibleCount: number
  canShowMoreBefore: boolean
  canShowMoreAfter: boolean
}

export function windowTimelineItems<T>(items: T[], options: TimelineWindowOptions): TimelineWindowResult<T> {
  const total = items.length
  const visibleCount = clampLimit(options.initialLimit, total, options.max)

  if (!total || !visibleCount) {
    return {
      items: [],
      total,
      hiddenBefore: 0,
      hiddenAfter: total,
      visibleCount: 0,
      canShowMoreBefore: false,
      canShowMoreAfter: total > 0,
    }
  }

  const focusIndex = normalizedFocusIndex(options.focusIndex, total)
  const anchor = options.anchor || 'tail'
  let start = 0

  if (focusIndex !== undefined) {
    const before = Math.floor((visibleCount - 1) / 2)
    start = clamp(focusIndex - before, 0, Math.max(0, total - visibleCount))
  } else if (anchor === 'tail') {
    start = Math.max(0, total - visibleCount)
  }

  const end = Math.min(total, start + visibleCount)
  return {
    items: items.slice(start, end),
    total,
    hiddenBefore: start,
    hiddenAfter: Math.max(0, total - end),
    visibleCount: end - start,
    canShowMoreBefore: start > 0,
    canShowMoreAfter: end < total,
  }
}

export function nextTimelineLimit(current: number, total: number, step: number): number {
  const safeTotal = Math.max(0, Math.floor(total))
  if (!safeTotal) return 0
  const safeCurrent = Math.max(0, Math.floor(current))
  const safeStep = Math.max(1, Math.floor(step))
  return Math.min(safeTotal, safeCurrent + safeStep)
}

export function buildEventLookup(events: TimelineEventView[]): Map<string, TimelineEventView> {
  const lookup = new Map<string, TimelineEventView>()
  for (const event of events) lookup.set(event.id, event)
  return lookup
}

function clampLimit(limit: number, total: number, max?: number) {
  const safeTotal = Math.max(0, Math.floor(total))
  const safeLimit = Math.max(0, Math.floor(limit))
  const safeMax = typeof max === 'number' ? Math.max(0, Math.floor(max)) : safeTotal
  return Math.min(safeTotal, safeLimit, safeMax)
}

function normalizedFocusIndex(focusIndex: number | undefined, total: number) {
  if (typeof focusIndex !== 'number' || !Number.isFinite(focusIndex)) return undefined
  if (focusIndex < 0 || focusIndex >= total) return undefined
  return Math.floor(focusIndex)
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value))
}
