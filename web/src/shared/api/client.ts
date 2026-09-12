import type { DashboardQuery, DashboardSnapshot } from './types'

export class ApiError extends Error {
  readonly status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

function queryString(params: DashboardQuery): string {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === '' || value === false) continue
    query.set(key, value === true ? '1' : String(value))
  }
  const encoded = query.toString()
  return encoded ? `?${encoded}` : ''
}

async function errorDetail(response: Response): Promise<string> {
  let detail = `${response.status} ${response.statusText}`.trim()
  try {
    const body = await response.json() as { error?: string }
    if (body.error) detail = body.error
  } catch {
    // Keep the status text when the response is not JSON.
  }
  return detail || 'Request failed'
}

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, {
    method: 'GET',
    headers: { Accept: 'application/json' },
    cache: 'no-store',
    signal
  })

  if (!response.ok) throw new ApiError(await errorDetail(response), response.status)
  return response.json() as Promise<T>
}

export function getDashboard(params: DashboardQuery = {}, signal?: AbortSignal): Promise<DashboardSnapshot> {
  return getJSON<DashboardSnapshot>(`/v1/dashboard${queryString(params)}`, signal)
}

export async function deleteRun(runID: string, signal?: AbortSignal): Promise<void> {
  const response = await fetch(`/v1/runs/${encodeURIComponent(runID)}`, {
    method: 'DELETE',
    headers: { Accept: 'application/json' },
    cache: 'no-store',
    signal
  })
  if (!response.ok) throw new ApiError(await errorDetail(response), response.status)
}
