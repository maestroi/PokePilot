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
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
)

// Stop says why a run ended.
type Stop uint8

const (
	StopDone   Stop = iota // the planner reported ErrDone
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
	Err       error // set when Stop is StopError or StopFailed
	Final     Observation
	// ReplyRetries counts how many times the planner answered in the wrong
	// shape and the same round was re-asked with the rejection quoted back.
	// It is the diagnostic that separates a loop problem from a capacity
	// problem: a run full of them answered but could not answer in shape,
	// while zero means every reply the model gave was structurally fine.
	ReplyRetries int
}

// MaxReplyRetries is how many times the planner may be asked for one
// round's choice: the initial ask plus re-asks that quote the rejection
// back. A malformed reply says nothing about the world — only that the
// model answered in the wrong shape — so it is retryable, but a model that
// cannot answer in shape three times running is a real finding, not a
// transient, and the round stops with StopError.
const MaxReplyRetries = 3

// FeedbackPlanner is a planner that can be re-asked about the same round
// with the text of its own rejection quoted back as feedback. LLMPlanner
// implements it; the scripted planners do not, and a plain Planner's error
// keeps stopping the run exactly as before.
type FeedbackPlanner interface {
	NextFeedback(obs Observation, offered []Objective, feedback string) (Objective, error)
}

// planWithRetries asks the planner for this round's objective and, when it
// rejects its own reply (a planner error that is not ErrDone), re-asks the
// SAME round with the rejection quoted back. The observation does not
// change; only the rejection feedback is added. It returns the objective
// (meaningful only when err is nil), the planner's error — ErrDone passes
// through untouched; any other non-nil error means MaxReplyRetries asks
// have all been rejected, or a planner that cannot take feedback errored at
// all — and n, how many re-asks happened, so the caller can count them in
// the result. Run classifies the error into a Stop reason: StopDone is the
// ZERO value of Stop, so it must never be signalled through a "stop != 0"
// check.
func planWithRetries(log io.Writer, round int, p Planner, obs Observation, offered []Objective) (Objective, error, int) {
	obj, err := p.Next(obs, offered)
	fp, canFeedback := p.(FeedbackPlanner)
	retries := 0
	for err != nil && !errors.Is(err, ErrDone) && canFeedback && retries < MaxReplyRetries-1 {
		retries++
		if log != nil {
			fmt.Fprintf(log, "round %d: reply rejected (ask %d of %d): %v\n",
				round, retries+1, MaxReplyRetries, err)
		}
		obj, err = fp.NextFeedback(obs, offered, err.Error())
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
		if d.last != "" && strings.HasPrefix(text, d.last) && len(d.lines) > 0 {
			d.lines[len(d.lines)-1] = text
		} else {
			d.lines = append(d.lines, text)
		}
		d.last = text
		if len(d.lines) > dialogueCap {
			d.lines = d.lines[len(d.lines)-dialogueCap:]
		}
		return true
	}
	return false
}

// recent returns a copy of the settled lines, oldest first.
func (d *dialogueTape) recent() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, len(d.lines))
	copy(out, d.lines)
	return out
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
// other round.
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
	stuck := 0
	consecFailures := 0 // consecutive failed objectives; a success resets it
	lastFailObj, lastFailErr := "", ""

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
		now := Offer(last, known)
		if len(now) == 0 {
			res.Stop = StopError
			res.Err = errors.New("agent: Run: nothing is possible from here")
			break
		}

		// A rejected reply is a different kind of event from a failed
		// objective: it says nothing about the world, only that the model
		// answered in the wrong shape, so the same round is re-asked with
		// the rejection quoted back (planWithRetries) instead of stopping.
		// ErrDone and other errors are classified here, not by a stop-value
		// check: StopDone is Stop(0), the zero value, so "stop != 0" would
		// read a finished planner as "keep going" and execute an empty
		// objective.
		obj, err, retries := planWithRetries(budget.Log, round, p, last, now)
		res.ReplyRetries += retries
		// StopDone is Stop(0), the zero value, so it cannot be signalled
		// through a "res.Stop != 0" check: the break must come from the
		// error itself, or a finished planner would read as "keep going"
		// and Execute would run on an empty objective.
		if errors.Is(err, ErrDone) {
			res.Stop = StopDone
			break
		}
		if err != nil {
			res.Stop = StopError
			res.Err = err
			break
		}

		// The checkpoint is taken BEFORE Execute: it is the exact state the
		// decision was made in, and the resume point if this objective is
		// where the run went wrong.
		if ring != nil {
			if err := ring.write(m, round, obj); err != nil {
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
			res.Rounds = round
			res.Err = err
			history = appendHistory(history, RoundRecord{Objective: obj.String(), Outcome: "failed: " + err.Error()})
			last = Observe(m, romData)
			if errors.Is(err, skill.ErrBlackedOut) {
				// The blackout bit clears on the respawn map entry, before
				// this Observe; carry the fact for the round that follows the
				// loss so the planner sees a wiped party, not just a healed
				// one.
				last.BlackedOut = true
			}
			last.History = history
			last.RecentDialogue = tape.recent()
			logRound(budget.Log, round, obj, last)

			consecFailures++
			// res.Stop is still the zero value unless one of these sets it.
			switch {
			case obj.String() == lastFailObj && err.Error() == lastFailErr:
				// The same objective failing the same way twice in a row:
				// the world is not going to change on its own, and repeating
				// the identical attempt will not teach the planner anything.
				res.Stop = StopFailed
			case consecFailures >= maxConsecFailures:
				// Different failures, but every objective the planner picks
				// dies: it is not reading the consequences.
				res.Stop = StopFailed
			case m.FrameCount()-startFrame >= uint64(budget.MaxFrames):
				res.Stop = StopBudget
			}
			if res.Stop != 0 {
				break
			}
			lastFailObj, lastFailErr = obj.String(), err.Error()
			continue
		}
		last = Observe(m, romData)
		res.Rounds = round
		res.Completed = append(res.Completed, obj)
		known.Done(obj)
		consecFailures = 0
		lastFailObj, lastFailErr = "", ""
		history = appendHistory(history, RoundRecord{Objective: obj.String(), Outcome: "done"})
		last.History = history
		last.RecentDialogue = tape.recent()
		logRound(budget.Log, round, obj, last)

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
	return res
}

// noteObservation folds one observation into the run's knowledge: the map
// the player is standing on and any place names in its recent dialogue.
func noteObservation(k *Knowledge, obs Observation) {
	k.SawMap(obs.Map)
	k.SawDialogue(obs.RecentDialogue)
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
// on Run's goroutine before Execute, where m is not being stepped.
func (c *checkpointRing) write(m *emu.Emu, round int, obj Objective) error {
	b, err := m.SaveState()
	if err != nil {
		return fmt.Errorf("SaveState: %w", err)
	}
	path := filepath.Join(c.dir, fmt.Sprintf("round-%03d-frame-%010d-%s.state",
		round, m.FrameCount(), checkpointSlug(obj)))
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return c.evict()
}

// evict keeps only the newest keep states in the ring's directory.
func (c *checkpointRing) evict() error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", c.dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, en := range entries {
		if strings.HasSuffix(en.Name(), ".state") {
			names = append(names, en.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names[:max(0, len(names)-c.keep)] {
		if err := os.Remove(filepath.Join(c.dir, n)); err != nil && !os.IsNotExist(err) {
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
func logRound(w io.Writer, round int, o Objective, after Observation) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "round %d: %s -> map %02x at (%d,%d)\n", round, o, after.Map, after.X, after.Y)
}
