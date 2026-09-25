package agent

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Stop says why a run ended.
type Stop uint8

const (
	StopUnset  Stop = iota // the zero value: no stop reason set yet. Never reported.
	StopDone               // the runtime goal is satisfied, or a prompt-only planner reported ErrDone
	StopStuck              // no progress for too many rounds
	StopBudget             // an optional round cap or the frame budget ran out
	StopFailed             // consecutive objective failures exhausted the failure budget
	StopError              // a planner error, invalid completion claim, or nothing is possible from here
)

// Result is the outcome of a run.
type Result struct {
	Stop      Stop
	Rounds    int
	Completed []Objective
	// Outcomes is every objective transaction in execution order, including
	// recoverable blockage and terminal controller/ownership failures. It is
	// the durable structured counterpart to Final.History's short prompt text.
	Outcomes []ObjectiveResult
	// OutcomeTimings is parallel to Outcomes and anchors each completed
	// transaction to the exact emulator frame at which its settled observation
	// was captured. Keeping timing separate avoids changing ObjectiveResult's
	// portable semantic contract merely for media consumers.
	OutcomeTimings []ObjectiveTiming
	// Initial/StartFrame and Final/FinalFrame delimit the semantic portion of
	// this agent run. Farm media code converts these absolute emulator counters
	// to replay-relative frames using the recording start frame.
	Initial    Observation
	StartFrame uint64
	FinalFrame uint64
	// Err is why the run STOPPED, and it is nil unless Stop is StopError or
	// StopFailed. A round that failed and was recovered from does not set
	// it: those live in Final.History, which is also where the planner reads
	// them. A caller can therefore treat a non-nil Err as "this run ended
	// badly" without having to re-derive that from Stop.
	Err   error
	Final Observation
	// GoalStatus is the final authoritative status for an opted-in
	// deterministic goal. It is nil for free-text/prompt-only runs.
	GoalStatus *GoalStatus
	// ReplyRetries counts how many times the planner's reply was rejected
	// and the same round was re-asked in a DIFFERENT form (see Retry).
	ReplyRetries int
	// PromptTokens and CompletionTokens are what the run's model calls spent,
	// summed over every call including rejected re-asks.
	PromptTokens     int
	CompletionTokens int
	// ProgressEarly is the progress sampled before the first objective ran;
	// ProgressFinal is the one at the stop.
	ProgressEarly *Progress
	ProgressFinal *Progress
	// Planning is the run-owned three-tier planner telemetry and final plan.
	Planning PlanningStats
}

// ObjectiveTiming is replay/media timing for the matching Result.Outcomes
// element. Frame is the absolute emulator frame counter; callers that own a
// recording start frame can translate it without making the agent aware of
// recording or presentation concerns.
type ObjectiveTiming struct {
	Frame       uint64
	Round       int
	WallElapsed time.Duration
}

// ObjectiveActivity is an optional live observer event around one objective
// transaction. It reports the semantic execution boundary without coupling the
// agent package to farm/operator presentation.
type ObjectiveActivity struct {
	Stage     string
	Objective string
	Outcome   string
	Error     string
	Frame     uint64
	Round     int
}

// Progress is one snapshot of how far a run has gotten.
type Progress struct {
	Round    int
	Badges   int
	Events   int
	Maps     int
	Map      uint8
	MapName  string
	Coverage Coverage
}

// MaxReplyRetries is how many times the planner may be asked for one round's
// choice: the initial ask plus changed re-asks.
const MaxReplyRetries = 3

// RetryTemperature is the sampling temperature a wrong-shaped-reply retry uses.
const RetryTemperature = 0.3

// Retry describes how one re-ask differs from the ask it repeats.
type Retry struct {
	Feedback        string
	Temperature     *float64
	MaxTokensFactor int
}

func (r Retry) describe() string {
	var parts []string
	if r.Feedback != "" {
		parts = append(parts, "the rejection quoted back")
	}
	if r.Temperature != nil {
		parts = append(parts, fmt.Sprintf("temperature %.1f", *r.Temperature))
	}
	if r.MaxTokensFactor > 1 {
		parts = append(parts, fmt.Sprintf("max_tokens x%d", r.MaxTokensFactor))
	}
	if len(parts) == 0 {
		return "nothing (bug: a retry must differ from the ask it repeats)"
	}
	return strings.Join(parts, " + ")
}

// UsagePlanner reports the tokens its model calls spent.
type UsagePlanner interface {
	Usage() (prompt, completion int)
}

// FeedbackPlanner is a planner that can be re-asked about the same round in a
// form that differs from the ask it repeats.
type FeedbackPlanner interface {
	NextRetry(obs Observation, offered []Objective, r Retry) (Objective, error)
}

func classifyRetry(err error) (Retry, bool) {
	switch {
	case errors.Is(err, ErrDone), errors.Is(err, ErrModelMismatch):
		return Retry{}, false
	case errors.Is(err, ErrNotFinished):
		if IsLengthTruncation(err) {
			return Retry{MaxTokensFactor: 2}, true
		}
		return Retry{}, false
	default:
		temp := RetryTemperature
		return Retry{Feedback: err.Error(), Temperature: &temp}, true
	}
}

func planWithRetries(log io.Writer, round int, p Planner, obs Observation, offered []Objective) (Objective, error, int) {
	obj, err := p.Next(obs, offered)
	fp, canRetry := p.(FeedbackPlanner)
	retries := 0
	for err != nil && !errors.Is(err, ErrDone) && canRetry && retries < MaxReplyRetries-1 {
		r, retryable := classifyRetry(err)
		if !retryable {
			if log != nil {
				fmt.Fprintf(log, "round %d: reply rejected and not retried (cannot change on a re-ask): %v\n", round, err)
			}
			break
		}
		retries++
		if log != nil {
			fmt.Fprintf(log, "round %d: reply rejected (ask %d of %d): %v; re-ask differs by %s\n",
				round, retries+1, MaxReplyRetries, err, r.describe())
		}
		obj, err = fp.NextRetry(obs, offered, r)
	}
	return obj, err, retries
}

const defaultStuckAfter = 3
const defaultMaxConsecutiveFailures = 3
const defaultCheckpointKeep = 16
const historyCap = 6
const dialogueCap = 6

// Budget bounds a run. MaxFrames is always required as a last-resort emulator
// watchdog. MaxRounds is optional: zero means no round cap.
type Budget struct {
	MaxRounds int
	MaxFrames int
	Goal      string

	// Build identifies the running binary (e.g. a git SHA). It scopes
	// Knowledge failure tallies: a step's failure history from a different
	// build is stale evidence, not proof the step is still broken.
	Build string

	StuckAfter             int
	StagnationAfter        int
	MaxConsecutiveFailures int
	Log                    io.Writer
	CheckpointDir          string
	CheckpointKeep         int
	ResumeFrom             string
	Cancel                 <-chan struct{}
	OnObjective            func(ObjectiveActivity)
}
