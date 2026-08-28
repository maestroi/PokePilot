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
	RunID     string `json:"run_id"`
	Seed      int64  `json:"seed"`
	Planner   string `json:"planner"`
	Starter   string `json:"starter"`
	Dest      string `json:"dest"`
	FPS       int    `json:"fps"`
	MaxRounds int    `json:"max_rounds"`
	MaxFrames int    `json:"max_frames"`
}

// Heartbeat is the small, frequent status push a runner sends while a
// leased run is in progress.
type Heartbeat struct {
	RunID     string `json:"run_id"`
	Frame     uint64 `json:"frame"`
	Map       uint8  `json:"map"`
	X         uint8  `json:"x"`
	Y         uint8  `json:"y"`
	Trace     string `json:"trace"`
	StopSoFar string `json:"stop_so_far"`
}

// HeartbeatReply is the wall's answer to a heartbeat. Cancel asks the
// runner to finish the current run at its next natural boundary and lease
// again; it is not a kill signal.
type HeartbeatReply struct {
	Cancel bool `json:"cancel"`
}

// FinishReport is why a run ended, sent once when it stops.
type FinishReport struct {
	RunID     string   `json:"run_id"`
	Reason    string   `json:"reason"`
	Detail    string   `json:"detail"`
	TraceTail []string `json:"trace_tail"`
	SaveState []byte   `json:"save_state"`
	FramePNG  []byte   `json:"frame_png,omitempty"`
}
