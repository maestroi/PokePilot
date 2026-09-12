import { onMounted, onScopeDispose, ref, shallowRef, watch, type Ref } from 'vue'

export type FramePumpState = 'idle' | 'loading' | 'ready' | 'error'

export function useFramePump(runID: Ref<string>, enabled: Ref<boolean>, intervalMs = 50) {
  const frameURL = shallowRef('')
  const state = ref<FramePumpState>('idle')
  const error = ref('')

  let serial = 0
  let timer: ReturnType<typeof setTimeout> | null = null
  let controller: AbortController | null = null

  function releaseFrame(): void {
    if (!frameURL.value) return
    URL.revokeObjectURL(frameURL.value)
    frameURL.value = ''
  }

  function stopPump(): void {
    serial++
    if (timer !== null) clearTimeout(timer)
    timer = null
    controller?.abort()
    controller = null
  }

  async function tick(id: number): Promise<void> {
    if (id !== serial || !enabled.value || !runID.value) return
    const currentRunID = runID.value
    const request = new AbortController()
    controller = request
    if (!frameURL.value) state.value = 'loading'

    try {
      const response = await fetch(`/frame?run=${encodeURIComponent(currentRunID)}`, {
        cache: 'no-store',
        signal: request.signal
      })
      if (!response.ok) throw new Error(response.status === 404 ? 'Waiting for a live frame' : `Frame request failed (${response.status})`)
      const contentType = response.headers.get('Content-Type') || ''
      if (!contentType.startsWith('image/')) throw new Error('Frame endpoint returned non-image data')
      const blob = await response.blob()
      if (id !== serial || request.signal.aborted || currentRunID !== runID.value) return

      const nextURL = URL.createObjectURL(blob)
      const previousURL = frameURL.value
      frameURL.value = nextURL
      if (previousURL) URL.revokeObjectURL(previousURL)
      error.value = ''
      state.value = 'ready'
    } catch (cause) {
      if (!request.signal.aborted && id === serial) {
        error.value = cause instanceof Error ? cause.message : 'Frame request failed'
        state.value = 'error'
      }
    } finally {
      if (id === serial && enabled.value && runID.value === currentRunID) {
        timer = setTimeout(() => void tick(id), Math.max(50, intervalMs))
      }
    }
  }

  function restart(): void {
    stopPump()
    error.value = ''
    releaseFrame()
    if (!enabled.value || !runID.value) {
      state.value = 'idle'
      return
    }
    state.value = 'loading'
    const id = serial
    void tick(id)
  }

  watch([runID, enabled], restart)
  onMounted(restart)
  onScopeDispose(() => {
    stopPump()
    releaseFrame()
  })

  return { frameURL, state, error, restart }
}
