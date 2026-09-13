export type RunStatus = 'queued' | 'leased' | 'running' | 'done' | string

export interface DashboardIssueLink {
  issue_number?: number
  issue_url?: string
  status?: string
  resolution?: string
  occurrence_count?: number
  fixed_revision?: string
  stale?: boolean
}

export interface PartyMon {
  name: string
  level: number
  hp: number
  max_hp: number
  status?: string
}

export interface PlayerSnapshot {
  money: number
  badges?: string[]
  party: PartyMon[]
}

export interface MapSprite {
  x: number
  y: number
  picture_id?: number
  slot?: number
}

export interface DashboardStats {
  round?: number
  rounds?: number
  rounds_left?: number
  calls?: number
  rejected?: number
  repeats?: number
  avg_seconds?: number
  last_seconds?: number
  prompt_tokens?: number
  completion_tokens?: number
  goal_summary?: string
  goal_current?: number
  goal_target?: number
  goal_complete?: boolean
  model?: string
  backend?: string
  [key: string]: unknown
}

export interface DashboardRun {
  run_id: string
  status: RunStatus
  planner?: string
  starter?: string
  dest?: string
  goal?: string
  llm_profile?: string
  play_style?: string
  risk_tolerance?: string
  wild_encounters?: string
  reasoning_effort?: string
  seed?: number
  fps?: number
  max_rounds?: number
  max_frames?: number
  endless?: boolean
  random_seed?: boolean
  queued_at?: number
  ended_at?: number
  frame?: number
  map?: number
  x?: number
  y?: number
  trace?: string
  question?: string
  decision?: string
  raw?: string
  stop_so_far?: string
  sprites?: MapSprite[]
  trail?: [number, number][]
  stats?: DashboardStats
  player?: PlayerSnapshot
  attempts?: number
  error_attempts?: number
  loss_recoveries?: number
  reason?: string
  detail?: string
  issue?: DashboardIssueLink
  replay_available?: boolean
  resume_from_run_id?: string
  [key: string]: unknown
}

export interface DashboardWorker {
  addr: string
  version?: string
  run_id?: string
  seen_ago?: string
}

export interface DashboardFacets {
  outcomes: string[]
  hows: string[]
  starters: string[]
}

export interface DashboardSnapshot {
  now: number
  wall_version?: string
  runs: DashboardRun[]
  workers: DashboardWorker[]
  total: number
  history_facets?: DashboardFacets
}

export interface DashboardQuery {
  active?: boolean
  status?: string
  limit?: number
  offset?: number
  facets?: boolean
  outcome?: string
  how?: string
  starter?: string
}

export interface RunSpec {
  run_id: string
  seed: number
  planner: string
  starter: string
  dest: string
  goal: string
  llm_profile: string
  play_style: string
  risk_tolerance: string
  wild_encounters: string
  reasoning_effort: string
  fps: number
  max_rounds: number
  max_frames: number
  endless: boolean
  random_seed: boolean
}

export interface TriageGroup {
  pattern?: string
  key: string
  fingerprint?: string
  count: number
  runs?: string[]
  examples?: string[]
  detail?: string
  latest_run_id?: string
  issue?: DashboardIssueLink
  [key: string]: unknown
}

export interface ReplayStatus {
  run_id?: string
  state: string
  size?: number
  error?: string
  [key: string]: unknown
}

export interface RunArtifact {
  name: string
  media_type?: string
  size?: number
  location?: string
  storage?: string
  sha256?: string
  [key: string]: unknown
}
