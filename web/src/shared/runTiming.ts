const GAME_BOY_CPU_HZ = 4_194_304
const GAME_BOY_CYCLES_PER_FRAME = 70_224

// Pokémon Red advances on the original DMG video cadence. Keep the 1×
// conversion tied to hardware timing rather than a runner's configured FPS,
// which is a throttle/target and can differ between workers.
export const GAME_BOY_FPS = GAME_BOY_CPU_HZ / GAME_BOY_CYCLES_PER_FRAME

export interface RunTimingLike {
  status?: string
  frame?: number
  started_at?: number
  queued_at?: number
  ended_at?: number
  stats?: {
    calls?: number
    avg_seconds?: number
  } | null
}

function finiteNonNegative(value: unknown): number {
  const number = Number(value || 0)
  return Number.isFinite(number) && number > 0 ? number : 0
}

export function gameTimeSeconds(run: RunTimingLike): number {
  return finiteNonNegative(run.frame) / GAME_BOY_FPS
}

export function elapsedRunSeconds(run: RunTimingLike, nowSeconds = Date.now() / 1000): number {
  // started_at is intentionally preferred for forward compatibility. The
  // current wall exposes queued_at, so today this is enqueue-to-now/end time.
  const startedAt = finiteNonNegative(run.started_at) || finiteNonNegative(run.queued_at)
  if (!startedAt) return 0

  const endedAt = finiteNonNegative(run.ended_at)
  const end = run.status === 'done' && endedAt ? endedAt : finiteNonNegative(nowSeconds)
  return end > startedAt ? end - startedAt : 0
}

export function thinkingTimeSeconds(run: RunTimingLike): number {
  const calls = finiteNonNegative(run.stats?.calls)
  const average = finiteNonNegative(run.stats?.avg_seconds)
  return calls * average
}

export function averageRunSpeed(run: RunTimingLike, nowSeconds = Date.now() / 1000): number | null {
  const elapsed = elapsedRunSeconds(run, nowSeconds)
  const gameTime = gameTimeSeconds(run)
  if (elapsed <= 0 || gameTime <= 0) return null
  return gameTime / elapsed
}

export function formatDuration(seconds: number): string {
  const total = Math.max(0, Math.round(Number(seconds) || 0))
  if (total < 60) return `${total}s`

  const minutes = Math.floor(total / 60)
  const remainingSeconds = total % 60
  if (minutes < 60) return `${minutes}m${remainingSeconds ? ` ${remainingSeconds}s` : ''}`

  const hours = Math.floor(minutes / 60)
  const remainingMinutes = minutes % 60
  if (hours < 24) return `${hours}h${remainingMinutes ? ` ${remainingMinutes}m` : ''}`

  const days = Math.floor(hours / 24)
  const remainingHours = hours % 24
  return `${days}d${remainingHours ? ` ${remainingHours}h` : ''}`
}

export function formatRunSpeed(speed: number | null): string {
  if (speed == null || !Number.isFinite(speed) || speed <= 0) return '—'
  const digits = speed < 1 ? 2 : 1
  return `${speed.toFixed(digits)}×`
}

export function runPaceSummary(run: RunTimingLike, nowSeconds = Date.now() / 1000): string {
  const gameTime = gameTimeSeconds(run)
  if (gameTime <= 0) return ''

  const parts = [`${formatDuration(gameTime)} @ 1×`]
  const elapsed = elapsedRunSeconds(run, nowSeconds)
  if (elapsed > 0) parts.push(`${formatDuration(elapsed)} elapsed`)

  const speed = averageRunSpeed(run, nowSeconds)
  if (speed != null) parts.push(`${formatRunSpeed(speed)} avg`)
  return parts.join(' · ')
}

export function runTimingSummary(run: RunTimingLike, nowSeconds = Date.now() / 1000): string {
  const pace = runPaceSummary(run, nowSeconds)
  if (!pace) return ''

  const thinking = thinkingTimeSeconds(run)
  return thinking > 0 ? `${pace} · ${formatDuration(thinking)} thinking` : pace
}
