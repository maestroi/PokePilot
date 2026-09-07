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
	// Err is why the run STOPPED, and it is nil unless Stop is StopError or
	// StopFailed. Recovered objective failures live in Final.History.
	Err   error
	Final Observation
	// GoalStatus is the final authoritative status for an opted-in
	// deterministic goal. It is nil for free-text/prompt-only runs.
	GoalStatus *GoalStatus
	// ReplyRetries counts structurally rejected planner replies that were
	// re-asked in a different form.
	ReplyRetries int
	// Model usage summed across every call, including retries. Zero means
	// the planner did not report usage.
	PromptTokens     int
	CompletionTokens int
	// ProgressEarly and ProgressFinal bracket the run's observable progress.
	// Both are nil when the run stopped before its first observation.
	ProgressEarly *Progress
	ProgressFinal *Progress
}

// Progress is one snapshot of how far a run has gotten.
type Progress struct {
	Round   int
	Badges  int
	Events  int
	Maps    int
	Map     uint8
	MapName string
}

// MaxReplyRetries is the initial planner ask plus bounded re-asks.
const MaxReplyRetries = 3

// RetryTemperature is the sampling temperature used for wrong-shaped replies.
const RetryTemperature = 0.3

// Retry describes how one planner re-ask differs from the ask it repeats.
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

// FeedbackPlanner is a planner that can be re-asked about the same round.
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

// Budget bounds a run.
type Budget struct {
	MaxRounds int
	MaxFrames int
	// Goal optionally configures the run-owned completion contract. Known
	// presets and structured forms are evaluated deterministically; arbitrary
	// free text remains prompt-only. When empty, a RunGoalProvider planner may
	// supply the raw goal already used for its prompt.
	Goal                   string
	StuckAfter             int
	StagnationAfter        int
	MaxConsecutiveFailures int
	Log                    io.Writer
	CheckpointDir          string
	CheckpointKeep         int
	ResumeFrom             string
	Cancel                 <-chan struct{}
}

type dialogueTape struct {
	mu      sync.Mutex
	lines   []string
	last    string
	pending string
	stable  bool
	maps    map[uint8]bool
}

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

func (d *dialogueTape) noteMap(id uint8) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.maps == nil {
		d.maps = map[uint8]bool{}
	}
	d.maps[id] = true
}

func (d *dialogueTape) observeText(text string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch {
	case text == "":
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
			d.lines = append(d.lines, text)
		case strings.HasPrefix(text, d.last):
			d.lines[len(d.lines)-1] = text
		default:
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

func (d *dialogueTape) recent() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, len(d.lines))
	copy(out, d.lines)
	return out
}

const roundRecoveryBudget = 600

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

func appendHistory(h []RoundRecord, r RoundRecord) []RoundRecord {
	out := make([]RoundRecord, 0, len(h)+1)
	if len(h) >= historyCap {
		out = append(out, h[len(h)-historyCap+1:]...)
	} else {
		out = append(out, h...)
	}
	return append(out, r)
}

// Run drives observe -> plan -> execute until the run-owned goal is done,
// the prompt-only planner is done, the world is stuck, or a guardrail fires.
func Run(m *emu.Emu, romData []byte, p Planner, budget Budget) Result {
	if budget.MaxRounds < 0 || budget.MaxFrames <= 0 {
		return Result{
			Stop: StopError,
			Err:  errors.New("agent: Run: MaxRounds cannot be negative and MaxFrames must be positive"),
		}
	}
	select {
	case <-budget.Cancel:
		return Result{Stop: StopBudget, Rounds: 0}
	default:
	}

	runGoal, deterministicGoal, err := resolveRunGoal(p, budget.Goal)
	if err != nil {
		return Result{Stop: StopError, Err: err}
	}

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
	intent, intentAge := "", 0
	if budget.ResumeFrom != "" {
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
	stagnationAfter := budget.StagnationAfter
	if stagnationAfter <= 0 {
		stagnationAfter = defaultStagnationAfter
	}
	maxConsecFailures := budget.MaxConsecutiveFailures
	if maxConsecFailures <= 0 {
		maxConsecFailures = defaultMaxConsecutiveFailures
	}

	res := Result{Completed: []Objective{}}
	startFrame := m.FrameCount()
	tape := &dialogueTape{}
	m.AlsoSample(tape.sample)
	var history []RoundRecord
	last := Observe(m, romData)
	early := progressOf(last, known, 0)
	res.ProgressEarly = &early
	majorProgress := majorProgressMarkOf(last, known)
	lastMajorProgressRound := 0
	stuck := 0
	consecFailures := 0
	lastFailObj, lastFailErr := "", ""
	retreatStreak := 0
	lastRetreatLevel := uint8(0)

	for round := 1; ; round++ {
		select {
		case <-budget.Cancel:
			return Result{Stop: StopBudget, Rounds: round - 1}
		default:
		}

		// Deterministic completion is checked at the settled round boundary,
		// before round/watchdog classification or another planner/model call.
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

		noteObservation(known, last)
		for _, id := range tape.seenMaps() {
			known.SawMap(id)
		}

		currentMajorProgress := majorProgressMarkOf(last, known)
		if majorProgress.absorb(currentMajorProgress) {
			lastMajorProgressRound = round - 1
			if budget.Log != nil && round > 1 {
				fmt.Fprintf(budget.Log, "round %d: major progress -> %s\n", round-1, majorProgress)
			}
		}
		completedRounds := round - 1
		stagnantRounds := completedRounds - lastMajorProgressRound

		if reqs := known.Requirements; len(reqs) > 0 {
			last.Requirements = append([]Requirement{}, reqs...)
		}
		last.Failures = known.FailureList()
		now := offerWithTMHM(m, romData, last, known)
		if len(now) == 0 {
			res.Stop = StopError
			res.Err = errors.New("agent: Run: nothing is possible from here")
			break
		}

		last.Intent = intent
		last.IntentAge = intentAge
		last.Round = round
		last.RoundsLeft = roundsLeft(round, budget.MaxRounds)

		obj, err, retries := planWithRetries(budget.Log, round, p, last, now)
		res.ReplyRetries += retries
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
		if stagnantRounds >= stagnationAfter {
			res.Stop = StopStuck
			if budget.Log != nil {
				fmt.Fprintf(budget.Log, "stagnation watchdog: %d rounds without major progress; high-water mark: %s\n",
					stagnantRounds, majorProgress)
			}
			break
		}

		switch {
		case obj.Intent != "" && obj.Intent != intent:
			intent, intentAge = obj.Intent, 0
		case intent != "":
			intentAge++
		}

		if ring != nil {
			if err := ring.write(m, round, obj, known, intent, intentAge); err != nil {
				res.Stop = StopError
				res.Err = fmt.Errorf("agent: Run: checkpoint round %d: %w", round, err)
				break
			}
		}

		before := last
		if err := executeObjective(m, romData, obj); err != nil {
			res.Rounds = round
			last = observeAfter(m, romData, budget.Log)
			outcome := "failed: " + err.Error()
			blackedOut := errors.Is(err, skill.ErrBlackedOut)
			retreated := errors.Is(err, skill.ErrTrainRetreat)
			trainProgressed := errors.Is(err, skill.ErrTrainProgress)
			if blackedOut {
				last.BlackedOut = true
				outcome += fmt.Sprintf(" (respawned in %s, money %d -> %d)",
					last.RespawnPlace, before.Money, last.Money)
			}
			known.Failed(obj, err)
			if trainProgressed {
				known.clearGymLossFailures()
			}
			history = appendHistory(history, RoundRecord{Objective: obj.String(), Outcome: outcome})
			last.History = history
			last.RecentDialogue = tape.recent()
			logRound(budget.Log, round, obj, outcome, last)

			// A skill can return an error after the game already committed the
			// story state. The observable goal wins over failure classification.
			if deterministicGoal {
				status := evaluateRunGoal(p, runGoal, last, round, budget.MaxRounds, intent, intentAge)
				res.GoalStatus = &status
				if status.Complete {
					res.Stop = StopDone
					break
				}
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
			switch {
			case obj.String() == lastFailObj && err.Error() == lastFailErr:
				res.Stop, res.Err = StopFailed, err
			case consecFailures >= maxConsecFailures:
				res.Stop, res.Err = StopFailed, err
			case m.FrameCount()-startFrame >= uint64(budget.MaxFrames):
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

		// Check the settled result before short-stuck or frame-budget guards.
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
	if deterministicGoal {
		status := evaluateRunGoal(p, runGoal, last, res.Rounds, budget.MaxRounds, intent, intentAge)
		res.GoalStatus = &status
	}
	final := progressOf(last, known, res.Rounds)
	res.ProgressFinal = &final
	if up, ok := p.(UsagePlanner); ok {
		res.PromptTokens, res.CompletionTokens = up.Usage()
	}
	return res
}

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

func noteObservation(k *Knowledge, obs Observation) {
	k.SawMap(obs.Map)
	k.SawDialogue(obs.RecentDialogue, obs.MapName, obs.X, obs.Y)
}

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

type checkpointRing struct {
	dir  string
	keep int
}

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

func checkpointSlug(o Objective) string { return slugify(o.String()) }

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

func logRound(w io.Writer, round int, o Objective, outcome string, after Observation) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "round %d: %s -> %s, map %02x at (%d,%d)\n", round, o, outcome, after.Map, after.X, after.Y)
}
