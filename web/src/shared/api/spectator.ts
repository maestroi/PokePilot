export interface SpectatorPartyMon {
  name: string
  level: number
  hp: number
  max_hp: number
  status?: string
}

export interface SpectatorBagItem {
  name: string
  quantity: number
}

export interface SpectatorPlayer {
  money: number
  badges?: string[]
  party: SpectatorPartyMon[]
  bag_used?: number
  bag_capacity?: number
  bag?: SpectatorBagItem[]
  dex_owned?: number
  dex_seen?: number
  dex_total?: number
  milestones?: string[]
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

export interface SpectatorMapSprite {
  x: number
  y: number
}

export interface SpectatorRun {
  run_id: string
  status: string
  starter?: string
  dest?: string
  goal?: string
  fps?: number
  llm_profile?: string
  play_style?: string
  purpose?: string
  risk_tolerance?: string
  wild_encounters?: string
  queued_at?: number
  ended_at?: number
  frame?: number
  map?: number
  x?: number
  y?: number
  maps_visited?: number
  planner_waiting?: boolean
  planner_options?: number
  decision?: string
  stop_so_far?: string
  stats?: SpectatorStats
  player?: SpectatorPlayer
  sprites?: SpectatorMapSprite[]
  trail?: [number, number][]
  attempts?: number
  reason?: string
  replay_ready?: boolean
  highlight?: string
  featured?: boolean
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
  featured_run_id?: string
}
