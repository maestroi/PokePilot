export type RunStatus = 'queued' | 'leased' | 'running' | 'done' | string
export type RecoveryProfile = 'strict' | 'resilient'

export interface SolverAttempt {
  id: string
  backend: string
  model?: string
  state: string
  run_id?: string
  branch?: string
  pr_number?: number
  pr_url?: string
  exit_code?: number
  note?: string
  started_at?: number
  updated_at?: number
}

export interface DashboardIssueLink {
  issue_number?: number
  issue_url?: string
  status?: string
  resolution?: string
  occurrence_count?: number
  fixed_revision?: string
  solver_attempts?: SolverAttempt[]
  verification_state?: string
  verification_revision?: string
  verification_verified_at?: number
  stale?: boolean
  circuit_open?: boolean
  circuit_kind?: string
  circuit_count?: number
  circuit_run_id?: string
  circuit_opened_at?: number
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
  decision_records?: TypedDecisionRecord[]
  decision_records_dropped?: number
  decision_summary?: DecisionSummary
  [key: string]: unknown
}

// One typed-decision call in a run's live feed (only the most recent calls
// travel; see DecisionSummary for the whole run).
export interface TypedDecisionRecord {
  kind?: string
  choice?: string
  choice_label?: string
  probabilities?: Record<string, number>
  confidence?: number
  duration_seconds?: number
  backend?: string
  model?: string
  fallback?: boolean
  shadow?: boolean
  executed?: string
  agreed?: boolean
  error?: string
}

// Fixed-size aggregate of every typed decision a run made, per kind.
export interface DecisionKindSummary {
  calls: number
  fallbacks?: number
  errors?: number
  shadow?: number
  agreements?: number
  disagreements?: number
  confidence: number[]
  confidence_judged: number[]
  confidence_agreed: number[]
  latency: number[]
  latency_seconds?: number
  p50_seconds?: number
  p95_seconds?: number
  prompt_tokens?: number
  completion_tokens?: number
  engine_choices?: Record<string, number>
  executed_choices?: Record<string, number>
}

export interface DecisionSummary {
  kinds?: Record<string, DecisionKindSummary>
}

// Fast typed-decision backend a run selected. Endpoints and credentials stay
// on the runner; only the choice travels with the run.
export interface DecisionEngineSpec {
  backend: 'off' | 'jev' | 'system-one'
  // Registered deployment id. The wall resolves it and copies the
  // secret-free identity into inference; clients never send inference.
  deployment?: string
  inference?: InferenceIdentity
  // Shadow records the backend's answers without acting on them; omitted
  // means active, which is how selections made before modes behaved.
  mode?: 'off' | 'shadow' | 'active'
  // Battle turns are observed only, so battles requires shadow mode.
  battles?: boolean
  objectives?: boolean
  failures?: boolean
  min_confidence?: number
}

export interface DashboardRun {
  run_id: string
  status: RunStatus
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
  purpose?: string
  risk_tolerance?: string
  wild_encounters?: string
  reasoning_effort?: string
  decision_engine?: DecisionEngineSpec
  seed?: number
  fps?: number
  max_rounds?: number
  max_frames?: number
  recovery_profile?: RecoveryProfile
  recovery_attempts?: number
  recovery_badges?: number
  recovery_events?: number
  recovery_maps?: number
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
  circuit_key?: string
  circuit_fingerprint?: string
  circuit_kind?: string
  circuit_count?: number
  circuit_badges?: number
  circuit_events?: number
  circuit_maps?: number
  circuit_revision?: string
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
  protocol?: DeploymentProtocol
  control_url?: string
  token_env?: string
  engine?: string
  engine_version?: string
  engine_config?: string
}

// Wire API a registered deployment speaks. Empty/openai can serve the
// strategist or a typed decision engine; typesafe-choice only the latter.
export type DeploymentProtocol = '' | 'openai' | 'typesafe-choice' 

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
  protocol?: DeploymentProtocol
  enabled: boolean
  discover?: boolean
  default_for?: string[]
  control_url?: string
  token_env?: string
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

export type ModelDeploymentInput = Omit<ModelDeployment, 'state' | 'loaded_deployment' | 'active_leases' | 'queued' | 'error'>

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
  arm_a: ExperimentArm
  arm_b: ExperimentArm
  game?: string
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
  recovery_profile?: RecoveryProfile
}

export interface ExperimentArmSummary {
  runs?: number
  done?: number
  goal_successes?: number
  boulder_successes?: number
  success_rate?: number
  badges?: number
  rounds?: number
  frames?: number
  median_rounds_to_goal?: number
  median_frames_to_goal?: number
  avg_run_seconds?: number
  calls?: number
  strategic_calls?: number
  strategic_rejected?: number
  strategic_seconds?: number
  plan_steps_produced?: number
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
  plan_execution_fraction?: number
  blackouts?: number
  objective_failures?: number
  stagnation_replans?: number
  plan_exhaustion_replans?: number
  replan_reasons?: Record<string, number>
  final_stop_reasons?: Record<string, number>
  strategic_records_dropped?: number
}

export interface ExperimentPairedSummary {
  a_wins?: number
  ties?: number
  b_wins?: number
  comparable_pairs?: number
  completed_pairs?: number
  excluded_pairs?: number
}

export interface ExperimentPairResult {
  seed: number
  comparable: boolean
  status_a?: string
  status_b?: string
  success_a?: boolean
  success_b?: boolean
  winner?: 'a' | 'b' | 'tie' | string
  non_comparable_reason?: string
}

export interface ExperimentIdentity {
  game?: string
  git_revision?: string
  rom_identity?: string
  prompt_identity?: string
  arm_a?: InferenceIdentity
  arm_b?: InferenceIdentity
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
  identity?: ExperimentIdentity
  pairs?: ExperimentPairResult[]
}

export interface ExperimentList {
  experiments: ExperimentView[]
}

export interface RunSpec {
  run_id: string
  seed: number
  game: string
  planner: string
  starter: string
  dest: string
  goal: string
  llm_profile: string
  llm_deployment?: string
  play_style: string
  purpose?: string
  risk_tolerance: string
  wild_encounters: string
  reasoning_effort: string
  decision_engine?: DecisionEngineSpec
  fps: number
  max_rounds: number
  max_frames: number
  recovery_profile: RecoveryProfile
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
  dismissable?: boolean
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
