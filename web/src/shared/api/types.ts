export type RunStatus = 'queued' | 'leased' | 'running' | 'done' | string

export interface DashboardIssueLink {
  issue_number?: number
  issue_url?: string
  status?: string
  occurrence_count?: number
  fixed_revision?: string
  stale?: boolean
}

export interface DashboardStats {
  round?: number
  rounds?: number
  rounds_left?: number
  repeats?: number
  avg_seconds?: number
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
  stop_so_far?: string
  stats?: DashboardStats
  attempts?: number
  error_attempts?: number
  loss_recoveries?: number
  reason?: string
  detail?: string
  issue?: DashboardIssueLink
  replay_available?: boolean
  resume_from_run_id?: string
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
