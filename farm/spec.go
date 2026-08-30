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
	Addrs []string `json:"addrs"`
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
}
