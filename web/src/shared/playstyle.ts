export type PlayStyle = 'speedrun' | 'adventure' | 'completionist' | 'team_builder'

export interface PlayStyleSource {
  planner?: string
  play_style?: string
  fps?: number
  risk_tolerance?: string
  wild_encounters?: string
}

export function normalizePlayStyle(run: PlayStyleSource | null | undefined): PlayStyle {
  switch ((run?.play_style || '').trim().toLowerCase()) {
    case 'adventure':
      return 'adventure'
    case 'completionist':
      return 'completionist'
    case 'team_builder':
    case 'team-builder':
    case 'teambuilder':
      return 'team_builder'
    default:
      return 'speedrun'
  }
}

export function defaultGoalForPlayStyle(run: PlayStyleSource | PlayStyle | null | undefined): string {
  const source = typeof run === 'string' ? { play_style: run } : run
  return normalizePlayStyle(source) === 'completionist'
    ? 'Complete the obtainable Pokédex.'
    : 'Beat the Elite Four and Champion.'
}

export function playStyleLabel(run: PlayStyleSource | null | undefined): string {
  switch (normalizePlayStyle(run)) {
    case 'adventure':
      return 'Adventure'
    case 'completionist':
      return 'Completionist'
    case 'team_builder':
      return 'Team Builder'
    default:
      return 'Speedrun'
  }
}

export function playStyleTagline(run: PlayStyleSource | null | undefined): string {
  switch (normalizePlayStyle(run)) {
    case 'adventure':
      return 'Natural play · exploration and story'
    case 'completionist':
      return 'Optional content · items and interactions'
    case 'team_builder':
      return 'Party growth · catches and training'
    default:
      return 'Progression first · minimal detours'
  }
}

export function playSpeedLabel(run: PlayStyleSource | null | undefined): string {
  const fps = Number(run?.fps ?? 0)
  if (fps <= 0) return 'MAX'
  const multiple = fps / 60
  if (Number.isInteger(multiple)) return `${multiple}×`
  return `${multiple.toFixed(1).replace(/\.0$/, '')}×`
}

export function policyLabel(value: string | undefined, fallback = ''): string {
  const text = (value || fallback).trim()
  if (!text) return ''
  return text
    .replaceAll('_', ' ')
    .replaceAll('-', ' ')
    .replace(/\b\w/g, (letter) => letter.toUpperCase())
}

export function isPlayStyleRun(run: PlayStyleSource | null | undefined): boolean {
  return Boolean(run) && run?.planner !== 'scripted'
}
