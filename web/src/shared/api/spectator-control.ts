export interface SpectatorRunControl {
  visible: boolean
}

export interface SpectatorControlSnapshot {
  featured_run_id?: string
  runs: Record<string, SpectatorRunControl>
}

export interface SpectatorRunControlResult {
  run_id: string
  visible: boolean
  featured: boolean
  featured_run_id?: string
}

export interface OperatorUIConfig {
  spectator_url?: string
  public_base_url?: string
  admin_base_url?: string
  api_base_url?: string
}

async function errorDetail(response: Response): Promise<string> {
  try {
    const body = await response.json() as { error?: string }
    if (body.error) return body.error
  } catch {
    // Fall through to the HTTP status below.
  }
  return `${response.status} ${response.statusText}`.trim() || 'Request failed'
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

export function getSpectatorControl(signal?: AbortSignal): Promise<SpectatorControlSnapshot> {
  return requestJSON<SpectatorControlSnapshot>('/v1/spectator/control', { signal })
}

export function patchSpectatorRunControl(
  runID: string,
  patch: { visible?: boolean; featured?: boolean },
  signal?: AbortSignal
): Promise<SpectatorRunControlResult> {
  return requestJSON<SpectatorRunControlResult>(`/v1/runs/${encodeURIComponent(runID)}/spectator`, {
    method: 'PATCH',
    body: JSON.stringify(patch),
    signal
  })
}

export function getOperatorUIConfig(signal?: AbortSignal): Promise<OperatorUIConfig> {
  return requestJSON<OperatorUIConfig>('/v1/ui-config', { signal })
}
