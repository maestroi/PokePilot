package farm

// ExperimentArm identifies one deployment in a paired benchmark.
type ExperimentArm struct {
	Name               string `json:"name"`
	Deployment         string `json:"deployment"`
	MaxParallelWorkers int    `json:"max_parallel_workers,omitempty"`
}

// ExperimentRequest is the operator wire contract for creating matched A/B
// runs. Every seed produces exactly one run per arm with identical gameplay
// settings; only Deployment differs.
type ExperimentRequest struct {
	Name               string        `json:"name"`
	ArmA               ExperimentArm `json:"arm_a"`
	ArmB               ExperimentArm `json:"arm_b"`
	Goal               string        `json:"goal"`
	Starter            string        `json:"starter,omitempty"`
	Seeds              []int64       `json:"seeds,omitempty"`
	SeedCount          int           `json:"seed_count,omitempty"`
	PlayStyle          string        `json:"play_style,omitempty"`
	RiskTolerance      string        `json:"risk_tolerance,omitempty"`
	WildEncounters     string        `json:"wild_encounters,omitempty"`
	ReasoningEffort    string        `json:"reasoning_effort,omitempty"`
	FPS                int           `json:"fps,omitempty"`
	MaxRounds          int           `json:"max_rounds,omitempty"`
	MaxFrames          int           `json:"max_frames,omitempty"`
	MaxParallelWorkers int           `json:"max_parallel_workers,omitempty"`
}

// ExperimentRunMeta is copied onto each generated run and persisted by the
// wall controller. Case is stable within a seed pair and Arm is "a" or "b".
type ExperimentRunMeta struct {
	ExperimentID string `json:"experiment_id"`
	Arm          string `json:"experiment_arm"`
	Case         string `json:"experiment_case"`
}

// ComparableRunConfig is the shared part of a paired run. Its canonical JSON
// is hashed by the wall; equality means the two arms differed only in model
// deployment/identity.
type ComparableRunConfig struct {
	GitRevision        string `json:"git_revision,omitempty"`
	ROMIdentity        string `json:"rom_identity,omitempty"`
	PromptIdentity     string `json:"prompt_identity,omitempty"`
	Seed               int64  `json:"seed"`
	Starter            string `json:"starter,omitempty"`
	Goal               string `json:"goal"`
	PlayStyle          string `json:"play_style,omitempty"`
	RiskTolerance      string `json:"risk_tolerance,omitempty"`
	WildEncounters     string `json:"wild_encounters,omitempty"`
	ReasoningEffort    string `json:"reasoning_effort,omitempty"`
	FPS                int    `json:"fps,omitempty"`
	MaxRounds          int    `json:"max_rounds,omitempty"`
	MaxFrames          int    `json:"max_frames,omitempty"`
	MaxParallelWorkers int    `json:"max_parallel_workers,omitempty"`
}
