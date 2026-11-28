/** OpenRouter publishes USD-per-token list prices (not USD-per-million). */
export type HostedApiPrice = {
  id: string
  provider: string
  model: string
  inputPerMillion: number
  outputPerMillion: number
  contextLength: number | null
  created: number | null
}

export type HostedApiEstimate = HostedApiPrice & { costUsd: number }

export type HostedApiCatalog = {
  models: HostedApiPrice[]
  fetchedAt: number
  /** True if a failed refresh returned previously fetched prices. */
  stale: boolean
}

export const OPENROUTER_MODELS_URL = 'https://openrouter.ai/api/v1/models?sort=most-popular'
const CATALOG_TTL_MS = 60 * 60 * 1000

const PROVIDER_LABELS: Record<string, string> = {
  anthropic: 'Anthropic',
  openai: 'OpenAI',
  google: 'Google',
  'x-ai': 'xAI',
  deepseek: 'DeepSeek',
  meta: 'Meta',
  mistralai: 'Mistral',
  qwen: 'Qwen',
  moonshotai: 'Moonshot AI'
}

function object(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null
}

function tokenPrice(value: unknown): number | null {
  if ((typeof value !== 'string' || !value.trim()) && typeof value !== 'number') return null
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed >= 0 ? parsed * 1_000_000 : null
}

function tokenCount(value: unknown): number {
  const parsed = Number(value || 0)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0
}

/** Ignore non-text, incomplete and unpriced catalog records. Preserve OpenRouter popularity order. */
export function parseOpenRouterModels(payload: unknown): HostedApiPrice[] {
  const rows = object(payload)?.data
  if (!Array.isArray(rows)) throw new Error('OpenRouter returned an invalid model catalog')

  const models: HostedApiPrice[] = []
  const seen = new Set<string>()
  for (const entry of rows) {
    const row = object(entry)
    if (!row || typeof row.id !== 'string' || !row.id.includes('/') || seen.has(row.id)) continue
    const architecture = object(row.architecture)
    if (!Array.isArray(architecture?.input_modalities) || !architecture.input_modalities.includes('text')) continue
    if (!Array.isArray(architecture?.output_modalities) || !architecture.output_modalities.includes('text')) continue
    const pricing = object(row.pricing)
    const inputPerMillion = tokenPrice(pricing?.prompt)
    const outputPerMillion = tokenPrice(pricing?.completion)
    if (inputPerMillion === null || outputPerMillion === null) continue

    const author = row.id.split('/')[0]
    models.push({
      id: row.id,
      provider: PROVIDER_LABELS[author] || author,
      model: typeof row.name === 'string' && row.name.trim() ? row.name : row.id,
      inputPerMillion,
      outputPerMillion,
      contextLength: typeof row.context_length === 'number' && Number.isFinite(row.context_length) && row.context_length > 0
        ? row.context_length : null,
      created: typeof row.created === 'number' && Number.isFinite(row.created) && row.created > 0
        ? row.created : null
    })
    seen.add(row.id)
  }
  if (!models.length) throw new Error('OpenRouter returned no priced text models')
  return models
}

export function hostedApiEquivalentCosts(
  promptTokens: unknown,
  completionTokens: unknown,
  prices: readonly HostedApiPrice[]
): HostedApiEstimate[] {
  const promptMillions = tokenCount(promptTokens) / 1_000_000
  const completionMillions = tokenCount(completionTokens) / 1_000_000
  return prices.map(price => ({
    ...price,
    costUsd: promptMillions * price.inputPerMillion + completionMillions * price.outputPerMillion
  }))
}

let cachedCatalog: HostedApiCatalog | null = null
let pendingCatalog: Promise<HostedApiCatalog> | null = null

/** Fetch only when the Analytics overview needs prices; never block the main stats request. */
export async function loadHostedApiPrices(forceRefresh = false): Promise<HostedApiCatalog> {
  if (!forceRefresh && cachedCatalog && Date.now() - cachedCatalog.fetchedAt < CATALOG_TTL_MS) {
    return { ...cachedCatalog, stale: false }
  }
  if (pendingCatalog) return pendingCatalog

  const request = (async (): Promise<HostedApiCatalog> => {
    try {
      const response = await fetch(OPENROUTER_MODELS_URL, {
        headers: { Accept: 'application/json' },
        signal: AbortSignal.timeout(10_000)
      })
      if (!response.ok) throw new Error(`OpenRouter model catalog returned HTTP ${response.status}`)
      const models = parseOpenRouterModels(await response.json())
      cachedCatalog = { models, fetchedAt: Date.now(), stale: false }
      return cachedCatalog
    } catch (error) {
      if (cachedCatalog) return { ...cachedCatalog, stale: true }
      throw error
    }
  })()
  pendingCatalog = request
  try {
    return await request
  } finally {
    if (pendingCatalog === request) pendingCatalog = null
  }
}
