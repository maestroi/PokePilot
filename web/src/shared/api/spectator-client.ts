import type { SpectatorSnapshot } from './spectator'

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
