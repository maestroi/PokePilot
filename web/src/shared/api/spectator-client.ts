import type { SpectatorSnapshot } from './spectator'
import { parseSemanticReplay, type SemanticReplayTimeline } from '../semanticReplay'

export async function getSpectatorSnapshot(signal?: AbortSignal): Promise<SpectatorSnapshot> {
  const response = await fetch('/v1/watch', {
    method: 'GET',
    headers: { Accept: 'application/json' },
    cache: 'no-store',
    signal
  })

  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`.trim()
    try {
      const body = await response.json() as { error?: string }
      if (body.error) message = body.error
    } catch {
      // Keep the HTTP status when the body is not JSON.
    }
    throw new Error(message || 'Spectator feed unavailable')
  }

  return response.json() as Promise<SpectatorSnapshot>
}

export function spectatorReplayVideoURL(runID: string): string {
  return `/v1/watch/runs/${encodeURIComponent(runID)}/replay/video`
}


export async function getSpectatorSemanticReplay(runID: string, signal?: AbortSignal): Promise<SemanticReplayTimeline> {
  const response = await fetch(`/v1/watch/runs/${encodeURIComponent(runID)}/replay/semantic`, {
    method: 'GET',
    headers: { Accept: 'application/json' },
    cache: 'no-store',
    signal
  })
  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`.trim()
    try {
      const body = await response.json() as { error?: string }
      if (body.error) message = body.error
    } catch {
      // Keep the HTTP status for a non-JSON failure.
    }
    throw new Error(message || 'Semantic replay unavailable')
  }
  return parseSemanticReplay(await response.json())
}
