export const PRODUCTION_PUBLIC_ORIGIN = 'https://rompilot.app'
export const PRODUCTION_ADMIN_ORIGIN = 'https://admin.rompilot.app'
export const PRODUCTION_API_ORIGIN = 'https://api.rompilot.app'
export const LEGACY_PUBLIC_HOST = 'pokemon.maestroi.cc'

export interface ExternalURLs {
  spectator_url?: string
  public_base_url?: string
  admin_base_url?: string
  api_base_url?: string
}

export function normalizeBase(raw?: string): string {
  const value = raw?.trim().replace(/\/+$/, '') || ''
  if (!value) return ''
  try {
    const parsed = new URL(value)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return ''
    parsed.pathname = ''
    parsed.search = ''
    parsed.hash = ''
    return parsed.toString().replace(/\/+$/, '')
  } catch {
    return ''
  }
}

export function publicBaseURL(config: ExternalURLs = {}): string {
  return normalizeBase(config.public_base_url) || normalizeBase(config.spectator_url)
}

export function spectatorRunPath(runID = ''): string {
  const id = runID.trim()
  return id ? `/runs/${encodeURIComponent(id)}` : '/'
}

export function runIDFromLocation(pathname: string, search = ''): string {
  const prefix = '/runs/'
  if (pathname.startsWith(prefix)) {
    const raw = pathname.slice(prefix.length).split('/').filter(Boolean)[0] || ''
    try {
      return decodeURIComponent(raw)
    } catch {
      return ''
    }
  }
  return new URLSearchParams(search).get('run')?.trim() || ''
}

export function spectatorURL(base: string, runID = ''): string {
  const origin = normalizeBase(base)
  if (!origin) return spectatorRunPath(runID)
  return `${origin}${spectatorRunPath(runID)}`
}

export function localSpectatorBase(currentHref: string): string {
  try {
    const current = new URL(currentHref)
    if (current.port !== '18080') return ''
    current.port = '18081'
    current.pathname = '/'
    current.search = ''
    current.hash = ''
    return current.toString().replace(/\/+$/, '')
  } catch {
    return ''
  }
}

export function resolveSpectatorBase(config: ExternalURLs, currentHref: string): string {
  return publicBaseURL(config) || localSpectatorBase(currentHref)
}

export function liveHTTPURL(base: string, path: string): string {
  const origin = normalizeBase(base)
  const suffix = path.startsWith('/') ? path : `/${path}`
  return origin ? `${origin}${suffix}` : suffix
}

export function websocketURL(httpBase: string, path = ''): string {
  const absolute = liveHTTPURL(httpBase, path || '/')
  if (absolute.startsWith('/')) return absolute
  const url = new URL(absolute)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return url.toString()
}
