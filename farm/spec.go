// Package farm holds the wire contract between a pokepilot runner and the
// pokewall orchestrator: lease/heartbeat/finish JSON, and (in client.go)
// the small HTTP client the runner uses to speak it. Nothing here reaches
// into emu, skill, agent, or red — those packages import nothing from
// farm, and farm imports nothing from them.
package farm

// Spec is one run's configuration, filled either by CLI flags (today) or
// by a lease from the wall (farm mode). Field names mirror the flags in
// cmd/pokepilot/main.go one for one.
type Spec struct {
	RunID string `json:"run_id"`
	// Attempt is which attempt of this run the wall is handing out: 1 for
	// the first, higher after retries. The runner echoes it back in its
	// FinishReport so a late finish from a dead attempt cannot settle a
	// newer one.
	Attempt int    `json:"attempt"`
	Seed    int64  `json:"seed"`
	Planner string `json:"planner"`
	Starter string `json:"starter"`
	Dest    string `json:"dest"`
	// Goal is the task statement for the llm planner: what to achieve,
	// never how. Empty means no goal (the pre-Goal prompt).
	Goal      string `json:"goal,omitempty"`
	FPS       int    `json:"fps"`
	MaxRounds int    `json:"max_rounds"`
	MaxFrames int    `json:"max_frames"`
	// Endless asks the wall to queue a successor when this run settles,
	// so idle workers keep picking up work. RandomSeed picks a fresh
	// seed on each successor; otherwise the seed is copied.
	Endless    bool `json:"endless,omitempty"`
	RandomSeed bool `json:"random_seed,omitempty"`
}

// Heartbeat is the small, frequent status push a runner sends while a
// leased run is in progress.
type Heartbeat struct {
	RunID string `json:"run_id"`
	Frame uint64 `json:"frame"`
	Map   uint8  `json:"map"`
	X     uint8  `json:"x"`
	Y     uint8  `json:"y"`
	Trace string `json:"trace"`
	// Question is the offered menu the planner was last asked, numbered
	// the way the model saw it. Decision is the objective that ask
	// resolved to, or empty while the model is still answering. Both are
	// empty until the first plan of an llm run; a scripted run never
	// fills them.
	Question  string `json:"question,omitempty"`
	Decision  string `json:"decision,omitempty"`
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
}

// LLMStats is the planner tally a runner pushes on its heartbeats: round
// progress, how often it re-picks an objective it already picked (Repeats —
// the wander signal), think time, spend, and the replies that never
// resolved. The wall carries it verbatim for the console; the field names
// are the same JSON keys the runner's watch page renders, so both surfaces
// show one number.
type LLMStats struct {
	Round      int `json:"round"`
	RoundsLeft int `json:"rounds_left"`
	// Calls counts every ask, Rounds only the ones that became an
	// objective: the gap between them is re-asks after a rejected reply.
	Calls    int `json:"calls"`
	Rounds   int `json:"rounds"`
	Rejected int `json:"rejected"`
	Repeats  int `json:"repeats"`

	AvgOffered  float64 `json:"avg_offered"`
	LastSeconds float64 `json:"last_seconds"`
	AvgSeconds  float64 `json:"avg_seconds"`

	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	Transport        int `json:"transport"`
	Fallbacks        int `json:"fallbacks"`

	Intent    string `json:"intent"`
	IntentAge int    `json:"intent_age"`

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
	// Artifacts are hashed checkpoint files collected after gameplay.
	// Empty from older runners and from scripted runs.
	Artifacts []Artifact `json:"artifacts,omitempty"`
}

// CheckpointReport is one in-flight checkpoint upload. The wall retains a
// bounded window of these independently of Finish.
type CheckpointReport struct {
	RunID     string     `json:"run_id"`
	Attempt   int        `json:"attempt,omitempty"`
	Artifacts []Artifact `json:"artifacts"`
}

// Artifact is one named blob in a FinishReport. Names are generated by
// PokePilot, never accepted from the model or a run ID. Validation does
// not interpret .state contents.
type Artifact struct {
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	SHA256    string `json:"sha256"`
	Data      []byte `json:"data"`
}
