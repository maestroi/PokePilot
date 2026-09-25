import { onMounted, onScopeDispose, ref, shallowRef, watch, type Ref } from 'vue'
import type { RenderState } from '../api/renderstate'

export type RenderStatePumpStatus = 'idle' | 'loading' | 'ready' | 'error'

export function useRenderStatePump(
  runID: Ref<string>,
  enabled: Ref<boolean>,
  intervalMs = 100,
  continuous?: Ref<boolean>
) {
  const renderState = shallowRef<RenderState | null>(null)
  const state = ref<RenderStatePumpStatus>('idle')
  const error = ref('')

  let serial = 0
  let timer: ReturnType<typeof setTimeout> | null = null
  let controller: AbortController | null = null
  let stopVisibility: (() => void) | null = null

  function isContinuous(): boolean {
    return continuous ? continuous.value : true
  }

  function stopPump(): void {
    serial++
    if (timer !== null) clearTimeout(timer)
    timer = null
    controller?.abort()
    controller = null
    stopVisibility?.()
    stopVisibility = null
  }

  function whenVisible(): Promise<void> {
    if (typeof document === 'undefined' || !document.hidden) return Promise.resolve()
    return new Promise((resolve) => {
      const onChange = () => {
        if (document.hidden) return
        document.removeEventListener('visibilitychange', onChange)
        if (stopVisibility === stop) stopVisibility = null
        resolve()
      }
      const stop = () => {
        document.removeEventListener('visibilitychange', onChange)
        resolve()
      }
      stopVisibility = stop
      document.addEventListener('visibilitychange', onChange)
    })
  }

  function validPayload(value: unknown): value is RenderState {
    if (!value || typeof value !== 'object') return false
    const candidate = value as Partial<RenderState>
    return candidate.schema_version === 1 &&
      typeof candidate.scene === 'string' &&
      Boolean(candidate.game && typeof candidate.game.id === 'string' && typeof candidate.game.revision === 'string') &&
      Boolean(candidate.clock && Number.isFinite(candidate.clock.frame))
  }

  async function tick(id: number): Promise<void> {
    if (id !== serial || !enabled.value || !runID.value) return
    await whenVisible()
    if (id !== serial || !enabled.value || !runID.value) return

    const currentRunID = runID.value
    const request = new AbortController()
    controller = request
    if (!renderState.value) state.value = 'loading'

    try {
      const response = await fetch(`/render-state?run=${encodeURIComponent(currentRunID)}`, {
        cache: 'no-store',
        headers: { Accept: 'application/json' },
        signal: request.signal
      })
      if (!response.ok) {
        throw new Error(response.status === 404 || response.status === 503
          ? 'Modern renderer is not available for this live state'
          : `Render-state request failed (${response.status})`)
      }
      const payload: unknown = await response.json()
      if (!validPayload(payload)) throw new Error('Render-state endpoint returned an unsupported payload')
      if (id !== serial || request.signal.aborted || currentRunID !== runID.value) return

      renderState.value = payload
      error.value = ''
      state.value = 'ready'
    } catch (cause) {
      if (!request.signal.aborted && id === serial) {
        error.value = cause instanceof Error ? cause.message : 'Render-state request failed'
        state.value = 'error'
      }
    } finally {
      if (id === serial && enabled.value && runID.value === currentRunID && isContinuous()) {
        timer = setTimeout(() => void tick(id), Math.max(100, intervalMs))
      }
    }
  }

  function restart(preserveState = false): void {
    stopPump()
    error.value = ''
    if (!preserveState) renderState.value = null
    if (!enabled.value || !runID.value) {
      state.value = 'idle'
      return
    }
    state.value = 'loading'
    const id = serial
    void tick(id)
  }

  watch([runID, enabled], () => restart())
  if (continuous) watch(continuous, () => restart(true))
  onMounted(() => restart())
  onScopeDispose(stopPump)

  return { renderState, state, error, restart }
}
