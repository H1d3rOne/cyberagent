import { describe, expect, it } from 'vitest'
import type { TimelineEventView } from '@/agent/state/store'
import { buildEventLookup, nextTimelineLimit, windowTimelineItems } from './timeline-windowing'

describe('timeline windowing helpers', () => {
  it('defaults to a tail window for recent timeline items', () => {
    const result = windowTimelineItems(range(10), { initialLimit: 4, step: 4, anchor: 'tail' })

    expect(result.items).toEqual([6, 7, 8, 9])
    expect(result.total).toBe(10)
    expect(result.hiddenBefore).toBe(6)
    expect(result.hiddenAfter).toBe(0)
    expect(result.visibleCount).toBe(4)
    expect(result.canShowMoreBefore).toBe(true)
    expect(result.canShowMoreAfter).toBe(false)
  })

  it('supports a head window for chronological groups', () => {
    const result = windowTimelineItems(range(10), { initialLimit: 3, step: 3, anchor: 'head' })

    expect(result.items).toEqual([0, 1, 2])
    expect(result.hiddenBefore).toBe(0)
    expect(result.hiddenAfter).toBe(7)
    expect(result.canShowMoreBefore).toBe(false)
    expect(result.canShowMoreAfter).toBe(true)
  })

  it('keeps a focused item inside the rendered window', () => {
    const result = windowTimelineItems(range(10), { initialLimit: 4, step: 4, focusIndex: 2, anchor: 'tail' })

    expect(result.items).toEqual([1, 2, 3, 4])
    expect(result.items).toContain(2)
    expect(result.hiddenBefore).toBe(1)
    expect(result.hiddenAfter).toBe(5)
  })

  it('clamps focus windows near the end of the timeline', () => {
    const result = windowTimelineItems(range(10), { initialLimit: 4, step: 4, focusIndex: 9, anchor: 'head' })

    expect(result.items).toEqual([6, 7, 8, 9])
    expect(result.hiddenBefore).toBe(6)
    expect(result.hiddenAfter).toBe(0)
  })

  it('increments limits without exceeding the total', () => {
    expect(nextTimelineLimit(30, 80, 30)).toBe(60)
    expect(nextTimelineLimit(60, 80, 30)).toBe(80)
    expect(nextTimelineLimit(0, 5, 0)).toBe(1)
    expect(nextTimelineLimit(5, 0, 10)).toBe(0)
  })

  it('applies an optional max cap before slicing', () => {
    const result = windowTimelineItems(range(5), { initialLimit: 5, step: 5, max: 2, anchor: 'tail' })

    expect(result.items).toEqual([3, 4])
    expect(result.hiddenBefore).toBe(3)
    expect(result.visibleCount).toBe(2)
  })

  it('builds an id lookup and lets later events replace stale cache entries', () => {
    const first = event('same', 'started')
    const latest = event('same', 'finished')
    const lookup = buildEventLookup([first, event('other', 'progress'), latest])

    expect(lookup.size).toBe(2)
    expect(lookup.get('same')).toBe(latest)
  })
})

function range(length: number) {
  return Array.from({ length }, (_, index) => index)
}

function event(id: string, type: string): TimelineEventView {
  return {
    id,
    type,
    timestamp: `2026-06-09T00:00:0${id.length}Z`,
  }
}
