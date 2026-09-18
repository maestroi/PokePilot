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

export interface BagItem {
  name: string
  quantity: number
}

export interface PlayerSnapshot {
  money: number
  badges?: string[]
  party: PartyMon[]
  bag_used?: number
  bag_capacity?: number
  bag?: BagItem[]
  dex_owned?: number
  dex_seen?: number
  dex_total?: number
  milestones?: string[]
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
  game?: string
  planner?: string
  starter?: string
  dest?: string
  goal?: string
  llm_profile?: string
  llm_deployment?: string
  inference?: InferenceIdentity
  experiment_id?: string
  experiment_arm?: string
  experiment_case?: string
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
  resume_protected?: boolean
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

export interface InferenceIdentity {
  deployment_id: string
  label?: string
  model_id: string
  revision?: string
  artifact?: string
  quantization?: string
  compute: string
  endpoint: string
  api_model: string
  control_url?: string
  token_env?: string
  engine?: string
  engine_version?: string
  engine_config?: string
}

export type DeploymentState = 'ready' | 'available' | 'loading' | 'busy' | 'failed' | 'unavailable' | string

export interface ModelDeployment {
  id: string
  label: string
  model_id: string
  revision?: string
  artifact?: string
  quantization?: string
  compute: string
  endpoint: string
  api_model: string
  enabled: boolean
  control_url?: string
  engine?: string
  engine_version?: string
  engine_config?: string
  max_parallel_workers?: number
  legacy_profile?: string
  state: DeploymentState
  loaded_deployment?: string
  active_leases?: number
  queued?: number
  error?: string
}

export interface ModelRegistrySnapshot {
  deployments: ModelDeployment[]
  hosts?: Record<string, unknown>
}

export interface ExperimentArm {
  name: string
  deployment: string
  max_parallel_workers?: number
}

export interface ExperimentRequest {
  name: string
  game?: string
  arm_a: ExperimentArm
  arm_b: ExperimentArm
  goal: string
  starter?: string
  seeds?: number[]
  seed_count?: number
  play_style?: string
  risk_tolerance?: string
  wild_encounters?: string
  reasoning_effort?: string
  fps?: number
  max_rounds?: number
  max_frames?: number
}

export interface ExperimentArmSummary {
  runs?: number
  done?: number
  boulder_successes?: number
  success_rate?: number
  rounds?: number
  frames?: number
  strategic_calls?: number
  avg_strategic_call_seconds?: number
  p50_strategic_call_seconds?: number
  p95_strategic_call_seconds?: number
  avg_prefill_tps?: number
  avg_decode_tps?: number
  prompt_tokens?: number
  completion_tokens?: number
  rejected?: number
  transport_errors?: number
  fallbacks?: number
  plan_executions?: number
  steps_skipped?: number
}

export interface ExperimentPairedSummary {
  a_wins?: number
  ties?: number
  b_wins?: number
}

export interface ExperimentView {
  id: string
  name: string
  created_at?: string
  total_pairs?: number
  request?: ExperimentRequest
  run_ids?: string[]
  arm_a?: ExperimentArmSummary
  arm_b?: ExperimentArmSummary
  paired?: ExperimentPairedSummary
}

export interface ExperimentList {
  experiments: ExperimentView[]
}

export interface RunSpec {
  run_id: string
  seed: number
  game?: string
  planner: string
  starter: string
  dest: string
  goal: string
  llm_profile: string
  llm_deployment?: string
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
  run_ids?: string[]
  examples?: string[]
  example?: string
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
