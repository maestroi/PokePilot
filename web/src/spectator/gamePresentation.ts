import type { SpectatorRun } from '../shared/api/spectator.ts'

function titleCase(value: string): string {
  return value
    .replaceAll('_', ' ')
    .replaceAll('-', ' ')
    .replace(/\b\w/g, (letter) => letter.toUpperCase())
}

export function isTetrisRun(run: SpectatorRun | null | undefined): boolean {
  return Boolean(run && (run.game === 'tetris' || run.game_state?.kind === 'tetris'))
}

export function gameTitle(run: SpectatorRun): string {
  if (isTetrisRun(run)) return 'Tetris'
  switch ((run.game || '').toLowerCase()) {
    case 'pokemon-blue': return 'Pokémon Blue'
    case 'pokemon-yellow': return 'Pokémon Yellow'
    default: return 'Pokémon Red'
  }
}

export function tetrisRouteLabel(run: SpectatorRun): string {
  const state = run.game_state
  if (run.goal) return run.goal
  const mode = state?.mode ? titleCase(state.mode) : 'Autonomous play'
  return `${mode} · ${Number(state?.lines_cleared || 0)} lines`
}

export function tetrisLocationLabel(run: SpectatorRun): string {
  const state = run.game_state
  const level = Number(state?.level || 0)
  const lines = Number(state?.lines_cleared || 0)
  if (state?.game_over) return `Game over · ${lines} lines`
  if (state?.complete) return `Goal complete · ${lines} lines`
  if (state?.paused) return `Paused · level ${level}`
  return `Level ${level} · ${lines} lines`
}

export function tetrisRunTitle(run: SpectatorRun): string {
  const score = Number(run.game_state?.score || 0).toLocaleString()
  const lines = Number(run.game_state?.lines_cleared || 0)
  return `Tetris · ${score} pts · ${lines} lines`
}
