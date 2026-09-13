export type HostedApiPrice = {
  provider: string
  model: string
  inputPerMillion: number
  outputPerMillion: number
}

export type HostedApiEstimate = HostedApiPrice & {
  costUsd: number
}

export const HOSTED_API_PRICING_UPDATED = 'Sep 2026'

export const HOSTED_API_PRICES: readonly HostedApiPrice[] = [
  { provider: 'Anthropic', model: 'Claude Sonnet 5', inputPerMillion: 2, outputPerMillion: 10 },
  { provider: 'OpenAI', model: 'GPT-5.6 Sol', inputPerMillion: 4, outputPerMillion: 20 },
  { provider: 'Anthropic', model: 'Claude Opus 5', inputPerMillion: 5, outputPerMillion: 25 }
]

function tokenCount(value: unknown): number {
  const parsed = Number(value || 0)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0
}

export function hostedApiEquivalentCosts(promptTokens: unknown, completionTokens: unknown): HostedApiEstimate[] {
  const promptMillions = tokenCount(promptTokens) / 1_000_000
  const completionMillions = tokenCount(completionTokens) / 1_000_000

  return HOSTED_API_PRICES.map((price) => ({
    ...price,
    costUsd: promptMillions * price.inputPerMillion + completionMillions * price.outputPerMillion
  }))
}
