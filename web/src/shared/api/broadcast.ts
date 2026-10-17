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
  music_track_id?: string
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
  music_track_id?: string
}

export interface BroadcastMusicTrack {
  id: string
  name: string
  format: string
  size: number
  uploaded_at_unix_ms: number
}

export interface BroadcastMusicLibrary {
  tracks: BroadcastMusicTrack[]
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

export function getBroadcastMusic(signal?: AbortSignal): Promise<BroadcastMusicLibrary> {
  return requestJSON<BroadcastMusicLibrary>('/v1/live/broadcast/music', { signal })
}

export async function uploadBroadcastMusic(file: File, licensed: boolean): Promise<BroadcastMusicTrack> {
  if (!licensed) throw new Error('Confirm you have permission to broadcast this music')
  if (file.size === 0 || file.size > 20 * 1024 * 1024) throw new Error('Music must be between 1 byte and 20 MiB')
  const path = `/v1/live/broadcast/music?name=${encodeURIComponent(file.name)}&licensed=true`
  const response = await fetch(path, {
    method: 'POST',
    cache: 'no-store',
    headers: { Accept: 'application/json', 'Content-Type': file.type || 'application/octet-stream' },
    body: file
  })
  if (!response.ok) throw new Error(await errorDetail(response))
  return response.json() as Promise<BroadcastMusicTrack>
}

export interface LiveCommentarySettings {
  enabled: boolean
  voice: string
  tts: boolean
  style: 'documentary' | 'sports' | 'calm'
  cadence_seconds: number
  verbosity: 'sparse' | 'normal'
}

export interface LiveCommentaryEmission {
  policy: string
  run_id: string
  attempt: number
  event_id: string
  event_type: string
  frame: number
  timestamp_ms: number
  emitted_at: string
  action: 'comment' | 'recap' | 'milestone' | 'comparison'
  text: string
  model: string
  style: string
  tts_state?: string
}

export interface LiveCommentaryStatus {
  run_id: string
  state: string
  settings: LiveCommentarySettings
  model: string
  tts_configured: boolean
  emitted: number
  recent?: LiveCommentaryEmission[]
  last_error?: string
}

export function getLiveCommentaryStatus(runID: string, signal?: AbortSignal): Promise<LiveCommentaryStatus> {
  return requestJSON<LiveCommentaryStatus>(`/v1/runs/${encodeURIComponent(runID)}/live/commentary/status`, { signal })
}

export function startLiveCommentary(runID: string, settings: LiveCommentarySettings): Promise<LiveCommentaryStatus> {
  return requestJSON<LiveCommentaryStatus>(`/v1/runs/${encodeURIComponent(runID)}/live/commentary/start`, {
    method: 'POST', body: JSON.stringify(settings)
  })
}

export function stopLiveCommentary(runID: string): Promise<LiveCommentaryStatus> {
  return requestJSON<LiveCommentaryStatus>(`/v1/runs/${encodeURIComponent(runID)}/live/commentary/stop`, {
    method: 'POST', body: '{}'
  })
}
