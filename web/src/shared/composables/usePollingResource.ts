import { computed, onMounted, onScopeDispose, ref, shallowRef } from 'vue'
import type { ResourceState } from '../resource'

export interface PollingResourceOptions<T> {
  intervalMs?: number
  immediate?: boolean
  isEmpty?: (value: T) => boolean
}

export function usePollingResource<T>(
  load: (signal: AbortSignal) => Promise<T>,
  options: PollingResourceOptions<T> = {}
) {
  const data = shallowRef<T | null>(null)
  const error = ref('')
  const refreshing = ref(false)
  const lastUpdatedAt = ref<number | null>(null)

  const intervalMs = Math.max(1000, options.intervalMs ?? 5000)
  const immediate = options.immediate ?? true
  const isEmpty = options.isEmpty ?? (() => false)

  let timer: ReturnType<typeof setInterval> | null = null
  let requestID = 0
  let controller: AbortController | null = null

  const state = computed<ResourceState>(() => {
    if (data.value === null && refreshing.value) return 'loading'
    if (data.value === null && error.value) return 'error'
    if (data.value !== null && error.value) return 'stale'
    if (data.value !== null && refreshing.value) return 'refreshing'
    if (data.value !== null && isEmpty(data.value)) return 'empty'
    return data.value === null ? 'loading' : 'ready'
  })

  async function refresh(force = false): Promise<void> {
    if (refreshing.value && !force) return

    const id = ++requestID
    if (force) controller?.abort()
    const nextController = new AbortController()
    controller = nextController
    refreshing.value = true
    error.value = ''

    try {
      const next = await load(nextController.signal)
      if (id !== requestID) return
      data.value = next
      lastUpdatedAt.value = Date.now()
    } catch (cause) {
      if (nextController.signal.aborted || id !== requestID) return
      error.value = cause instanceof Error ? cause.message : 'Request failed'
    } finally {
      if (id === requestID) refreshing.value = false
    }
  }

  function retry(): Promise<void> {
    return refresh(true)
  }

  function start(): void {
    if (timer !== null) return
    if (immediate && data.value === null) void refresh()
    timer = setInterval(() => void refresh(), intervalMs)
  }

  function stop(): void {
    if (timer !== null) clearInterval(timer)
    timer = null
    requestID++
    controller?.abort()
    controller = null
    refreshing.value = false
  }

  onMounted(start)
  onScopeDispose(stop)

  return {
    data,
    error,
    lastUpdatedAt,
    refreshing,
    state,
    refresh,
    retry,
    start,
    stop
  }
}
