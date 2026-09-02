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
	StopDone               // the planner reported ErrDone
	StopStuck              // no progress for too many rounds
	StopBudget             // the round or frame budget ran out
	StopFailed             // consecutive objective failures exhausted the failure budget
	StopError              // a planner error, or nothing is possible from here
)

// Result is the outcome of a run.
type Result struct {
	Stop      Stop
	Rounds    int
	Completed []Objective
	// Err is why the run STOPPED, and it is nil unless Stop is StopError or
	// StopFailed. A round that failed and was recovered from does not set
	// it: those live in Final.History, which is also where the planner reads
	// them. A caller can therefore treat a non-nil Err as "this run ended
	// badly" without having to re-derive that from Stop.
	Err   error
	Final Observation
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
	// run stopped before its first observation (a zero budget, a cancel
	// before the first frame, or a ROM the graph could not build), so a
	// nil pair means "never played", never "played and moved nothing".
	ProgressEarly *Progress
	ProgressFinal *Progress
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

// Budget bounds a run. MaxRounds and MaxFrames are both required; a zero
// budget is an error, not "unlimited". An unbounded loop against an
// emulator is how a run silently eats 45 minutes.
type Budget struct {
	MaxRounds int
	MaxFrames int
	// StuckAfter is how many consecutive objectives may leave the
	// observation unchanged before the run stops with StopStuck.
	// Zero means defaultStuckAfter.
	StuckAfter int
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

// Run drives observe -> plan -> execute until the planner is done or a
// budget is exhausted. A failed objective does not end the run: what was
// attempted and the error text are recorded in the observation history, the
// game is left where the failure left it, and the planner chooses again with
// the failure visible. That is the loop the run exists to produce: blocked
// at the town exit, hear why, go get what unblocks it. Stopping on every
// failure was right about SILENT failure — a planner reasoning from a state
// it thinks it reached is worse than stopping — and too strict for a precise
// one.
//
// The continuation is bounded three ways. A streak of failed objectives
// stops the run with StopFailed once it reaches MaxConsecutiveFailures
// (default 3); the same objective failing with the same error twice stops
// it immediately, because two identical failures outweigh any number of
// different ones; and MaxRounds and MaxFrames bound failure rounds like any
// other round. A blackout is exempt from the failure accounting: it is the
// game answering, not the planner failing — the party is healed and standing
// in a town. It is still recorded in history and in the failure tally —
// the objective was not reached — but it neither counts against
// MaxConsecutiveFailures nor repeats the last failure, because the respawn
// changed the world. A lost gym challenge is NOT exempt: the objective said
// "beat the gym leader" and the run lost, so it is a failure like any other
// (KindGym returns the loss as an error); the loss's own blackout is what
// makes the next round startable, and the round budget still bounds a
// planner that keeps challenging an unbeatable leader. A train retreat (ErrTrainRetreat) is exempt
// for the same reason in miniature: the session damaged the party, so a
// repeated attempt is a new one from a new state, and the planner's correct
// response — heal — is visible in the next observation's party HP; a planner
// that ignores it trips StuckAfter, because a retried train leaves the player
// where it started. The round budget still bounds a planner that keeps
// choosing doomed trains: each blackout cycle costs a round and levels the
// lead up, so the loop is slow progress, not a stall.
//
// Failure text reaches the planner as plain consequence, never as advice:
// Run records the error the skills produced ("step up blocked at (10,1)")
// and appends nothing to it. "you need a starter first" would be us solving
// the objective, which makes the whole measurement worthless.
//
// A failed objective leaves the game in a state the next one can start from:
// the skills settle the world before returning an error (Travel waits out
// battles and blackouts; a blocked step leaves the player standing), so the
// next round starts from RAM the game is done changing.
//
// Run also owns what Observe cannot decode from one snapshot: the recent
// dialogue (sampled from the emulator while objectives run) and the round
// history (what was attempted and how it turned out). Both are set on the
// observation before each plan, so a planner that just lost to Brock sees
// that in its prompt instead of choosing the same objective again.
//
// Run also owns the offered menu: it is rebuilt every round from the
// current observation and the run's accumulated knowledge (Offer), never
// built once at startup. A menu built before the first frame offers the
// same everything-forever question on every round, including places the
// player has no way of knowing exist.
func Run(m *emu.Emu, romData []byte, p Planner, budget Budget) Result {
	if budget.MaxRounds <= 0 || budget.MaxFrames <= 0 {
		return Result{
			Stop: StopError,
			Err:  errors.New("agent: Run: a zero budget is not unlimited; set MaxRounds and MaxFrames"),
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
	maxConsecFailures := budget.MaxConsecutiveFailures
	if maxConsecFailures <= 0 {
		maxConsecFailures = defaultMaxConsecutiveFailures
	}

	res := Result{Completed: []Objective{}}
	startFrame := m.FrameCount()
	tape := &dialogueTape{}
	m.OnSample(tape.sample)
	var history []RoundRecord
	last := Observe(m, romData)
	// The early progress sample: what the run started with, taken before
	// any objective ran. It is the baseline the finish sample is compared
	// against in the finish dump; a single end-of-run snapshot cannot
	// say whether the run moved at all.
	early := progressOf(last, known, 0)
	res.ProgressEarly = &early
	stuck := 0
	consecFailures := 0 // consecutive failed objectives; a success resets it
	lastFailObj, lastFailErr := "", ""
	retreatStreak := 0           // consecutive train retreats ending at the SAME level; capped like any other failure streak
	lastRetreatLevel := uint8(0) // the lead's level after the last retreat, 0 meaning none yet

	for round := 1; ; round++ {
		select {
		case <-budget.Cancel:
			return Result{Stop: StopBudget, Rounds: round - 1}
		default:
		}

		if round > budget.MaxRounds {
			res.Stop = StopBudget
			break
		}

		// Fold this round's observation into the run's knowledge before
		// building the menu: the map the player stands on is known, and a
		// place name the game spoke stays known even after it scrolls out
		// of the dialogue window. Then rebuild the menu: what is possible
		// depends on where the player is and what they already have.
		noteObservation(known, last)
		// Every map the last objective actually walked through, not just
		// the one it ended on: see dialogueTape.maps.
		for _, id := range tape.seenMaps() {
			known.SawMap(id)
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
		now := Offer(last, known)
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
		// What the budget has left, in the observation the planner reads.
		// MaxRounds is the last round that runs (the loop breaks on
		// round > MaxRounds), so this round counts itself.
		last.Round = round
		last.RoundsLeft = budget.MaxRounds - round + 1

		obj, err, retries := planWithRetries(budget.Log, round, p, last, now)
		res.ReplyRetries += retries
		// The break comes from the error, not from a stop-value check: the
		// zero value of Stop is StopUnset ("no reason set yet"), so the
		// checks below can ask "has a reason been set?" without ever
		// mistaking a finished planner for one.
		if errors.Is(err, ErrDone) {
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
			if err := ring.write(m, round, obj, known, intent, intentAge); err != nil {
				res.Stop = StopError
				res.Err = fmt.Errorf("agent: Run: checkpoint round %d: %w", round, err)
				break
			}
		}

		before := last
		if err := Execute(m, romData, obj); err != nil {
			// The failure is recorded where the planner reads it — the next
			// round's history — and the run continues. res.Rounds counts the
			// failed round: it ran.
			//
			// res.Err is NOT set here. Its documented meaning is why the run
			// STOPPED, and a failed objective the run recovers from is not
			// that: it is a round that went badly in a run that carried on.
			// Setting it on every failure left the last one showing on a run
			// that finished fine — cmd/pokepilot printed "error: ..." after a
			// healthy run, and the farm filed the same text as the run's
			// failure detail, so every recovered blackout was being counted
			// as a failed run in the wall's triage. It is set below, only
			// where the failure actually ends the run.
			res.Rounds = round
			last = observeAfter(m, romData, budget.Log)
			outcome := "failed: " + err.Error()
			blackedOut := errors.Is(err, skill.ErrBlackedOut)
			retreated := errors.Is(err, skill.ErrTrainRetreat)
			if blackedOut {
				// The blackout bit clears on the respawn map entry, before
				// this Observe; carry the fact for the round that follows the
				// loss so the planner sees a wiped party, not just a healed
				// one.
				last.BlackedOut = true
				// What the loss actually cost, in the one place the planner
				// re-reads every round. The respawn is wherever the party was
				// last healed at a Center — PALLET_TOWN until it uses one —
				// and the money is halved on the way there. Without this the
				// history says "blacked out" and the planner sees a healed
				// party in a town, which reads like a free reset.
				outcome += fmt.Sprintf(" (respawned in %s, money %d -> %d)",
					last.RespawnPlace, before.Money, last.Money)
			}
			known.Failed(obj, err)
			history = appendHistory(history, RoundRecord{Objective: obj.String(), Outcome: outcome})
			last.History = history
			last.RecentDialogue = tape.recent()
			logRound(budget.Log, round, obj, outcome, last)

			if blackedOut || retreated {
				// The blackout is recorded in history like any failure, but it
				// does not count against the failure budget and it breaks the
				// same-failure-twice chain: the respawn healed the party and
				// moved the player, so the world changed. A train retreat is
				// exempt for the same reason in miniature: the session spent
				// battles and damaged the party, so repeating the objective is
				// a new attempt from a new state, not an identical repetition —
				// and without the exemption, "train to 19" stopping hurt twice
				// in a row would read as the same failure twice and end the run,
				// trading a recoverable, costless stop for a dead one. The
				// planner's correct response (heal) is visible in the next
				// observation's party HP, and a planner that ignores it trips
				// StuckAfter instead: a retried train leaves the player standing
				// where it started. Only the frame budget still applies to this
				// round.
				//
				// The exemption still needs a ceiling: MEASURED 2026-08-30 on
				// the live GPU farm, a lead that retreats at the same level
				// every time (HP not meaningfully changing between attempts)
				// gets the identical objective and identical error 23 rounds
				// running, because retreated never trips StopFailed AND never
				// touches `stuck` (that counter only lives on the success
				// path).
				//
				// The ceiling counts STUCK-ness, not raw retries: a session
				// that keeps ending at a higher level each attempt IS
				// progress (the level-up's max-HP bump is exactly why two
				// retreats can end at different levels with nothing else
				// changing) and must not be capped just for repeating the
				// objective — only ended-level-unchanged streaks trip it.
				// The requested level in obj.String() is deliberately NOT
				// part of the comparison: a model that varies the number
				// asked for (10, then 11, then 11 again) while the lead's
				// actual level stays exactly as stuck is still the
				// identical problem repeating — MEASURED live, same
				// session, same run.
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
					res.Stop, res.Err = StopFailed, err
				}
				lastFailObj, lastFailErr = "", ""
				if m.FrameCount()-startFrame >= uint64(budget.MaxFrames) {
					res.Stop = StopBudget
				}
				if res.Stop != StopUnset {
					break
				}
				continue
			}
			retreatStreak, lastRetreatLevel = 0, 0

			consecFailures++
			// res.Stop is still StopUnset unless one of these sets it.
			switch {
			case obj.String() == lastFailObj && err.Error() == lastFailErr:
				// The same objective failing the same way twice in a row:
				// the world is not going to change on its own, and repeating
				// the identical attempt will not teach the planner anything.
				res.Stop, res.Err = StopFailed, err
			case consecFailures >= maxConsecFailures:
				// Different failures, but every objective the planner picks
				// dies: it is not reading the consequences.
				res.Stop, res.Err = StopFailed, err
			case m.FrameCount()-startFrame >= uint64(budget.MaxFrames):
				// The frame budget ran out on a round that happened to fail.
				// The budget is the reason, not the failure, so Err stays
				// nil and StopBudget carries the meaning.
				res.Stop = StopBudget
			}
			if res.Stop != StopUnset {
				break
			}
			lastFailObj, lastFailErr = obj.String(), err.Error()
			continue
		}
		last = observeAfter(m, romData, budget.Log)
		res.Rounds = round
		res.Completed = append(res.Completed, obj)
		known.Done(obj)
		if obj.Kind == KindTalk {
			known.TalkedTo(before.Map, obj.X, obj.Y)
		}
		consecFailures = 0
		lastFailObj, lastFailErr = "", ""
		retreatStreak, lastRetreatLevel = 0, 0
		history = appendHistory(history, RoundRecord{Objective: obj.String(), Outcome: "done"})
		last.History = history
		last.RecentDialogue = tape.recent()
		logRound(budget.Log, round, obj, "done", last)

		if sameProgress(before, last) {
			stuck++
		} else {
			stuck = 0
		}
		if stuck >= stuckAfter {
			res.Stop = StopStuck
			break
		}
		if m.FrameCount()-startFrame >= uint64(budget.MaxFrames) {
			res.Stop = StopBudget
			break
		}
	}

	res.Final = last
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
func (c *checkpointRing) write(m *emu.Emu, round int, obj Objective, k *Knowledge, intent string, intentAge int) error {
	b, err := m.SaveState()
	if err != nil {
		return fmt.Errorf("SaveState: %w", err)
	}
	path := filepath.Join(c.dir, fmt.Sprintf("round-%03d-frame-%010d-%s.state",
		round, m.FrameCount(), checkpointSlug(obj)))
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := writeMemoryFile(path, k, intent, intentAge); err != nil {
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
func checkpointSlug(o Objective) string {
	s := o.String()
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
