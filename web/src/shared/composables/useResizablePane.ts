import { computed, ref } from 'vue'

// A docked detail pane the operator can drag (or arrow-key) wider or narrower.
// The width is remembered per page. Bind `handle` onto a thin element on the
// pane's left edge; the parent grid reads `width`.
export function useResizablePane(storageKey: string, { initial = 560, min = 400, max = 840 } = {}) {
  function clamp(value: number): number {
    const room = typeof window === 'undefined' ? max : Math.max(min, window.innerWidth - 560)
    return Math.round(Math.min(max, room, Math.max(min, value)))
  }
  function read(): number {
    try {
      const stored = Number(window.localStorage.getItem(storageKey))
      return Number.isFinite(stored) && stored > 0 ? clamp(stored) : initial
    } catch {
      return initial
    }
  }
  function remember(): void {
    try { window.localStorage.setItem(storageKey, String(width.value)) } catch { /* private mode: width just isn't kept */ }
  }
  const width = ref(read())

  let drag: { pointerId: number; startX: number; startWidth: number } | null = null
  function onPointerdown(event: PointerEvent): void {
    if (event.button !== 0) return
    event.preventDefault()
    drag = { pointerId: event.pointerId, startX: event.clientX, startWidth: width.value }
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    try { (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId) } catch { /* pointer may already be gone */ }
  }
  function onPointermove(event: PointerEvent): void {
    if (!drag || event.pointerId !== drag.pointerId) return
    width.value = clamp(drag.startWidth + (drag.startX - event.clientX))
  }
  function onPointerup(event: PointerEvent): void {
    if (!drag || event.pointerId !== drag.pointerId) return
    drag = null
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
    remember()
  }
  function onDblclick(): void {
    width.value = initial
    remember()
  }
  function onKeydown(event: KeyboardEvent): void {
    const step = event.shiftKey ? 80 : 24
    if (event.key === 'ArrowLeft') width.value = clamp(width.value + step)
    else if (event.key === 'ArrowRight') width.value = clamp(width.value - step)
    else if (event.key === 'Home') width.value = initial
    else return
    event.preventDefault()
    event.stopPropagation()
    remember()
  }

  const handle = computed(() => ({
    role: 'separator',
    'aria-orientation': 'vertical' as const,
    tabindex: 0,
    title: 'Drag to resize. Double-click to reset.',
    'aria-valuenow': width.value,
    'aria-valuemin': min,
    'aria-valuemax': max,
    onPointerdown, onPointermove, onPointerup, onPointercancel: onPointerup, onDblclick, onKeydown
  }))
  return { width, handle }
}
