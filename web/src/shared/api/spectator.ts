export interface SpectatorPartyMon {
  name: string
  level: number
  hp: number
  max_hp: number
  status?: string
}

export interface SpectatorPlayer {
  money: number
  badges?: string[]
  party: SpectatorPartyMon[]
}

export interface SpectatorStats {
  round: number
  rounds_left: number
  calls: number
  rounds: number
  rejected: number
  repeats: number
  last_seconds: number
  avg_seconds: number
  goal_summary?: string
  goal_current?: number
  goal_target?: number
  goal_complete?: boolean
}

export interface SpectatorRun {
  run_id: string
  status: string
  starter?: string
  dest?: string
  goal?: string
  fps?: number
  play_style?: string
  risk_tolerance?: string
  wild_encounters?: string
  queued_at?: number
  ended_at?: number
  frame?: number
  map?: number
  x?: number
  y?: number
  decision?: string
  stop_so_far?: string
  stats?: SpectatorStats
  player?: SpectatorPlayer
  attempts?: number
  reason?: string
  replay_ready?: boolean
  highlight?: string
}

export interface SpectatorSummary {
  live: number
  queued: number
  completed: number
}

export interface SpectatorSnapshot {
  now: number
  runs: SpectatorRun[]
  summary: SpectatorSummary
}
