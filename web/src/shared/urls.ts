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

export function replayPath(runID = ''): string {
  const id = runID.trim()
  return id ? `/replays/${encodeURIComponent(id)}` : '/replays'
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

export function replayURL(base: string, runID = ''): string {
  const origin = normalizeBase(base)
  if (!origin) return replayPath(runID)
  return `${origin}${replayPath(runID)}`
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


// Private operator routing is independent from public /runs and /replays URLs.
// Keep the hash for the workspace and query parameters for durable resource IDs.
export const operatorPrimaryViews = ['live', 'runs', 'failures', 'analytics', 'media', 'operations'] as const
export type OperatorView = typeof operatorPrimaryViews[number] | 'settings' | 'new-run'
export type OperatorRoute = { view: OperatorView; section: string }
export type OperatorLinkOptions = {
  section?: string
  run?: string
  output?: string
  job?: string
  experiment?: string
}

const operatorSections: Partial<Record<OperatorView, readonly string[]>> = {
  analytics: ['overview', 'experiments'],
  media: ['library', 'studio', 'broadcasts', 'jobs', 'themes'],
  settings: ['models']
}

export function operatorRoute(hash: string): OperatorRoute {
  const [rawPage = '', rawSection = ''] = hash.replace(/^#\/?/, '').split('/')
  const page = rawPage === 'tools' ? 'new-run' : rawPage
  const view: OperatorView = [...operatorPrimaryViews, 'settings', 'new-run'].includes(page)
    ? page as OperatorView
    : 'live'
  const alias = view === 'media' ? ({ replays: 'library', 'render-jobs': 'jobs' } as Record<string, string>)[rawSection] : undefined
  const requested = alias || rawSection
  const allowed = operatorSections[view]
  return { view, section: allowed?.includes(requested) ? requested : (allowed?.[0] || '') }
}

// An empty string explicitly clears a context key; omitted keys remain intact.
// Relative query/hash URLs preserve the current operator pathname.
export function operatorHref(view: OperatorView, options: OperatorLinkOptions = {}, search = ''): string {
  const params = new URLSearchParams(search)
  for (const key of ['run', 'output', 'job', 'experiment'] as const) {
    const value = options[key]
    if (value === undefined) continue
    if (value) params.set(key, value)
    else params.delete(key)
  }
  const route = operatorRoute('#' + view + (options.section ? '/' + options.section : ''))
  const section = options.section && route.section === options.section ? '/' + route.section : ''
  const query = params.toString()
  return (query ? '?' + query : '') + '#' + view + section
}
