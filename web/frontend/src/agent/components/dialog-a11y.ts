import { useEffect, useRef, type RefObject } from 'react'

const FOCUSABLE_SELECTOR = [
  'a[href]',
  'button:not([disabled])',
  'textarea:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
  '[contenteditable="true"]',
].join(',')

export interface DialogA11yOptions {
  open?: boolean
  modal?: boolean
  initialFocusSelector?: string
  onClose: () => void
}

export function useDialogA11y<T extends HTMLElement>({
  open = true,
  modal = true,
  initialFocusSelector,
  onClose,
}: DialogA11yOptions): RefObject<T> {
  const containerRef = useRef<T>(null)
  const previousFocusRef = useRef<HTMLElement | null>(null)

  useEffect(() => {
    if (!open || typeof document === 'undefined') return undefined

    previousFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const focusDialog = () => {
      const container = containerRef.current
      if (!container) return
      const target = initialFocusSelector
        ? container.querySelector<HTMLElement>(initialFocusSelector)
        : firstFocusableElement(container)
      ;(target || container).focus({ preventScroll: true })
    }

    const frame = window.requestAnimationFrame(focusDialog)
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        onClose()
        return
      }
      if (!modal || event.key !== 'Tab') return

      const container = containerRef.current
      if (!container) return
      const focusable = focusableElements(container)
      if (!focusable.length) {
        event.preventDefault()
        container.focus({ preventScroll: true })
        return
      }

      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      const active = document.activeElement
      if (event.shiftKey && (active === first || !container.contains(active))) {
        event.preventDefault()
        last.focus({ preventScroll: true })
      } else if (!event.shiftKey && active === last) {
        event.preventDefault()
        first.focus({ preventScroll: true })
      }
    }

    document.addEventListener('keydown', onKeyDown)
    return () => {
      window.cancelAnimationFrame(frame)
      document.removeEventListener('keydown', onKeyDown)
      window.setTimeout(() => {
        const previous = previousFocusRef.current
        if (previous && document.contains(previous)) previous.focus({ preventScroll: true })
      }, 0)
    }
  }, [initialFocusSelector, modal, onClose, open])

  return containerRef
}

function firstFocusableElement(container: HTMLElement) {
  return focusableElements(container)[0]
}

function focusableElements(container: HTMLElement) {
  return Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR))
    .filter((element) => !element.hasAttribute('disabled') && element.tabIndex !== -1 && isVisible(element))
}

function isVisible(element: HTMLElement) {
  if (element.hidden) return false
  const style = window.getComputedStyle(element)
  return style.visibility !== 'hidden' && style.display !== 'none'
}
