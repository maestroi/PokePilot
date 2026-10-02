export interface BroadcastDestination {
  provider: 'twitch' | 'youtube' | 'generic' | string
  label: string
  configured: boolean
  host?: string
  reason?: string
}

export interface BroadcastControlConfig {
  destinations: BroadcastDestination[]
  restart_behavior: 'manual_restart_required' | string
}

export interface BroadcastStatus {
  run_id: string
  state: 'starting' | 'healthy' | 'live' | 'reconnecting' | 'stopped' | 'failed' | string
  provider?: string
  host?: string
  width?: number
  height?: number
  fps?: number
  video_bitrate_kbps?: number
  codec?: string
  preset?: string
  keyframe_interval_seconds?: number
  audio?: boolean
  audio_bitrate_kbps?: number
  audio_sample_rate?: number
  reconnects?: number
  last_error?: string
  started_at_unix_ms?: number
  updated_at_unix_ms?: number
}

export interface BroadcastStartInput {
  provider: string
  width?: number
  height?: number
  fps?: number
  video_bitrate_kbps?: number
  codec?: string
  preset?: string
  keyframe_interval_seconds?: number
  audio?: boolean
  audio_bitrate_kbps?: number
  audio_sample_rate?: number
}

async function errorDetail(response: Response): Promise<string> {
  let detail = `${response.status} ${response.statusText}`.trim()
  try {
    const body = await response.json() as { error?: string }
    if (body.error) detail = body.error
  } catch {
    // Keep the HTTP status when the response is not JSON.
  }
  return detail || 'Request failed'
}

async function requestJSON<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    cache: 'no-store',
    headers: {
      Accept: 'application/json',
      ...(init.body ? { 'Content-Type': 'application/json' } : {}),
      ...(init.headers || {})
    },
    ...init
  })
  if (!response.ok) throw new Error(await errorDetail(response))
  return response.json() as Promise<T>
}

export function getBroadcastControl(signal?: AbortSignal): Promise<BroadcastControlConfig> {
  return requestJSON<BroadcastControlConfig>('/v1/live/broadcast/config', { signal })
}

export function getBroadcastStatus(runID: string, signal?: AbortSignal): Promise<BroadcastStatus> {
  return requestJSON<BroadcastStatus>(`/v1/runs/${encodeURIComponent(runID)}/live/broadcast/status`, { signal })
}

export function startBroadcast(runID: string, input: BroadcastStartInput, signal?: AbortSignal): Promise<BroadcastStatus> {
  return requestJSON<BroadcastStatus>(`/v1/runs/${encodeURIComponent(runID)}/live/broadcast/start`, {
    method: 'POST',
    body: JSON.stringify(input),
    signal
  })
}

export function stopBroadcast(runID: string, signal?: AbortSignal): Promise<BroadcastStatus> {
  return requestJSON<BroadcastStatus>(`/v1/runs/${encodeURIComponent(runID)}/live/broadcast/stop`, {
    method: 'POST',
    body: '{}',
    signal
  })
}
