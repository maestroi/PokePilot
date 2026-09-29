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

export interface SpectatorDecisionRecord {
  kind?: string
  choice?: string
  choice_label?: string
  confidence?: number
  duration_seconds?: number
  backend?: string
  model?: string
  fallback?: boolean
}

export interface SpectatorDecisionEngine {
  backend?: string
  mode?: string
  deployment?: string
  label?: string
  model?: string
  min_confidence?: number
  max_choices?: number
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
  decision_calls?: number
  decision_rejected?: number
  decision_fallbacks?: number
  decision_avg_seconds?: number
  decision_backend?: string
  decision_model?: string
  decision_mode?: string
  decision_choice?: string
  decision_confidence?: number
  decision_reference?: string
  decision_reference_agreed?: boolean
  decision_reference_agreements?: number
  decision_reference_disagreements?: number
  decision_records?: SpectatorDecisionRecord[]
}

export interface SpectatorMapSprite {
  x: number
  y: number
}

export interface SpectatorTetrisPiece {
  piece?: string
  rotation?: number
  x?: number
  y?: number
}

export interface SpectatorTetrisState {
  kind?: 'tetris' | string
  mode?: string
  screen?: string
  board?: string[]
  level?: number
  score?: number
  score_valid?: boolean
  lines_cleared?: number
  lines_remaining?: number
  line_goal?: number
  paused?: boolean
  locking?: boolean
  clearing?: boolean
  game_over?: boolean
  complete?: boolean
  ready_for_piece_input?: boolean
  active?: SpectatorTetrisPiece
  next?: SpectatorTetrisPiece
}

export interface SpectatorRun {
  run_id: string
  status: string
  game?: string
  game_state?: SpectatorTetrisState
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
  decision_engine?: SpectatorDecisionEngine
  stats?: SpectatorStats
  player?: SpectatorPlayer
  sprites?: SpectatorMapSprite[]
  trail?: [number, number][]
  attempts?: number
  reason?: string
  replay_ready?: boolean
  replay_state?: 'generating' | 'ready' | string
  replay_stage?: 'preparing' | 'rendering' | 'assembling' | 'uploading' | string
  replay_segments?: number
  replay_segments_done?: number
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


export interface SpectatorProgrammingEntry {
  id: string
  challenge_id: string
  challenge_version: number
  challenge_name: string
  state: string
  scheduled_at?: number
  started_at?: number
  run_ids?: string[]
}

export interface SpectatorProgrammingSnapshot {
  paused: boolean
  live_now?: SpectatorProgrammingEntry
  up_next?: SpectatorProgrammingEntry
  future: SpectatorProgrammingEntry[]
}
