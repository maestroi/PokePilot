package agent

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
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
	// and the same round was re-asked in a DIFFERENT form (see Retry). It
	// is the diagnostic that separates a loop problem from a capacity
	// problem: a run full of them answered but could not answer in shape,
	// while zero means every reply the model gave was structurally fine.
	ReplyRetries int
	// PromptTokens and CompletionTokens are what the run's model calls
	// spent, summed over EVERY call including rejected re-asks — a re-ask
	// costs a full prompt, and a scoreboard row that hides that reads
	// cheaper than the run was. Zero means the planner does not report
	// usage (UsagePlanner), which is "not reported", never "free".
	PromptTokens     int
	CompletionTokens int
	// ProgressEarly is the progress sampled before the first objective
	// ran; ProgressFinal, the one at the stop. Together they let a dump
	// of ONE run answer "did this move?": a run that stalled at round 3
	// and one that progressed steadily can stop looking identical, and
	// only the pair of samples tells them apart. Both are nil when the
	// run stopped before its first observation (an invalid frame budget, a
	// cancel before the first frame, or a ROM the graph could not build),
	// so a nil pair means "never played", never "played and moved nothing".
	ProgressEarly *Progress
	ProgressFinal *Progress
	// Planning is the run-owned three-tier planner telemetry and final plan.
	Planning PlanningStats
}

// Progress is one snapshot of how far a run has gotten: badges held,
// story events set, distinct maps the player has stood on, and the map
// the player stands on. Run samples it twice — before the first
// objective and at the stop — and carries both on Result. The values
// come from state the run already has: the observation's decoded badges
// and events (red/state, via Observe) and the run's own Knowledge.Visited
// for the maps, so sampling costs nothing the run was not already paying.
type Progress struct {
	// Round is the run round the sample was taken at: 0 before the first
	// objective ran, N after round N settled.
	Round int
	// Badges is how many badges the party holds; Events, how many story
	// event flags are set. Both are what red/state decoded, never counted
	// here.
	Badges int
	Events int
	// Maps is how many distinct maps the player has stood on this run.
	Maps int
	// Map and MapName are where the player stands at the sample.
	Map     uint8
	MapName string
}

// MaxReplyRetries is how many times the planner may be asked for one
// round's choice: the initial ask plus re-asks. It is NOT "ask the same
// thing three times": at temperature 0 an unchanged request returns the
// same bytes (MEASURED by S9-12: the model repeated the identical invalid
// reply through all three asks), so planWithRetries classifies each
// rejection (classifyRetry) and makes every re-ask differ — a raised
// temperature for a wrong-shaped reply, a raised max_tokens for a "length"
// truncation — and spends no re-ask at all on a class that cannot change
// (a model mismatch, a non-length non-stop finish). A round that exhausts
// the asks stops with StopError.
const MaxReplyRetries = 3

// RetryTemperature is the sampling temperature a wrong-shaped-reply retry
// asks with (see Retry). Non-zero, so a deterministic sampler can produce
// different bytes at all; small, so a reply that was in the right shape
// but off in a detail is still overwhelmingly likely to stay a valid
// choice.
const RetryTemperature = 0.3

// Retry describes how one re-ask differs from the ask it repeats. A
// re-ask that is byte-identical to the ask it follows costs the full call
// latency to obtain the same bytes, so every retry must change the request
// in the way its rejection class needs:
//
//	Feedback        the rejection quoted back into the prompt (wrong shape)
//	Temperature     raised sampling (wrong shape; the only change that
//	                makes a temperature-0 sampler emit different bytes)
//	MaxTokensFactor a larger completion budget ("length" truncation; a
//	                request change, not a prompt change)
//
// A zero Retry is the ordinary ask and changes nothing.
type Retry struct {
	// Feedback is the rejection text quoted back to the planner. Empty
	// means the prompt is unchanged.
	Feedback string
	// Temperature, when non-nil, overrides the planner's sampling
	// temperature for this ask only.
	Temperature *float64
	// MaxTokensFactor, when > 1, multiplies the planner's effective
	// max_tokens for this ask only.
	MaxTokensFactor int
}

// describe names what this re-ask changes, for the run log.
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

// UsagePlanner reports the tokens its model calls spent. LLMPlanner
// implements it; scripted planners do not, and a run without one reports
// zero usage, which means "not reported", never "free". The totals are
// read at the end of the run, not per round: they are a property of the
// whole run, and a planner that sums over every call (re-asks included) is
// already doing the accumulation.
type UsagePlanner interface {
	Usage() (prompt, completion int)
}

// FeedbackPlanner is a planner that can be re-asked about the same round
// in a form that differs from the ask it repeats (Retry). LLMPlanner
// implements it; the scripted planners do not, and a plain Planner's error
// keeps stopping the run exactly as before.
type FeedbackPlanner interface {
	NextRetry(obs Observation, offered []Objective, r Retry) (Objective, error)
}

// classifyRetry decides how a rejected reply is re-asked, or that it is
// not re-asked at all. It classifies on the typed errors the planner
// returns, not on message text:
//
//	ErrDone                 not a rejection: the planner is finished.
//	ErrModelMismatch        never retried — the server answered with a
//	                        different model than the one requested, and no
//	                        re-ask changes which model the server loads.
//	ErrNotFinished "length" retried with a doubled max_tokens and an
//	                        UNCHANGED prompt: the reply was cut off at the
//	                        completion budget, so the budget is what must
//	                        change, not the question.
//	ErrNotFinished (other)  never retried — a content filter or any other
//	                        non-stop reason is deterministic at temperature
//	                        0 with an unchanged prompt.
//	any other error         retried with the rejection quoted back AND a
//	                        raised temperature: the model answered in the
//	                        wrong shape, and a temperature-0 re-ask of the
//	                        same prompt returns the same bytes (MEASURED by
//		                    S9-12).
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

// planWithRetries asks the planner for this round's objective and, when it
// rejects its own reply (a planner error that is not ErrDone), re-asks the
// SAME round in a form that differs from the ask it repeats (classifyRetry):
// the observation never changes — a malformed reply says nothing about the
// world — but the request does. It returns the objective (meaningful only
// when err is nil), the planner's error — ErrDone passes through untouched;
// any other non-nil error means the asks are exhausted, the rejection class
// cannot change on a re-ask, or a planner that cannot take retries errored
// at all — and n, how many re-asks happened, so the caller can count them
// in the result. Run classifies the error into a Stop reason from the error
// itself: the zero value of Stop is StopUnset ("no reason set yet"), and a
// finished planner must never be read as "keep going".
func planWithRetries(log io.Writer, round int, p Planner, obs Observation, offered []Objective) (Objective, error, int) {
	obj, err := p.Next(obs, offered)
	fp, canRetry := p.(FeedbackPlanner)
	retries := 0
	for err != nil && !errors.Is(err, ErrDone) && canRetry && retries < MaxReplyRetries-1 {
		r, retryable := classifyRetry(err)
		if !retryable {
			if log != nil {
				fmt.Fprintf(log, "round %d: reply rejected and not retried (cannot change on a re-ask): %v\n",
					round, err)
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

// defaultStuckAfter is the StuckAfter used when Budget leaves it zero.
// Small on purpose: a run that is not stuck will change the map, the
// position, the party, or the events within a few rounds.
const defaultStuckAfter = 3

// defaultMaxConsecutiveFailures bounds a streak of failed objectives when
// Budget leaves it zero. It is only reached by DIFFERENT failures: the same
// objective failing with the same error twice stops the run before this
// count, because two identical failures are a stronger signal than any
// number of different ones.
const defaultMaxConsecutiveFailures = 3

// defaultCheckpointKeep is the ring size when Budget.CheckpointDir is set
// and CheckpointKeep is left zero. A state is ~292KB (measured,
// docs/DESIGN.md 1.3), so sixteen states are ~4.7MB per run: small enough
// that a sweep of runs cannot fill a disk, large enough that the rounds
// around a failure — the only ones anyone reads — are all still there.
const defaultCheckpointKeep = 16

// historyCap is how many past rounds the observation carries: enough to
// show a two- or three-step oscillation, short enough not to crowd out
// the current state.
const historyCap = 6

// dialogueCap is how many lines of recent dialogue the observation
// carries. Same shape as historyCap: the game's hints are worth a screen,
// not a transcript.
const dialogueCap = 6

// Budget bounds a run. MaxFrames is always required as a last-resort
// emulator watchdog. MaxRounds is optional: zero means no round cap, because
// normal LLM runs are goal-driven and should end on success, a real failure,
// or stagnation rather than an arbitrary decision count.
type Budget struct {
	// MaxRounds is an optional emergency objective cap. Zero means no round
	// cap. A positive value still produces StopBudget when reached so explicit
	// experiments can request a fixed decision budget.
	MaxRounds int
	MaxFrames int
	// Goal optionally configures the run-owned completion contract. Known
	// presets and documented structured forms are evaluated deterministically;
	// arbitrary free text remains prompt-only. When empty, a RunGoalProvider
	// planner may supply the raw goal already used for its prompt.
	Goal string
	// StuckAfter is how many consecutive objectives may leave the
	// observation unchanged before the run stops with StopStuck.
	// Zero means defaultStuckAfter.
	StuckAfter int
	// StagnationAfter is the long watchdog: how many completed rounds may
	// pass without a badge, story event, new map, party growth or level gain.
	// Movement, HP, money and consumable churn do not reset it. Zero means
	// defaultStagnationAfter.
	StagnationAfter int
	// MaxConsecutiveFailures is how many objectives may fail in a row
	// before the run stops with StopFailed. The same objective failing
	// with the same error twice stops it sooner, whatever this is.
	// Zero means defaultMaxConsecutiveFailures.
	MaxConsecutiveFailures int
	// Log receives one line per round. Nil means no logging.
	Log io.Writer
	// CheckpointDir, when non-empty, makes Run write a save-state snapshot
	// before every objective it executes, under this directory, named after
	// the round, the objective, and the frame. That is what makes a failed
	// run inspectable — and replayable from the round before it went wrong
	// — instead of re-run from boot. It is the system observing the run:
	// there is no objective Kind for saving or loading, so the planner can
	// never save-scum (lose to Brock, reload, retry until the RNG
	// cooperates). Off by default; never unbounded: the directory holds a
	// ring of the last CheckpointKeep states.
	CheckpointDir string
	// CheckpointKeep is how many checkpoints per run are kept. Zero means
	// defaultCheckpointKeep.
	CheckpointKeep int
	// ResumeFrom, when non-empty, is a checkpoint .state file written by a
	// CheckpointDir ring. Run restores that save state into m and loads the
	// knowledge file written beside it before round 1: the game and the
	// run's understanding start from the same captured moment, so a resumed
	// run is not amnesiac in a world it has already explored — every map is
	// not unvisited, every place unnamed, every completed one-shot offered
	// again. Both halves are loaded from this ONE path (see
	// LoadCheckpointMemory), which keeps the pairing structural: knowledge
	// can only be restored onto the exact save state it was captured with.
	// A plain Run leaves this empty and behaves exactly as before.
	ResumeFrom string
	// Cancel, when closed, stops Run before the next round's objective
	// starts. Nil means never cancelled — the zero value of Budget keeps
	// every existing caller's behavior unchanged. Checked between rounds
	// only: an objective already in flight always finishes. The farm
	// runner needs this to stop a leased run; it is orthogonal to the
	// checkpoint fields above and both are kept.
	Cancel <-chan struct{}
}

// dialogueTape records what the game says, sampled on the emulator's
// sample hook, for the planner to read at round boundaries. A box that
// opens and closes inside one Execute is invisible to a post-Execute
// Observe, so the only way the gym guide's line reaches the prompt is by
// sampling while the game runs.
//
// Gen 1 types dialogue out a character at a time, so a line is kept only
// once two consecutive samples agree (typing has caught up or paused),
// and it is deduped against the last kept line. Each settled line is also
// forwarded to the emulator's trace: Run installs this tape as the sample
// hook, replacing whatever was there (the watch page's own tracer does
// exactly this forwarding), so the /trace panel keeps working.
type dialogueTape struct {
	mu      sync.Mutex
	lines   []string // settled lines, oldest first
	last    string   // last line kept
	pending string   // last reading, not yet stable
	stable  bool
	// maps are the map ids the player has stood on since the last read,
	// sampled for the same reason the dialogue is: an objective can walk
	// through half of Kanto inside one Execute, and a post-Execute Observe
	// sees only where it ended. Without this, Knowledge.Visited grows by at
	// most one map per round, so the parcel errand walks to Viridian and
	// back and the run still does not know Viridian City exists — the menu
	// stays the four maps around Pallet and the planner shuffles between
	// them forever. MEASURED 2026-08-30 on a live run: 18 rounds inside
	// Pallet after the Pokedex.
	maps map[uint8]bool
}

// sample runs on the goroutine stepping the emulator, so it may read
// memory here; Run reads recent() from its own goroutine, under the lock.
func (d *dialogueTape) sample(m *emu.Emu) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	text := ""
	if ds := state.DecodeDialogue(&mem); ds != nil {
		text = ds.Text
	}
	if d.observeText(text) {
		m.TraceNote("dialogue", text)
	}
	d.noteMap(mem.U8(sym.CurMap))
}

// noteMap records one sampled map id.
func (d *dialogueTape) noteMap(id uint8) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.maps == nil {
		d.maps = map[uint8]bool{}
	}
	d.maps[id] = true
}

// observeText folds one sampled screen-text value into the recent dialogue.
// A line may pause long enough while typing for several growing prefixes to
// settle; those replace the current page instead of becoming separate prompt
// entries. d.last is cleared when the box closes, so the first settled text
// in a genuinely new page always appends.
func (d *dialogueTape) observeText(text string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch {
	case text == "":
		// Box closed: forget the line so saying it again later re-keeps it.
		d.last, d.pending, d.stable = "", "", false
		return false
	case text != d.pending:
		d.pending, d.stable = text, false
		return false
	case !d.stable:
		d.stable = true
		if text == d.last {
			return false
		}
		switch {
		case d.last == "" || len(d.lines) == 0:
			// A genuinely new box: the last one closed (or this is the
			// first), so this starts its own entry.
			d.lines = append(d.lines, text)
		case strings.HasPrefix(text, d.last):
			// Still typing the same page: replace, do not accumulate.
			d.lines[len(d.lines)-1] = text
		default:
			// A later PAGE of a box that is still open. It is the same
			// utterance, so it EXTENDS the entry instead of starting a new
			// one. Gen 1 breaks a sentence across pages at "para", and
			// splitting there is what left the run holding "You can't go
			// through here!" while "This is private property!" — the rest
			// of the same breath — was filed as unrelated chatter and
			// dropped by the requirement filter. An utterance is what the
			// game said, not what fit on the screen at once.
			d.lines[len(d.lines)-1] += " " + text
		}
		d.last = text
		if len(d.lines) > dialogueCap {
			d.lines = d.lines[len(d.lines)-dialogueCap:]
		}
		return true
	}
	return false
}

// seenMaps returns the map ids sampled since the last call, and clears
// them: the caller folds them into Knowledge, which is the durable copy.
func (d *dialogueTape) seenMaps() []uint8 {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]uint8, 0, len(d.maps))
	for id := range d.maps {
		out = append(out, id)
	}
	d.maps = nil
	return out
}

// recent returns a copy of the settled lines, oldest first.
func (d *dialogueTape) recent() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, len(d.lines))
	copy(out, d.lines)
	return out
}

// roundRecoveryBudget bounds the between-rounds dialogue recovery, in
// frames. Ten seconds of game time: a leftover box is a page or two, which
// is a handful of A presses, so anything that has not cleared by here is a
// real failure to report rather than something to keep waiting on. It is
// deliberately far shorter than skill's own 10000-frame budget, which is
// sized for a forced cutscene — spending that between every round would be
// nearly three minutes of wall clock on a paced run.
const roundRecoveryBudget = 600

// observeAfter pages away a text box the finished objective left open, then
// observes. Every skill demands a controllable start and refuses otherwise
// ("skill: Buy: not controllable (wFontLoaded=0x0001)"), and no objective
// exists whose job is to close a box — so a single round that ends
// mid-dialogue poisons EVERY round after it, whatever the planner picks,
// until the run dies on repeated failures. MEASURED 2026-08-30: a run
// reached the Viridian mart, bought potions, and then failed every
// remaining round on a box that was never closed.
//
// RecoverDialogue is the right tool and already exists: it only pages
// ordinary text, never sends a direction, and stops without pressing
// anything if a choice is up — answering a question the run did not ask is
// not recovery, so a choice is left alone and the next objective's own
// guard reports it.
func observeAfter(m *emu.Emu, romData []byte, log io.Writer) Observation {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		res := skill.RecoverDialogue(m, roundRecoveryBudget)
		if log != nil {
			if res.Stop == skill.DialogueRecovered {
				fmt.Fprintf(log, "  recovered: closed a text box the objective left open (%d A press(es))\n", res.Presses)
			} else {
				fmt.Fprintf(log, "  recovery failed (%s): the next objective will refuse to start; box says %q\n",
					recoveryStopName(res.Stop), res.Text)
			}
		}
	}
	return Observe(m, romData)
}

// recoveryStopName names a recovery outcome for the run log. The type has
// no String() of its own, and a bare enum number in a log a human reads in
// the morning is not a diagnosis.
func recoveryStopName(s skill.DialogueRecoveryStop) string {
	switch s {
	case skill.DialogueRecovered:
		return "recovered"
	case skill.DialogueChoiceRequired:
		return "a choice is up and this layer does not answer questions"
	case skill.DialogueBudgetExhausted:
		return "the box never closed"
	case skill.DialogueUnexpectedMode:
		return "a battle, not a text box"
	case skill.DialogueMenuOpen:
		return "a menu is open and this layer does not operate menus"
	}
	return fmt.Sprintf("unknown stop %d", int(s))
}

// appendHistory adds one round to the run's history, keeping at most
// historyCap entries. It returns a fresh slice each time so an observation
// that already holds a copy cannot be mutated by a later round.
func appendHistory(h []RoundRecord, r RoundRecord) []RoundRecord {
	out := make([]RoundRecord, 0, len(h)+1)
	if len(h) >= historyCap {
		out = append(out, h[len(h)-historyCap+1:]...)
	} else {
		out = append(out, h...)
	}
	return append(out, r)
}

// Run drives observe -> plan -> execute until the run-owned deterministic
// goal is done, a prompt-only planner is done, the world is demonstrably
// stuck, a hard failure occurs, or a safety watchdog fires. Every executed
// objective is normalized into ObjectiveResult before policy is applied:
// blocked gameplay is replanned, while ownership/controller/invariant defects
// stop immediately instead of being fed back to the planner as ordinary play.
//
// There are two distinct progress watchdogs. StuckAfter catches a few
// consecutive objectives that literally leave the observation unchanged.
// StagnationAfter catches longer moving loops by requiring occasional
// monotonic game progress (badge, story event, new map, party growth or level
// gain). MaxRounds is only an optional experiment/emergency cap; zero means
// goal-driven with no hard round limit. MaxFrames remains the final emulator
// guardrail.
func Run(m *emu.Emu, romData []byte, p Planner, budget Budget) Result {
	if budget.MaxRounds < 0 || budget.MaxFrames <= 0 {
		return Result{
			Stop: StopError,
			Err:  errors.New("agent: Run: MaxRounds cannot be negative and MaxFrames must be positive"),
		}
	}
	// Cancelled before we start: return without touching the emulator or the
	// ROM. This must precede BuildGraph below, because a caller that is
	// already cancelled is allowed to pass nils — the farm's wall does
	// exactly that when a lease is revoked before the run begins.
	select {
	case <-budget.Cancel:
		return Result{Stop: StopBudget, Rounds: 0}
	default:
	}
	// Parse the optional deterministic goal before touching ROM state. This
	// makes malformed documented syntax a configuration error, not a gameplay
	// failure, and lets callers validate a run without a ROM.
	runGoal, deterministicGoal, err := resolveRunGoal(p, budget.Goal)
	if err != nil {
		return Result{Stop: StopError, Err: err}
	}
	// Route geometry for the menu: which map's exits lead where. Built
	// once, like the ROM itself; it names no places.
	graph, err := world.BuildGraph(romData)
	if err != nil {
		return Result{Stop: StopError, Err: fmt.Errorf("agent: Run: build map graph: %w", err)}
	}
	adjacency := make(map[uint8][]uint8, len(graph.Edges))
	for from, edges := range graph.Edges {
		for _, e := range edges {
			adjacency[from] = append(adjacency[from], e.To)
		}
	}
	known := NewKnowledge(adjacency)
	// intent and intentAge are what the planner said with its last choice,
	// and how many rounds it has gone unchanged. Run only CARRIES them:
	// it never writes, edits or summarises the sentence — generating an
	// intent would be planning for the model again, which defeats the
	// measurement this exists to take (S9-7).
	intent, intentAge := "", 0
	resumedPlan := Plan{}
	if budget.ResumeFrom != "" {
		// Resume from a checkpoint: restore the save state and the knowledge
		// captured beside it, both from this one path. The knowledge file's
		// name is derived from the state file's (knowledgeFileName), so the
		// two cannot be loaded independently — see LoadCheckpointMemory.
		stateBytes, err := os.ReadFile(budget.ResumeFrom)
		if err != nil {
			return Result{Stop: StopError, Err: fmt.Errorf("agent: Run: resume %s: %w", budget.ResumeFrom, err)}
		}
		if err := m.LoadState(stateBytes); err != nil {
			return Result{Stop: StopError, Err: fmt.Errorf("agent: Run: resume %s: LoadState: %w", budget.ResumeFrom, err)}
		}
		mem := LoadCheckpointMemory(budget.ResumeFrom, adjacency, budget.Log)
		known = mem.Knowledge
		intent, intentAge = mem.Intent, mem.IntentAge
		resumedPlan = mem.Plan.clone()
	}
	var ring *checkpointRing
	if budget.CheckpointDir != "" {
		keep := budget.CheckpointKeep
		if keep <= 0 {
			keep = defaultCheckpointKeep
		}
		ring = &checkpointRing{dir: budget.CheckpointDir, keep: keep}
	}
	stuckAfter := budget.StuckAfter
	if stuckAfter <= 0 {
		stuckAfter = defaultStuckAfter
	}
	stagnationAfter := budget.StagnationAfter
	if stagnationAfter <= 0 {
		stagnationAfter = defaultStagnationAfter
	}
	maxConsecFailures := budget.MaxConsecutiveFailures
	if maxConsecFailures <= 0 {
		maxConsecFailures = defaultMaxConsecutiveFailures
	}

	res := Result{Completed: []Objective{}, Outcomes: []ObjectiveResult{}}
	startFrame := m.FrameCount()
	tape := &dialogueTape{}
	m.AlsoSample(tape.sample)
	var history []RoundRecord
	last := Observe(m, romData)
	planning := newRunPlanning(resumedPlan)
	quarantine := newFailureQuarantine()
	notifyPlanning(p, planning.snapshot())
	knownRequirementCount := len(known.Requirements)
	observedBadges, observedEvents := len(last.Badges), len(last.Events)
	stuckEscalated, stagnationEscalated := false, false
	failureEscalated := map[string]bool{}
	// The early progress sample: what the run started with, taken before
	// any objective ran. It is the baseline the finish sample is compared
	// against in the finish dump; a single end-of-run snapshot cannot
	// say whether the run moved at all.
	early := progressOf(last, known, 0)
	res.ProgressEarly = &early
	majorProgress := majorProgressMarkOf(last, known)
	lastMajorProgressRound := 0
	// lastUnroutable is the last set logUnroutable printed, so a run standing
	// still logs the dead end once instead of every round.
	lastUnroutable := ""
	stuck := 0
	consecFailures := 0 // consecutive failed objectives; a success resets it
	lastFailKey := ""
	retreatStreak := 0           // consecutive train retreats ending at the SAME level; capped like any other failure streak
	lastRetreatLevel := uint8(0) // the lead's level after the last retreat, 0 meaning none yet

	for round := 1; ; round++ {
		select {
		case <-budget.Cancel:
			return Result{Stop: StopBudget, Rounds: round - 1}
		default:
		}

		// The runtime-owned goal has precedence over every watchdog and over
		// another planner/model call. This is a pure read of the settled
		// observation; planners may mirror it but cannot decide it.
		if deterministicGoal {
			status := evaluateRunGoal(p, runGoal, last, round, budget.MaxRounds, intent, intentAge)
			res.GoalStatus = &status
			if status.Complete {
				res.Stop = StopDone
				break
			}
		}

		if roundCapReached(round, budget.MaxRounds) {
			res.Stop = StopBudget
			break
		}

		// Fold this round's observation into the run's knowledge before
		// building the menu: the map the player stands on is known, and a
		// place name the game spoke stays known even after it scrolls out
		// of the dialogue window. Then rebuild the menu: what is possible
		// depends on where the player is and what they already have.
		noteObservation(known, last)
		if len(known.Requirements) > knownRequirementCount {
			planning.request("new_requirement")
			knownRequirementCount = len(known.Requirements)
		}
		if round > 1 && len(last.Badges) > observedBadges {
			planning.request("badge_changed")
		}
		if round > 1 && len(last.Events) > observedEvents {
			planning.request("story_changed")
		}
		observedBadges, observedEvents = len(last.Badges), len(last.Events)
		// Every map the last objective actually walked through, not just
		// the one it ended on: see dialogueTape.maps.
		for _, id := range tape.seenMaps() {
			known.SawMap(id)
		}
		// Places the router cannot reach from here are withheld from the menu
		// (Offer), which is right for the run and wrong for us: a menu that
		// silently shrinks is how the Mt. Moon dead end stayed invisible for a
		// whole run. Log the withholding, so "we could not go there" is a
		// record someone can act on rather than an absence nobody sees. Logged
		// only when the set CHANGES, so standing still does not spam the log,
		// and never for the empty set.
		logUnroutable(budget.Log, round, last, &lastUnroutable)

		// The short stuck detector below catches objectives that literally
		// changed nothing. This longer detector catches moving loops. It runs
		// at the next round boundary so it sees every map sampled during the
		// previous Execute as well as the settled badge/event/party state.
		// We calculate stagnation here but do not stop yet: deterministic
		// completion was checked above, so a goal reached exactly on the
		// watchdog boundary is success, not stuck.
		currentMajorProgress := majorProgressMarkOf(last, known)
		if majorProgress.absorb(currentMajorProgress) {
			lastMajorProgressRound = round - 1
			stagnationEscalated = false
			if budget.Log != nil && round > 1 {
				fmt.Fprintf(budget.Log, "round %d: major progress -> %s\n", round-1, majorProgress)
			}
		}
		completedRounds := round - 1
		stagnantRounds := completedRounds - lastMajorProgressRound
		if stagnantRounds >= stagnationAfter {
			if planning.hasStrategist(p) {
				if !replanOnce(&stagnationEscalated) {
					res.Stop = StopStuck
					if budget.Log != nil {
						fmt.Fprintf(budget.Log, "stagnation watchdog recurred after strategic replan: %d rounds without major progress; high-water mark: %s\n", stagnantRounds, majorProgress)
					}
					break
				}
				planning.request("stagnation")
				lastMajorProgressRound = completedRounds
			} else {
				res.Stop = StopStuck
				if budget.Log != nil {
					fmt.Fprintf(budget.Log, "stagnation watchdog: %d rounds without major progress; high-water mark: %s\n", stagnantRounds, majorProgress)
				}
				break
			}
		}

		// The walls the game has stated stay visible every round: Knowledge
		// keeps them across rounds (and checkpoints), and this is where the
		// planner reads them. A copy, so a later round cannot mutate what an
		// earlier observation already showed. Offer does not branch on these:
		// the run reports what it heard; the planner decides.
		if reqs := known.Requirements; len(reqs) > 0 {
			last.Requirements = append([]Requirement{}, reqs...)
		}
		// The failure tally, for the same reason and read the same way:
		// History scrolls, this does not.
		last.Failures = known.FailureList()
		now := offerWithTMHM(m, romData, last, known)
		now = quarantine.filter(last, now)
		if len(now) == 0 {
			res.Stop = StopError
			res.Err = errors.New("agent: Run: nothing is possible from here")
			break
		}

		// A rejected reply is a different kind of event from a failed
		// objective: it says nothing about the world, only that the model
		// answered in the wrong shape, so the same round is re-asked in a
		// form that differs from the ask it repeats (planWithRetries)
		// instead of stopping.
		// ErrDone and other errors are classified here from the error itself,
		// not from a stop value: the break must come from the error, or a
		// finished planner would read as "keep going" and execute an empty
		// objective.
		// The planner sees what it said last time: the same sentence, with
		// its age, is read back from this observation.
		last.Intent = intent
		last.IntentAge = intentAge
		last.Round = round
		last.RoundsLeft = roundsLeft(round, budget.MaxRounds)

		obj, fromPlan, err, retries := planning.choose(budget.Log, round, p, last, now)
		res.ReplyRetries += retries
		notifyPlanning(p, planning.snapshot())
		// A prompt-only run still treats planner exhaustion as completion.
		// With a deterministic goal, however, Run already evaluated the same
		// settled observation above: an ErrDone while that status is incomplete
		// is a false completion claim and therefore a run error.
		if errors.Is(err, ErrDone) {
			if deterministicGoal {
				status := GoalStatus{}
				if res.GoalStatus != nil {
					status = *res.GoalStatus
				} else {
					status = evaluateRunGoal(p, runGoal, last, round, budget.MaxRounds, intent, intentAge)
					res.GoalStatus = &status
				}
				res.Stop = StopError
				res.Err = incompleteGoalError(status)
				break
			}
			res.Stop = StopDone
			break
		}
		if err != nil {
			res.Stop = StopError
			res.Err = err
			break
		}
		// Carry the planner's sentence forward, verbatim. A different
		// non-empty intent replaces it (age 0); the same one, or silence,
		// ages it by one round — a model that keeps re-affirming or ignoring
		// its purpose has been chasing it just as long.
		switch {
		case obj.Intent != "" && obj.Intent != intent:
			intent, intentAge = obj.Intent, 0
		case intent != "":
			intentAge++
		}

		// The checkpoint is taken BEFORE Execute: it is the exact state the
		// decision was made in, and the resume point if this objective is
		// where the run went wrong.
		if ring != nil {
			if err := ring.write(m, round, obj, known, intent, intentAge, planning.Plan); err != nil {
				res.Stop = StopError
				res.Err = fmt.Errorf("agent: Run: checkpoint round %d: %w", round, err)
				break
			}
		}

		before := last
		objectiveResult, execErr := executeObjectiveResult(m, romData, obj)
		last = observeAfter(m, romData, budget.Log)
		objectiveResult.Final = last
		res.Rounds = round

		if execErr != nil {
			// The normalized result, rather than the raw error string, decides
			// whether this is ordinary gameplay blockage or a terminal
			// ownership/controller invariant. The raw typed error remains for
			// diagnostics, Knowledge failure tallies, and errors.Is checks.
			blackedOut := errors.Is(execErr, skill.ErrBlackedOut)
			retreated := errors.Is(execErr, skill.ErrTrainRetreat)
			trainProgressed := errors.Is(execErr, skill.ErrTrainProgress)
			if blackedOut {
				last.BlackedOut = true
				objectiveResult.Final = last
				objectiveResult.Summary += fmt.Sprintf(" (respawned in %s, money %d -> %d)",
					last.RespawnPlace, before.Money, last.Money)
			}
			res.Outcomes = append(res.Outcomes, objectiveResult)
			outcome := objectiveResult.HistoryText()

			known.Failed(obj, execErr)
			if trainProgressed {
				known.clearGymLossFailures()
			}
			history = appendHistory(history, RoundRecord{Objective: obj.String(), Outcome: outcome})
			last.History = history
			last.RecentDialogue = tape.recent()
			logRound(budget.Log, round, obj, outcome, last)

			// Observable goal completion wins over fault policy. The failure still
			// remains in Outcomes/telemetry, but it did not terminate the campaign.
			if deterministicGoal {
				status := evaluateRunGoal(p, runGoal, last, round, budget.MaxRounds, intent, intentAge)
				res.GoalStatus = &status
				if status.Complete {
					markLastOutcomeRecovered(&res)
					res.Stop = StopDone
					break
				}
			}

			action := actionFor(objectiveResult.Outcome)
			switch action {
			case actionChoice, actionStop:
				markLastOutcomeTerminal(&res)
				res.Stop, res.Err = StopError, execErr
				break
			case actionContinue:
				markLastOutcomeTerminal(&res)
				res.Stop = StopError
				res.Err = fmt.Errorf("agent: objective %s returned error with completed outcome: %w", obj, execErr)
				break
			case actionReplan:
				// The final transaction boundary is trustworthy. Keep the fault in
				// telemetry, quarantine this exact objective/state, and hand the
				// fresh observation back to policy instead of killing the run.
				quarantine.record(objectiveResult)
			}
			if res.Stop != StopUnset {
				break
			}

			failureKey := recoverableFailureKey(obj, objectiveResult)
			if planning.hasStrategist(p) {
				consecFailures++
				reason, key, terminal := recoverableFailureReplan(
					failureEscalated, obj, objectiveResult, blackedOut, retreated, consecFailures, maxConsecFailures,
				)
				if terminal {
					markLastOutcomeTerminal(&res)
					res.Stop, res.Err = StopFailed, execErr
					break
				}
				failureEscalated[key] = true
				planning.request(reason)
				notifyPlanning(p, planning.snapshot())
				markLastOutcomeRecovered(&res)
				lastFailKey = failureKey
				if m.FrameCount()-startFrame >= uint64(budget.MaxFrames) {
					res.Stop = StopBudget
					break
				}
				continue
			}
			if blackedOut || retreated {
				retreatLevel := uint8(0)
				if len(last.Party) > 0 {
					retreatLevel = last.Party[0].Level
				}
				if retreated && retreatLevel != 0 && retreatLevel == lastRetreatLevel {
					retreatStreak++
				} else if retreated {
					retreatStreak = 1
					lastRetreatLevel = retreatLevel
				} else {
					retreatStreak, lastRetreatLevel = 0, 0
				}
				if retreated && retreatStreak >= maxConsecFailures {
					markLastOutcomeTerminal(&res)
					res.Stop, res.Err = StopFailed, execErr
				} else {
					markLastOutcomeRecovered(&res)
				}
				lastFailKey = ""
				if m.FrameCount()-startFrame >= uint64(budget.MaxFrames) && res.Stop == StopUnset {
					res.Stop = StopBudget
				}
				if res.Stop != StopUnset {
					break
				}
				continue
			}
			retreatStreak, lastRetreatLevel = 0, 0

			consecFailures++
			faultTerminal := false
			switch {
			case lastFailKey != "" && failureKey == lastFailKey:
				faultTerminal = true
				res.Stop, res.Err = StopFailed, execErr
			case consecFailures >= maxConsecFailures:
				faultTerminal = true
				res.Stop, res.Err = StopFailed, execErr
			case m.FrameCount()-startFrame >= uint64(budget.MaxFrames):
				res.Stop = StopBudget
			}
			if faultTerminal {
				markLastOutcomeTerminal(&res)
			} else {
				markLastOutcomeRecovered(&res)
			}
			if res.Stop != StopUnset {
				break
			}
			lastFailKey = failureKey
			continue
		}

		res.Outcomes = append(res.Outcomes, objectiveResult)
		res.Completed = append(res.Completed, obj)
		quarantine.clear(obj)
		planning.success(fromPlan)
		notifyPlanning(p, planning.snapshot())
		known.Done(obj)
		if obj.Kind == KindTalk {
			known.TalkedTo(before.Map, obj.X, obj.Y)
		}
		consecFailures = 0
		failureEscalated = map[string]bool{}
		lastFailKey = ""
		retreatStreak, lastRetreatLevel = 0, 0
		history = appendHistory(history, RoundRecord{Objective: obj.String(), Outcome: objectiveResult.HistoryText()})
		last.History = history
		last.RecentDialogue = tape.recent()
		logRound(budget.Log, round, obj, objectiveResult.HistoryText(), last)

		// Goal completion is checked before the short stuck detector and frame
		// budget. A state change that satisfies the goal must not be reported
		// as stuck/budget merely because the same objective also hit a guardrail.
		if deterministicGoal {
			status := evaluateRunGoal(p, runGoal, last, round, budget.MaxRounds, intent, intentAge)
			res.GoalStatus = &status
			if status.Complete {
				res.Stop = StopDone
				break
			}
		}

		if sameProgress(before, last) {
			stuck++
		} else {
			stuck = 0
			stuckEscalated = false
		}
		if stuck >= stuckAfter {
			if planning.hasStrategist(p) {
				if !replanOnce(&stuckEscalated) {
					res.Stop = StopStuck
					break
				}
				planning.request("stuck")
				stuck = 0
				notifyPlanning(p, planning.snapshot())
			} else {
				res.Stop = StopStuck
				break
			}
		}
		if m.FrameCount()-startFrame >= uint64(budget.MaxFrames) {
			res.Stop = StopBudget
			break
		}
	}

	res.Final = last
	res.Planning = planning.snapshot()
	if deterministicGoal {
		status := evaluateRunGoal(p, runGoal, last, res.Rounds, budget.MaxRounds, intent, intentAge)
		res.GoalStatus = &status
	}
	// The finish sample, at the same points the early one read: badges,
	// events, maps stood on, and where the player stands.
	final := progressOf(last, known, res.Rounds)
	res.ProgressFinal = &final
	if up, ok := p.(UsagePlanner); ok {
		res.PromptTokens, res.CompletionTokens = up.Usage()
	}
	return res
}

// progressOf decodes one progress sample from an observation and the
// run's knowledge: what red/state already put on the observation, plus
// the maps the player has actually stood on. round is the run round the
// sample was taken at (0 before the first objective ran).
func progressOf(obs Observation, k *Knowledge, round int) Progress {
	return Progress{
		Round:   round,
		Badges:  len(obs.Badges),
		Events:  len(obs.Events),
		Maps:    len(k.Visited),
		Map:     obs.Map,
		MapName: obs.MapName,
	}
}

// noteObservation folds one observation into the run's knowledge: the map
// the player is standing on and any place names in its recent dialogue.
func noteObservation(k *Knowledge, obs Observation) {
	k.SawMap(obs.Map)
	k.SawDialogue(obs.RecentDialogue, obs.MapName, obs.X, obs.Y)
}

// sameProgress reports whether two observations are identical on the fields
// that count as progress: the map, the position, the party count, and the
// event list. Everything else (facing, money, HP) may drift without saying
// the run is making headway.
func sameProgress(a, b Observation) bool {
	if a.Map != b.Map || a.X != b.X || a.Y != b.Y || a.PartyCount != b.PartyCount {
		return false
	}
	if len(a.Events) != len(b.Events) {
		return false
	}
	for i := range a.Events {
		if a.Events[i] != b.Events[i] {
			return false
		}
	}
	return true
}

// checkpointRing is the bounded record of a run: one save state per
// objective, kept as a ring of the last keep entries. The bound is the
// point — a state is ~292KB, so an unbounded directory would grow past a
// gigabyte on a 100-run sweep.
//
// Filenames are zero-padded round-frame-slug so that lexicographic order
// IS round order and eviction can keep the newest without reading anything.
type checkpointRing struct {
	dir  string
	keep int
}

// write snapshots m and evicts whatever the ring no longer holds. It runs
// on Run's goroutine before Execute, where m is not being stepped. The
// knowledge file is written HERE, beside the state it describes: same
// function, same base name, so the two cannot drift out of step — a
// surviving checkpoint is always a state and the understanding the run had
// at that moment, and resume (LoadCheckpointMemory) can only pair them.
func (c *checkpointRing) write(m *emu.Emu, round int, obj Objective, k *Knowledge, intent string, intentAge int, plans ...Plan) error {
	b, err := m.SaveState()
	if err != nil {
		return fmt.Errorf("SaveState: %w", err)
	}
	path := filepath.Join(c.dir, fmt.Sprintf("round-%03d-frame-%010d-%s.state",
		round, m.FrameCount(), checkpointSlug(obj)))
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := writeMemoryFile(path, k, intent, intentAge, plans...); err != nil {
		return fmt.Errorf("knowledge round %d: %w", round, err)
	}
	return c.evict()
}

// evict keeps only the newest keep states in the ring's directory. A state
// and the knowledge file beside it are ONE checkpoint: evicting the state
// evicts its knowledge too, and a knowledge file whose state is gone is
// orphaned — its save state no longer exists to pair with — so it is
// dropped as well. Knowledge without a state is exactly the "claims to know
// things this game state has not seen" case, just with no state at all.
func (c *checkpointRing) evict() error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", c.dir, err)
	}
	names := make([]string, 0, len(entries))
	stateSet := map[string]bool{}
	for _, en := range entries {
		if strings.HasSuffix(en.Name(), ".state") {
			names = append(names, en.Name())
			stateSet[en.Name()] = true
		}
	}
	// A knowledge file whose state is still in the ring belongs to it; only
	// a knowledge file with NO state beside it is orphaned and dropped.
	for _, en := range entries {
		if !isKnowledgeName(en.Name()) {
			continue
		}
		base := strings.TrimSuffix(strings.TrimSuffix(en.Name(), ".json"), fmt.Sprintf(".knowledge-v%d", memoryVersion))
		if stateSet[base+".state"] {
			continue
		}
		if err := os.Remove(filepath.Join(c.dir, en.Name())); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("evict %s: %w", en.Name(), err)
		}
	}
	sort.Strings(names)
	for _, n := range names[:max(0, len(names)-c.keep)] {
		if err := os.Remove(filepath.Join(c.dir, n)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("evict %s: %w", n, err)
		}
		if err := os.Remove(knowledgePathForState(filepath.Join(c.dir, n))); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("evict %s: %w", n, err)
		}
	}
	return nil
}

// checkpointSlug renders an objective for a filename: alphanumerics kept,
// everything else one dash.
func checkpointSlug(o Objective) string { return slugify(o.String()) }

// slugify is checkpointSlug's body over any string, so RAM captures keyed on
// something other than an objective (a planner intent, say) name their files
// the same way.
func slugify(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// logRound writes the one per-round line that makes an overnight run
// diagnosable in the morning: the round number, what was attempted, and
// where the player ended up.
func logRound(w io.Writer, round int, o Objective, outcome string, after Observation) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "round %d: %s -> %s, map %02x at (%d,%d)\n", round, o, outcome, after.Map, after.X, after.Y)
}

// logUnroutable records the places Offer is about to withhold because the
// router cannot plan a journey to them from this tile. prev carries the last
// line's set so a run standing in one spot logs once, not every round.
//
// This is the whole point of the field: withholding an unreachable place is
// what keeps the run moving, and logging it is what keeps the dead end
// findable. One without the other is either a wasted round every time or a
// bug nobody can see. The line names the map and tile because that is what
// makes it reproducible — "unroutable from Mt. Moon 1F" is a shrug,
// "unroutable from map 3b at (5,5)" is a probe.
func logUnroutable(w io.Writer, round int, obs Observation, prev *string) {
	if w == nil || len(obs.Unroutable) == 0 {
		return
	}
	line := strings.Join(obs.Unroutable, ", ")
	if line == *prev {
		return
	}
	*prev = line
	fmt.Fprintf(w, "round %d: unroutable from map %02x at (%d,%d): %s\n",
		round, obs.Map, obs.X, obs.Y, line)
}
