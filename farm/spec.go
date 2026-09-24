// Package farm holds the wire contract between a pokepilot runner and the
// pokewall orchestrator: lease/heartbeat/finish JSON, and (in client.go)
// the small HTTP client the runner uses to speak it. Nothing here reaches
// into emu, skill, agent, or red — those packages import nothing from
// farm, and farm imports nothing from them.
package farm

// RecoveryProfile controls what the wall does when a goal-driven runner stops
// before satisfying its goal. Empty is intentionally equivalent to strict so
// older queued specs preserve their historic bounded-retry semantics.
type RecoveryProfile string

const (
	RecoveryProfileStrict    RecoveryProfile = "strict"
	RecoveryProfileResilient RecoveryProfile = "resilient"
)

func (p RecoveryProfile) Valid() bool {
	return p == "" || p == RecoveryProfileStrict || p == RecoveryProfileResilient
}

func (p RecoveryProfile) Resilient() bool {
	return p == RecoveryProfileResilient
}

// RunPurpose describes why the run exists independently from how it plays and
// what terminal goal it pursues. Empty is the backwards-compatible normal run.
type RunPurpose string

const (
	RunPurposeNormal        RunPurpose = "normal"
	RunPurposeDebugCoverage RunPurpose = "debug_coverage"
)

func (p RunPurpose) Valid() bool {
	return p == "" || p == RunPurposeNormal || p == RunPurposeDebugCoverage
}

// Spec is one run's configuration, filled either by CLI flags (today) or
// by a lease from the wall (farm mode). Field names mirror the flags in
// cmd/pokepilot/main.go one for one.
type Spec struct {
	RunID string `json:"run_id"`
	// Attempt is which attempt of this run the wall is handing out: 1 for
	// the first, higher after retries. The runner echoes it back in its
	// FinishReport so a late finish from a dead attempt cannot settle a
	// newer one.
	Attempt int   `json:"attempt"`
	Seed    int64 `json:"seed"`
	// Game is the game id the run should play, e.g. "pokemon-red" or
	// "pokemon-blue". Empty means no preference: the runner plays whichever
	// cartridge it has mounted, which keeps older deployments working.
	Game    string `json:"game,omitempty"`
	Planner string `json:"planner"`
	Starter string `json:"starter"`
	Dest    string `json:"dest"`
	// Goal is the task statement for the llm planner: what to achieve,
	// never how. It is omitted entirely when no goal was provided, so a
	// serialized Spec still distinguishes Free play (provided, empty) from
	// an unset goal. See RunGoal.
	Goal RunGoal `json:"goal,omitzero"`
	// PlayStyle, Purpose, RiskTolerance, and WildEncounters are orthogonal
	// gameplay policy knobs. They live on the Spec so one run's behavior is
	// fully described by its own wire payload, and so two runs can coexist
	// in one process without cross-talk. Empty intentionally means "use the
	// historical compatibility default", not a specific profile.
	PlayStyle      string     `json:"play_style,omitempty"`
	Purpose        RunPurpose `json:"purpose,omitempty"`
	RiskTolerance  string     `json:"risk_tolerance,omitempty"`
	WildEncounters string     `json:"wild_encounters,omitempty"`
	LLMProfile     string     `json:"llm_profile,omitempty"`
	// LLMDeployment is the first-class deployment selection. LLMProfile is
	// retained only as a compatibility adapter for older queued runs/runners.
	LLMDeployment string             `json:"llm_deployment,omitempty"`
	Inference     *InferenceIdentity `json:"inference,omitempty"`
	// Paired experiment identity is optional for ordinary runs. Case is shared
	// by the A/B pair for one seed.
	ExperimentID   string `json:"experiment_id,omitempty"`
	ExperimentArm  string `json:"experiment_arm,omitempty"`
	ExperimentCase string `json:"experiment_case,omitempty"`
	// ReasoningEffort overrides the strategist's reasoning_effort field for
	// this run: "low", "medium", or "high". Empty means the endpoint's
	// configured default (POKEPILOT_LLM_REASONING_EFFORT, or "medium").
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	// DecisionEngine optionally selects the fast typed-decision backend for
	// this run, independently of the strategist deployment. Nil keeps the
	// runner's environment default.
	DecisionEngine *DecisionEngineSpec `json:"decision_engine,omitempty"`
	FPS            int                 `json:"fps"`
	// MaxRounds is an OPTIONAL emergency/experiment cap for an LLM run.
	// Zero is the normal goal-driven mode: there is no hard round limit and
	// the run ends on goal completion, a real failure, cancellation, or the
	// agent's automatic stagnation watchdog. A positive value is preserved
	// for fixed-budget experiments and still ends with reason "budget".
	MaxRounds int `json:"max_rounds"`
	// MaxFrames remains the last-resort emulator watchdog. Zero on the wire
	// asks the runner to use its built-in frame safety limit.
	MaxFrames int `json:"max_frames"`
	// RecoveryProfile is orthogonal to the agent's bounded local retry budgets.
	// Strict preserves the historic wall-level terminal budgets. Resilient keeps
	// the campaign alive across error/failed/stuck/budget stops, escalating
	// checkpoint rollback until progress resumes or the operator cancels.
	RecoveryProfile RecoveryProfile `json:"recovery_profile,omitempty"`
	// Endless asks the wall to queue a successor when this run settles,
	// so idle workers keep picking up work. A successor of a failed
	// campaign resumes from the parent's latest major checkpoint; a
	// successful `done` starts fresh. RandomSeed picks a fresh seed on
	// each successor; otherwise the seed is copied.
	Endless    bool `json:"endless,omitempty"`
	RandomSeed bool `json:"random_seed,omitempty"`
}

// RunPolicy is the subset of a Spec that selects planner behavior. Goal says
// what ends the run, PlayStyle says how it plays, and Purpose says why the run
// exists (normal gameplay versus deliberate debug coverage). Run wiring passes
// it by value so one run never reaches into process-global policy state.
type RunPolicy struct {
	Goal           string     `json:"goal,omitempty"`
	PlayStyle      string     `json:"play_style,omitempty"`
	Purpose        RunPurpose `json:"purpose,omitempty"`
	RiskTolerance  string     `json:"risk_tolerance,omitempty"`
	WildEncounters string     `json:"wild_encounters,omitempty"`
}

// RunPolicyFor extracts the behavior policy from a run's Spec.
func RunPolicyFor(spec Spec) RunPolicy {
	return RunPolicy{
		Goal:           spec.Goal.String(),
		PlayStyle:      spec.PlayStyle,
		Purpose:        spec.Purpose,
		RiskTolerance:  spec.RiskTolerance,
		WildEncounters: spec.WildEncounters,
	}
}

// MapSprite is one live map object on the runner's current map. These are
// ephemeral RAM observations: consumers may draw them as blockers, but must
// never retain them as learned geometry after a newer heartbeat arrives.
type MapSprite struct {
	X         uint8 `json:"x"`
	Y         uint8 `json:"y"`
	PictureID uint8 `json:"picture_id,omitempty"`
	Slot      uint8 `json:"slot,omitempty"`
}

// PartyMon is one party member on the operator wire: named, never a ROM
// index. Status is empty when healthy.
type PartyMon struct {
	Name   string `json:"name"`
	Level  uint8  `json:"level"`
	HP     uint16 `json:"hp"`
	MaxHP  uint16 `json:"max_hp"`
	Status string `json:"status,omitempty"`
}

// BagItem is one semantic bag entry. Names come from the game adapter;
// unknown item bytes stay displayable and never cross this boundary as IDs.
type BagItem struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}

// Player is a live trainer snapshot the runner decodes from RAM. Nil on
// older runners and before the first sample. An empty Party is a real
// pre-starter snapshot and must still be sent. Bag, Dex, and milestone
// fields are omitted by older runners; missing capacity/total means hide
// those meters rather than render 0/0.
type Player struct {
	Money       uint32     `json:"money"`
	Badges      []string   `json:"badges,omitempty"`
	Party       []PartyMon `json:"party"`
	BagUsed     int        `json:"bag_used,omitempty"`
	BagCapacity int        `json:"bag_capacity,omitempty"`
	Bag         []BagItem  `json:"bag,omitempty"`
	DexOwned    int        `json:"dex_owned,omitempty"`
	DexSeen     int        `json:"dex_seen,omitempty"`
	DexTotal    int        `json:"dex_total,omitempty"`
	Milestones  []string   `json:"milestones,omitempty"`
}

// ActivityEvent is the latest structured execution event from the runner.
// It complements Trace: Trace is emulator/debug evidence, while ActivityEvent
// names the subsystem and semantic action that actually happened.
type ActivityEvent struct {
	Source  string `json:"source"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	Detail  string `json:"detail,omitempty"`
	Frame   uint64 `json:"frame,omitempty"`
	Round   int    `json:"round,omitempty"`
}

// Heartbeat is the small, frequent status push a runner sends while a
// leased run is in progress.
type Heartbeat struct {
	RunID       string `json:"run_id"`
	Frame       uint64 `json:"frame"`
	Map         uint8  `json:"map"`
	X           uint8  `json:"x"`
	Y           uint8  `json:"y"`
	MapsVisited int    `json:"maps_visited,omitempty"`
	Trace       string `json:"trace"`
	// Sprites are the current live map objects (slots 1..15). Trail is a
	// bounded history of recent positions on this map, oldest first. Both
	// are live-only and optional for compatibility with older runners.
	Sprites []MapSprite `json:"sprites,omitempty"`
	Trail   [][2]uint8  `json:"trail,omitempty"`
	// Question is the offered menu the planner was last asked, numbered
	// the way the model saw it. Decision is the objective that ask
	// resolved to, or empty while the model is still answering. Both are
	// empty until the first plan of an llm run; a scripted run never
	// fills them.
	Question string `json:"question,omitempty"`
	Decision string `json:"decision,omitempty"`
	// Raw is the last model exchange verbatim: the prompt as it was sent,
	// and — once it arrives — the reply content as the server sent it.
	// Question is the menu rendered for a human; this is the bytes. It is
	// live-only (never persisted by the wall) and clipped, because it
	// rides every heartbeat.
	Raw       string `json:"raw,omitempty"`
	StopSoFar string `json:"stop_so_far"`
	// WorkerAddrs is where this runner's watch server (frame.png) is
	// reachable from the swarm network, one "host:port" per interface.
	// The wall uses these to proxy the live screen for its dashboard; it
	// is empty on runners that do not report it.
	WorkerAddrs []string `json:"worker_addrs,omitempty"`
	// Version is this runner's build identity (git SHA), so the wall can
	// show which build each worker runs. Empty from older runners.
	Version string `json:"version,omitempty"`
	// Stats is the llm planner's tally — the same numbers the runner's own
	// watch page renders, pushed here so the console shows them too. Nil on
	// scripted runs and on runners that predate it.
	Stats *LLMStats `json:"stats,omitempty"`
	// Player is the live party/money/badges snapshot. Nil on older
	// runners and before the first sample.
	Player *Player `json:"player,omitempty"`
	// Activity is the latest semantic execution event. It is intentionally a
	// single event rather than an unbounded log; the wall deduplicates and
	// retains a bounded operator history.
	Activity *ActivityEvent `json:"activity,omitempty"`
}

// LLMStats is the planner tally a runner pushes on its heartbeats: round
// progress, how often it re-picks an objective it already picked (Repeats —
// the wander signal), call latency, spend, and the replies that never
// resolved. The wall carries it verbatim for the console; the field names
// are the same JSON keys the runner's watch page renders, so both surfaces
// show one number.
type LLMStats struct {
	Round int `json:"round"`
	// RoundsLeft is positive only when the run has an explicit MaxRounds
	// cap. Zero means uncapped/goal-driven; it does NOT mean the run is out
	// of rounds.
	RoundsLeft int `json:"rounds_left"`
	// Calls counts every ask, Rounds only the ones that became an
	// objective: the gap between them is re-asks after a rejected reply.
	Calls    int `json:"calls"`
	Rounds   int `json:"rounds"`
	Rejected int `json:"rejected"`
	Repeats  int `json:"repeats"`

	AvgOffered           float64 `json:"avg_offered"`
	LastSeconds          float64 `json:"last_seconds"`
	AvgSeconds           float64 `json:"avg_seconds"`
	SuccessfulAvgSeconds float64 `json:"successful_avg_seconds,omitempty"`
	RejectedAvgSeconds   float64 `json:"rejected_avg_seconds,omitempty"`
	StrategicAvgSeconds  float64 `json:"strategic_avg_seconds,omitempty"`

	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	Transport        int `json:"transport"`
	Fallbacks        int `json:"fallbacks"`

	// Backend and Model name the endpoint that answered the most recent ask.
	// Backend is "primary" or "fallback"; Failovers counts primary transport
	// failures that pinned the run to its configured fallback.
	Backend   string `json:"backend,omitempty"`
	Model     string `json:"model,omitempty"`
	Failovers int    `json:"failovers,omitempty"`

	// Per-call serving telemetry is intentionally separate from the cumulative
	// PromptTokens/CompletionTokens above. Endpoint is the actual HTTP origin.
	// ResponseModel is what the server reported. The token counts describe only
	// the most recent ask. Prefill/decode fields come from llama.cpp's optional
	// top-level timings object; generic OpenAI-compatible servers can omit them
	// while still reporting per-call token usage.
	Endpoint               string  `json:"endpoint,omitempty"`
	ResponseModel          string  `json:"response_model,omitempty"`
	LastPromptTokens       int     `json:"last_prompt_tokens,omitempty"`
	LastCompletionTokens   int     `json:"last_completion_tokens,omitempty"`
	LastCachedPromptTokens int     `json:"last_cached_prompt_tokens,omitempty"`
	PrefillMS              float64 `json:"prefill_ms,omitempty"`
	PrefillTPS             float64 `json:"prefill_tps,omitempty"`
	DecodeMS               float64 `json:"decode_ms,omitempty"`
	DecodeTPS              float64 `json:"decode_tps,omitempty"`
	OverheadMS             float64 `json:"overhead_ms,omitempty"`
	TimingSource           string  `json:"timing_source,omitempty"`

	Intent    string `json:"intent"`
	IntentAge int    `json:"intent_age"`

	StrategicCalls          int                   `json:"strategic_calls,omitempty"`
	FastCalls               int                   `json:"fast_calls,omitempty"`
	PlanExecutions          int                   `json:"plan_executions,omitempty"`
	LegAutoExecutions       int                   `json:"leg_auto_executions,omitempty"`
	LegFastExecutions       int                   `json:"leg_fast_executions,omitempty"`
	LegBoundaries           int                   `json:"leg_boundaries,omitempty"`
	LegTailStepsDropped     int                   `json:"leg_tail_steps_dropped,omitempty"`
	StepsSkipped            int                   `json:"steps_skipped,omitempty"`
	PlanGoal                string                `json:"plan_goal,omitempty"`
	PlanSteps               []string              `json:"plan_steps,omitempty"`
	PlanStep                int                   `json:"plan_step,omitempty"`
	PlanRound               int                   `json:"plan_round,omitempty"`
	PlanBoundary            bool                  `json:"plan_boundary,omitempty"`
	LastLegDecision         string                `json:"last_leg_decision,omitempty"`
	LastReplanReason        string                `json:"last_replan_reason,omitempty"`
	ReplanReasons           map[string]int        `json:"replan_reasons,omitempty"`
	StrategicSeconds        float64               `json:"strategic_seconds,omitempty"`
	StrategicRecords        []StrategicCallRecord `json:"strategic_records,omitempty"`
	StrategicRecordsDropped int                   `json:"strategic_records_dropped,omitempty"`

	// Decision* is the independent constrained-backend telemetry. These fields
	// deliberately do not reuse Calls/Rejected/Model above: those describe the
	// generative planner and must remain comparable when typed decisions are
	// enabled only for one part of a run.
	DecisionCalls            int                   `json:"decision_calls,omitempty"`
	DecisionRejected         int                   `json:"decision_rejected,omitempty"`
	DecisionFallbacks        int                   `json:"decision_fallbacks,omitempty"`
	DecisionSeconds          float64               `json:"decision_seconds,omitempty"`
	DecisionAvgSeconds       float64               `json:"decision_avg_seconds,omitempty"`
	DecisionPromptTokens     int                   `json:"decision_prompt_tokens,omitempty"`
	DecisionCompletionTokens int                   `json:"decision_completion_tokens,omitempty"`
	DecisionInputBytes       int                   `json:"decision_input_bytes,omitempty"`
	DecisionOutputBytes      int                   `json:"decision_output_bytes,omitempty"`
	DecisionBackend          string                `json:"decision_backend,omitempty"`
	DecisionModel            string                `json:"decision_model,omitempty"`
	DecisionKind             string                `json:"decision_kind,omitempty"`
	DecisionChoice           string                `json:"decision_choice,omitempty"`
	DecisionConfidence       float64               `json:"decision_confidence,omitempty"`
	DecisionProbabilities    map[string]float64    `json:"decision_probabilities,omitempty"`
	DecisionRecords          []TypedDecisionRecord `json:"decision_records,omitempty"`
	DecisionRecordsDropped   int                   `json:"decision_records_dropped,omitempty"`

	// Goal* is present only when LLMPlanner.Goal opted into the structured
	// deterministic syntax. Summary is the human/model-facing status; the
	// numeric fields make dashboards able to render progress without parsing
	// prose. Complete becomes true on the final pre-model stop snapshot.
	GoalSummary  string `json:"goal_summary,omitempty"`
	GoalCurrent  int    `json:"goal_current,omitempty"`
	GoalTarget   int    `json:"goal_target,omitempty"`
	GoalComplete bool   `json:"goal_complete,omitempty"`

	Choices []ChoiceCount `json:"choices"`
}

// ChoiceCount is one objective and how many times the model has chosen it
// this run. The full sentence, not the kind: "go to pallet town" chosen six
// times is the finding, and "go-to chosen six times" hides it.
type ChoiceCount struct {
	Objective string `json:"objective"`
	Count     int    `json:"count"`
}

// HeartbeatReply is the wall's answer to a heartbeat. Cancel asks the
// runner to finish the current run at its next natural boundary and lease
// again; it is not a kill signal.
type HeartbeatReply struct {
	Cancel bool `json:"cancel"`
}

// WorkerPing advertises a runner's presence while it is between runs.
// Heartbeats carry WorkerAddrs for the in-flight half of a worker's life;
// this is the idle half, so the wall's grid can show which runners are
// available, not only which runs are in flight.
type WorkerPing struct {
	Addrs   []string `json:"addrs"`
	Version string   `json:"version,omitempty"`
}

// Coverage is the persisted, game-agnostic breadth summary for a run. It is
// intentionally nested under Progress so older runners/walls can ignore it
// while Completionist analytics can compare coverage directly.
type Coverage struct {
	UniqueMapsVisited     int `json:"unique_maps_visited,omitempty"`
	TrainersDefeated      int `json:"trainers_defeated,omitempty"`
	NPCInteractions       int `json:"npc_interactions,omitempty"`
	UniqueItemsAcquired   int `json:"unique_items_acquired,omitempty"`
	UniqueItemsUsed       int `json:"unique_items_used,omitempty"`
	DexOwned              int `json:"dex_owned,omitempty"`
	DexSeen               int `json:"dex_seen,omitempty"`
	OptionalMilestones    int `json:"optional_milestones,omitempty"`
	TMsHMsAcquired        int `json:"tms_hms_acquired,omitempty"`
	TMsHMsUsed            int `json:"tms_hms_used,omitempty"`
	Evolutions            int `json:"evolutions,omitempty"`
	Catches               int `json:"catches,omitempty"`
	UniqueSpeciesAcquired int `json:"unique_species_acquired,omitempty"`
}

// Progress is one snapshot of how far a run has gotten, at one point in
// it: badges held, story event flags set, distinct maps the player has
// stood on, and the map the player stands on. The runner decodes these
// from the same RAM it already reads (red/state) and carries them here;
// the wall stores them verbatim in the finish dump.
type Progress struct {
	// Round is the run round the sample was taken at: 0 before the first
	// objective ran, N after round N settled.
	Round int `json:"round"`
	// Badges is how many of the eight badges the party holds.
	Badges int `json:"badges"`
	// Events is how many story event flags are set.
	Events int `json:"events"`
	// Maps is how many distinct maps the player has stood on this run.
	Maps int `json:"maps"`
	// Map is where the player stands at the sample; MapName is its name
	// when the ROM names it.
	Map      uint8     `json:"map"`
	MapName  string    `json:"map_name,omitempty"`
	Coverage *Coverage `json:"coverage,omitempty"`
}

// FinishReport is why a run ended, sent once when it stops.
type FinishReport struct {
	RunID string `json:"run_id"`
	// Attempt echoes the spec's attempt number; 0 from older runners is
	// accepted without validation.
	Attempt   int      `json:"attempt,omitempty"`
	Reason    string   `json:"reason"`
	Detail    string   `json:"detail"`
	TraceTail []string `json:"trace_tail"`
	SaveState []byte   `json:"save_state"`
	FramePNG  []byte   `json:"frame_png,omitempty"`
	// RunnerVersion is the leased runner's build identity (git SHA). Empty
	// from older runners.
	RunnerVersion string `json:"runner_version,omitempty"`
	// SeedBurn is the idle frames this run burned after boot. Zero is a
	// real value (bit-identical replay) and is always encoded.
	SeedBurn int `json:"seed_burn"`
	// Artifacts are hashed checkpoint/diagnostic files collected after
	// gameplay. Small evidence may be inline; large evidence can be an S3
	// reference with Data omitted. Empty from older runners and scripted runs.
	Artifacts []Artifact `json:"artifacts,omitempty"`
	// ProgressEarly and ProgressFinal are the run's progress sampled
	// before the first objective ran and at the stop. Comparing the two
	// answers "did this run move?" from one dump. Both are nil when the run
	// never reached the agent loop (a validation or LoadState failure) or
	// came from a runner that predates the field. A single end-of-run
	// snapshot cannot make that distinction, which is why there are two
	// samples, not one.
	ProgressEarly *Progress `json:"progress_early,omitempty"`
	ProgressFinal *Progress `json:"progress_final,omitempty"`
}

// CheckpointReport is one in-flight checkpoint upload. The wall retains a
// bounded window of these independently of Finish.
type CheckpointReport struct {
	RunID     string     `json:"run_id"`
	Attempt   int        `json:"attempt,omitempty"`
	Artifacts []Artifact `json:"artifacts"`
}

// Artifact is one named blob in a FinishReport. Names are generated by
// PokePilot, never accepted from the model or a run ID. Inline artifacts use
// Data and leave Store empty. Remote artifacts leave Data empty and carry a
// storage locator plus the original byte size and SHA-256.
type Artifact struct {
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	SHA256    string `json:"sha256"`
	Data      []byte `json:"data,omitempty"`
	Store     string `json:"store,omitempty"`
	Bucket    string `json:"bucket,omitempty"`
	ObjectKey string `json:"object_key,omitempty"`
	Size      int64  `json:"size,omitempty"`
}
