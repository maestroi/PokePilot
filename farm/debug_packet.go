package farm

// DebugPacketVersion versions the compact run-debug handoff consumed by local
// coding-agent tooling. Keep this deliberately small: the full run-debug
// payload remains available through pokepilot_get_run_debug when escalation is
// genuinely required.
const DebugPacketVersion = 1

// DebugPacket is the bounded, model-facing diagnosis envelope for one run.
// It contains stable identities, the terminal error chain, exact reproduction
// artifacts, and a few targeted timeline facts without embedding raw model
// exchanges, screenshots, RAM, recordings, or the full activity history.
type DebugPacket struct {
	Version            int             `json:"version"`
	RunID              string          `json:"run_id"`
	Mode               string          `json:"mode"`
	Status             string          `json:"status,omitempty"`
	Game               string          `json:"game,omitempty"`
	Planner            string          `json:"planner,omitempty"`
	Goal               string          `json:"goal,omitempty"`
	Attempts           int             `json:"attempts,omitempty"`
	ErrorAttempts      int             `json:"error_attempts,omitempty"`
	Location           DebugLocation   `json:"location,omitempty"`
	ClassificationHint string          `json:"classification_hint,omitempty"`
	Finish             DebugFinish     `json:"finish,omitempty"`
	Failure            DebugFailure    `json:"failure,omitempty"`
	Triage             DebugTriage     `json:"triage,omitempty"`
	Repro              DebugRepro      `json:"repro,omitempty"`
	SearchTerms        []string        `json:"search_terms,omitempty"`
	Evidence           []DebugEvidence `json:"evidence,omitempty"`
	Next               []string        `json:"next,omitempty"`
}

type DebugLocation struct {
	Map uint8 `json:"map"`
	X   uint8 `json:"x"`
	Y   uint8 `json:"y"`
}

type DebugFinish struct {
	Attempt       int    `json:"attempt,omitempty"`
	Reason        string `json:"reason,omitempty"`
	Detail        string `json:"detail,omitempty"`
	RunnerVersion string `json:"runner_version,omitempty"`
}

type DebugFailure struct {
	ErrorChain string `json:"error_chain,omitempty"`
	LeafError  string `json:"leaf_error,omitempty"`
	Objective  string `json:"objective,omitempty"`
	Cause      string `json:"cause,omitempty"`
	Outcome    string `json:"outcome,omitempty"`
}

type DebugTriage struct {
	Key               string               `json:"key,omitempty"`
	Fingerprint       string               `json:"fingerprint,omitempty"`
	Count             int                  `json:"count,omitempty"`
	IssueNumber       int64                `json:"issue_number,omitempty"`
	IssueURL          string               `json:"issue_url,omitempty"`
	Status            string               `json:"status,omitempty"`
	Resolution        string               `json:"resolution,omitempty"`
	FixedRevision     string               `json:"fixed_revision,omitempty"`
	VerificationState string               `json:"verification_state,omitempty"`
	Actionable        bool                 `json:"actionable"`
	SolverAttempts    []DebugSolverAttempt `json:"solver_attempts,omitempty"`
}

type DebugSolverAttempt struct {
	Backend  string `json:"backend,omitempty"`
	Model    string `json:"model,omitempty"`
	State    string `json:"state,omitempty"`
	RunID    string `json:"run_id,omitempty"`
	Branch   string `json:"branch,omitempty"`
	PRNumber int64  `json:"pr_number,omitempty"`
	PRURL    string `json:"pr_url,omitempty"`
	Note     string `json:"note,omitempty"`
}

type DebugRepro struct {
	ContractArtifact string `json:"contract_artifact,omitempty"`
	Checkpoint       string `json:"checkpoint,omitempty"`
	Knowledge        string `json:"knowledge,omitempty"`
	Fingerprint      string `json:"fingerprint,omitempty"`
	ObservedRevision string `json:"observed_revision,omitempty"`
	Diagnostic       string `json:"diagnostic,omitempty"`
	Materialize      string `json:"materialize_command,omitempty"`
	Deterministic    bool   `json:"deterministic,omitempty"`
}

type DebugEvidence struct {
	Source  string `json:"source,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Attempt int    `json:"attempt,omitempty"`
	Round   int    `json:"round,omitempty"`
	Message string `json:"message,omitempty"`
	Detail  string `json:"detail,omitempty"`
}
