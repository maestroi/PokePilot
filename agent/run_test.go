package agent_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// testBudget is a generous budget: the assertions under test are about the
// stop reason, not about running out of headroom.
func testBudget() agent.Budget {
	return agent.Budget{MaxRounds: 10, MaxFrames: 10_000_000}
}

// TestRunDone runs starter -> walk to Pallet Town and expects the planner to
// run out of objectives: StopDone, two rounds, and the player on map 0x00.
func TestRunDone(t *testing.T) {
	e := loadFixture(t)

	var log bytes.Buffer
	p := agent.NewScriptedPlanner(
		agent.Objective{Kind: agent.KindStarter},
		agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"},
	)
	b := testBudget()
	b.Log = &log
	res := agent.Run(e, e.ROM(), p, b)

	t.Logf("run log:\n%s", log.String())

	if res.Stop != agent.StopDone {
		t.Fatalf("Stop = %d, want StopDone (planner exhausted is success)", res.Stop)
	}
	if res.Rounds != 2 {
		t.Fatalf("Rounds = %d, want 2", res.Rounds)
	}
	if len(res.Completed) != 2 {
		t.Fatalf("len(Completed) = %d, want 2: %v", len(res.Completed), res.Completed)
	}
	if res.Final.Map != 0x00 {
		t.Fatalf("Final.Map = %#04x, want 0x00 (pallet town) at (%d,%d)",
			res.Final.Map, res.Final.X, res.Final.Y)
	}
	if got := strings.Count(log.String(), "round "); got != 2 {
		t.Fatalf("log has %d round lines, want 2:\n%s", got, log.String())
	}
}

// TestRunRoundBudget gives a two-objective script a one-round budget and
// expects the run to stop with StopBudget after the first round.
func TestRunRoundBudget(t *testing.T) {
	e := loadFixture(t)

	p := agent.NewScriptedPlanner(
		agent.Objective{Kind: agent.KindStarter},
		agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"},
	)
	res := agent.Run(e, e.ROM(), p, agent.Budget{MaxRounds: 1, MaxFrames: 10_000_000})

	if res.Stop != agent.StopBudget {
		t.Fatalf("Stop = %d, want StopBudget", res.Stop)
	}
	if res.Rounds != 1 {
		t.Fatalf("Rounds = %d, want 1", res.Rounds)
	}
	if len(res.Completed) != 1 {
		t.Fatalf("len(Completed) = %d, want 1: %v", len(res.Completed), res.Completed)
	}
}

// errPlanner fails in Next itself: a failure of the planner, which no
// objective-failure budget absorbs — the run stops with StopError on the
// first round.
type errPlanner struct{ err error }

func (p *errPlanner) Next(agent.Observation, []agent.Objective) (agent.Objective, error) {
	return agent.Objective{}, p.err
}

// TestRunError feeds the planner a planner-level error and expects the run
// to stop with StopError, keeping it in Result.Err.
func TestRunError(t *testing.T) {
	e := loadFixture(t)

	const msg = "planner exploded"
	p := &errPlanner{err: errors.New(msg)}
	res := agent.Run(e, e.ROM(), p, testBudget())

	if res.Stop != agent.StopError {
		t.Fatalf("Stop = %d, want StopError", res.Stop)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), msg) {
		t.Fatalf("Err = %v, want the planner's error kept", res.Err)
	}
	if res.Rounds != 0 {
		t.Fatalf("Rounds = %d, want 0 (no objective ran)", res.Rounds)
	}
	if len(res.Completed) != 0 {
		t.Fatalf("Completed = %v, want empty", res.Completed)
	}
}

// capturePlanner records every observation it is handed, so a test can
// assert what the planner actually saw — not just where the run ended.
type capturePlanner struct {
	objs []agent.Objective
	next int
	seen []agent.Observation
}

func (p *capturePlanner) Next(obs agent.Observation, offered []agent.Objective) (agent.Objective, error) {
	p.seen = append(p.seen, obs)
	if p.next >= len(p.objs) {
		return agent.Objective{}, agent.ErrDone
	}
	o := p.objs[p.next]
	p.next++
	return o, nil
}

// TestRunObservationCarriesHistoryAndMoves is the point of the run memory:
// the observation a planner sees after round one carries the lead's moves,
// what the game said during the story, and how round one turned out. A
// stateless planner re-choosing the same objective forever is what this
// exists to end.
func TestRunObservationCarriesHistoryAndMoves(t *testing.T) {
	e := loadFixture(t)
	// The dialogue tape samples on the emulator's sample hook, which runs
	// with the capture loop: enable Watch exactly as cmd/pokepilot does.
	if _, err := e.Watch("127.0.0.1:0", 4); err != nil {
		t.Fatalf("Watch: %v", err)
	}

	p := &capturePlanner{objs: []agent.Objective{
		{Kind: agent.KindStarter},
		{Kind: agent.KindGoTo, Place: "pallet town"},
	}}
	res := agent.Run(e, e.ROM(), p, testBudget())

	if res.Stop != agent.StopDone {
		t.Fatalf("Stop = %d, want StopDone", res.Stop)
	}
	// Next is called with the initial observation and once per completed
	// round: three observations for a two-round run.
	if len(p.seen) != 3 {
		t.Fatalf("len(seen) = %d, want 3 (initial + one per round)", len(p.seen))
	}
	second := p.seen[1]

	// History: the starter round is over and it turned out done.
	if len(second.History) != 1 {
		t.Fatalf("History = %+v, want exactly the completed starter round", second.History)
	}
	if second.History[0].Objective != "take the charmander starter" || second.History[0].Outcome != "done" {
		t.Errorf("History[0] = %+v, want {take the charmander starter done}", second.History[0])
	}

	// Moves: the lead exists after the starter and its moves are decoded
	// (Charmander: SCRATCH + GROWL).
	if len(second.LeadMoves) != 2 || second.LeadMoves[0] != (agent.Move{Power: 40, Type: "normal"}) {
		t.Errorf("LeadMoves = %+v, want SCRATCH (power 40 normal) first", second.LeadMoves)
	}

	// Dialogue: the story talks (Oak's cutscene, the rival's challenge),
	// and boxes that closed inside Execute must still reach the prompt.
	if len(second.RecentDialogue) == 0 {
		t.Fatalf("RecentDialogue is empty after a story full of text boxes")
	}

	// The third observation carries both rounds, oldest first.
	third := p.seen[2]
	if len(third.History) != 2 || third.History[0].Objective != "take the charmander starter" {
		t.Errorf("third History = %+v, want both rounds, oldest first", third.History)
	}
}

// TestRunFailedObjectiveFeedsNextRound is the loop this run exists to
// produce: an objective fails, the failure lands in the observation history,
// and the planner chooses again — with the game still in a state the next
// objective can start from. Heal at a fresh boot fails before any input is
// sent (no party to heal), so the round after it starts from the exact same
// world the failure left.
func TestRunFailedObjectiveFeedsNextRound(t *testing.T) {
	e := loadFixture(t)

	p := &capturePlanner{objs: []agent.Objective{
		{Kind: agent.KindHeal},
		{Kind: agent.KindStarter},
	}}
	res := agent.Run(e, e.ROM(), p, testBudget())

	if res.Stop != agent.StopDone {
		t.Fatalf("Stop = %d, want StopDone (a failed objective does not end the run)", res.Stop)
	}
	if res.Rounds != 2 {
		t.Fatalf("Rounds = %d, want 2", res.Rounds)
	}
	if len(res.Completed) != 1 || res.Completed[0].Kind != agent.KindStarter {
		t.Fatalf("Completed = %v, want the starter (the failed heal is not completed)", res.Completed)
	}

	// The second observation — what the planner actually saw when it chose
	// the starter — carries the failure: objective and error text.
	if len(p.seen) != 3 {
		t.Fatalf("len(seen) = %d, want 3 (initial + one per round)", len(p.seen))
	}
	second := p.seen[1]
	if len(second.History) != 1 {
		t.Fatalf("History = %+v, want the failed heal round", second.History)
	}
	h := second.History[0]
	if h.Objective != "heal the party" || !strings.HasPrefix(h.Outcome, "failed: ") {
		t.Fatalf("History[0] = %+v, want {heal the party failed: ...}", h)
	}
	// Plain consequence, never advice: the outcome is the error the skill
	// produced ("no party to heal"), with nothing appended telling the
	// planner what to do about it.
	if !strings.Contains(h.Outcome, "no party to heal") {
		t.Errorf("History[0].Outcome = %q, want the skill's failure text", h.Outcome)
	}

	// The failed objective left the game in a state the next one can start
	// from: a controllable overworld, and the starter ran to completion.
	if !second.Controllable {
		t.Errorf("not controllable after the failed objective: %+v", second)
	}
	if res.Final.PartyCount != 1 {
		t.Errorf("Final.PartyCount = %d, want 1 (the starter started from the failure's aftermath)", res.Final.PartyCount)
	}
}

// TestRunRepeatedFailureStops: the same doomed objective twice in a row is
// terminal. A planner that offers it every round no matter what the history
// says stops with StopFailed on the second identical failure — before any
// counting budget, which is the point: two identical failures are a stronger
// signal than ten different ones.
func TestRunRepeatedFailureStops(t *testing.T) {
	e := loadFixture(t)

	objs := make([]agent.Objective, 10)
	for i := range objs {
		objs[i] = agent.Objective{Kind: agent.KindHeal}
	}
	b := testBudget()
	b.MaxRounds = 50
	res := agent.Run(e, e.ROM(), agent.NewScriptedPlanner(objs...), b)

	if res.Stop != agent.StopFailed {
		t.Fatalf("Stop = %d after %d rounds, want StopFailed", res.Stop, res.Rounds)
	}
	if res.Rounds != 2 {
		t.Fatalf("Rounds = %d, want 2 (the same failure twice is decisive)", res.Rounds)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "no party to heal") {
		t.Fatalf("Err = %v, want the objective's failure kept", res.Err)
	}
	if len(res.Completed) != 0 {
		t.Fatalf("Completed = %v, want empty", res.Completed)
	}
	// Both failures are in the history the planner would have seen on a
	// third round.
	if len(res.Final.History) != 2 {
		t.Fatalf("Final.History = %+v, want both failed rounds", res.Final.History)
	}
	for i, h := range res.Final.History {
		if h.Objective != "heal the party" || !strings.HasPrefix(h.Outcome, "failed: ") {
			t.Errorf("History[%d] = %+v, want {heal the party failed: ...}", i, h)
		}
	}
}

// TestRunConsecutiveFailureBudget: three DIFFERENT failures in a row exhaust
// the default consecutive-failure budget even though no two of them are
// alike — the same-failure-twice rule never fires, so this is what bounds a
// planner that keeps trying new things and keeps dying.
func TestRunConsecutiveFailureBudget(t *testing.T) {
	e := loadFixture(t)

	p := agent.NewScriptedPlanner(
		agent.Objective{Kind: agent.KindHeal},
		agent.Objective{Kind: agent.KindGoTo, Place: "atlantis"},
		agent.Objective{Kind: agent.KindGoTo, Place: "nowhere"},
	)
	res := agent.Run(e, e.ROM(), p, testBudget())

	if res.Stop != agent.StopFailed {
		t.Fatalf("Stop = %d after %d rounds, want StopFailed", res.Stop, res.Rounds)
	}
	if res.Rounds != 3 {
		t.Fatalf("Rounds = %d, want 3 (the default consecutive-failure budget)", res.Rounds)
	}
	if len(res.Final.History) != 3 {
		t.Fatalf("Final.History = %+v, want all three failed rounds", res.Final.History)
	}
}

// TestRunBlackoutDoesNotStopTheRun: a blackout is the game answering, not
// the planner failing — the party is healed and standing in a town, the same
// recoverable state a lost gym challenge leaves. It must be recorded in
// history (the objective was not reached) but it must NOT count against the
// consecutive-failure budget: under the old accounting this exact sequence
// stopped with StopFailed on the third round, which is what killed the live
// runs — a lone starter blackouts in about three training battles, and the
// model retried the doomed train until the budget ran out.
func TestRunBlackoutDoesNotStopTheRun(t *testing.T) {
	e := fixture.Load(t, "post_starter")

	// Setup: get the player onto Route 1, where Train can find grass.
	// fixture.Travel retries through blackouts, so the setup is bounded.
	dest, ok := skill.Place("route 1")
	if !ok {
		t.Fatal("Place(route 1) did not resolve")
	}
	if _, err := fixture.Travel(e, dest, skill.StatAwareMove(e.ROM()), 20); err != nil {
		t.Fatalf("setup: travel to route 1: %v", err)
	}

	p := &capturePlanner{objs: []agent.Objective{
		// A lone L6 lead cannot reach level 100 in 20 battles; the session
		// ends when cumulative damage blackouts the party, which is certain:
		// finite HP against a wild that keeps hitting.
		{Kind: agent.KindTrain, Level: 100},
		// The blackout lands the player in Pallet Town, which has no center:
		// a deterministic failure that DOES count against the budget.
		{Kind: agent.KindHeal},
		// A second different failure; two counted failures are under the
		// default budget of three, so the run must survive to the planner's
		// end instead of stopping with StopFailed.
		{Kind: agent.KindGoTo, Place: "atlantis"},
	}}
	res := agent.Run(e, e.ROM(), p, testBudget())

	if res.Stop != agent.StopDone {
		t.Fatalf("Stop = %d after %d rounds, want StopDone (the blackout is not a failure-budget event)", res.Stop, res.Rounds)
	}
	if res.Rounds != 3 {
		t.Fatalf("Rounds = %d, want 3 (the three failed rounds; the planner's end is not a round)", res.Rounds)
	}
	if len(res.Completed) != 0 {
		t.Fatalf("Completed = %v, want empty (none of the three objectives was reached)", res.Completed)
	}
	if len(p.seen) != 4 {
		t.Fatalf("len(seen) = %d, want 4", len(p.seen))
	}
	// The round after the blackout saw the carried fact and the failure text.
	if !p.seen[1].BlackedOut {
		t.Errorf("seen[1].BlackedOut = false, want true (the fact must carry across the respawn)")
	}
	if len(p.seen[1].History) != 1 || !strings.Contains(p.seen[1].History[0].Outcome, "blacked out") {
		t.Errorf("seen[1].History = %+v, want the blackout failure recorded", p.seen[1].History)
	}
	if len(res.Final.History) != 3 {
		t.Fatalf("Final.History = %+v, want all three failed rounds", res.Final.History)
	}
	for i, h := range res.Final.History {
		if !strings.HasPrefix(h.Outcome, "failed: ") {
			t.Errorf("Final.History[%d] = %+v, want a failed round", i, h)
		}
	}
}

// checkpointForRound finds the checkpoint file Run wrote for a round.
func checkpointForRound(t *testing.T, dir string, round int) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", dir, err)
	}
	prefix := fmt.Sprintf("round-%03d-", round)
	var names []string
	for _, en := range entries {
		names = append(names, en.Name())
		if strings.HasPrefix(en.Name(), prefix) {
			return filepath.Join(dir, en.Name())
		}
	}
	t.Fatalf("no checkpoint for round %d in %s (have %v)", round, dir, names)
	return ""
}

// restoreCheckpoint loads a checkpoint file into a fresh emulator and
// returns the decoded player map and position.
func restoreCheckpoint(t *testing.T, rom, path string) (uint8, int, int) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	e, err := emu.Open(rom)
	if err != nil {
		t.Fatalf("emu.Open: %v", err)
	}
	defer e.Close()
	if err := e.LoadState(b); err != nil {
		t.Fatalf("LoadState %s: %v", path, err)
	}
	var m state.Mem
	state.Snapshot(e, &m)
	gs := state.Decode(&m)
	return gs.Player.MapID, int(gs.Player.X), int(gs.Player.Y)
}

// TestRunCheckpointRoundTrips: with CheckpointDir set, Run writes a save
// state before every objective, and loading one back restores the exact map
// and position the run was in before that objective ran — the state the
// decision was made in. That is what makes a failed run inspectable instead
// of re-run from boot.
func TestRunCheckpointRoundTrips(t *testing.T) {
	rom := os.Getenv("POKEMON_RED_ROM")
	if rom == "" {
		t.Skip("POKEMON_RED_ROM not set; cannot load checkpoints back")
	}

	e := loadFixture(t)
	dir := t.TempDir()

	p := &capturePlanner{objs: []agent.Objective{
		{Kind: agent.KindStarter},
		{Kind: agent.KindGoTo, Place: "pallet town"},
	}}
	b := testBudget()
	b.CheckpointDir = dir
	res := agent.Run(e, e.ROM(), p, b)
	if res.Stop != agent.StopDone {
		t.Fatalf("Stop = %d, want StopDone", res.Stop)
	}

	// The round-1 checkpoint is the state before the starter ran: exactly
	// what the planner first saw.
	initial := p.seen[0]
	if gotMap, gotX, gotY := restoreCheckpoint(t, rom, checkpointForRound(t, dir, 1)); gotMap != initial.Map || gotX != int(initial.X) || gotY != int(initial.Y) {
		t.Errorf("round-1 checkpoint = map %#04x (%d,%d), want map %#04x (%d,%d)",
			gotMap, gotX, gotY, initial.Map, initial.X, initial.Y)
	}

	// The round-2 checkpoint is the state after the starter: exactly what
	// the planner saw when it chose the walk.
	second := p.seen[1]
	if gotMap, gotX, gotY := restoreCheckpoint(t, rom, checkpointForRound(t, dir, 2)); gotMap != second.Map || gotX != int(second.X) || gotY != int(second.Y) {
		t.Errorf("round-2 checkpoint = map %#04x (%d,%d), want map %#04x (%d,%d)",
			gotMap, gotX, gotY, second.Map, second.X, second.Y)
	}
}

// TestRunCheckpointRingIsBounded: the checkpoint directory holds a ring of
// the last CheckpointKeep states, not one file per round forever. A run that
// executes four rounds with keep=2 leaves exactly the last two checkpoints
// on disk — proof the bound evicts rather than grows.
func TestRunCheckpointRingIsBounded(t *testing.T) {
	e := loadFixture(t)
	dir := t.TempDir()

	// Twelve GoTo to a place reachable in one round: the first moves, the
	// repeats do not, and the run stops with StopStuck after four rounds —
	// four checkpoints written.
	objs := make([]agent.Objective, 12)
	for i := range objs {
		objs[i] = agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"}
	}
	b := testBudget()
	b.MaxRounds = 50
	b.CheckpointDir = dir
	b.CheckpointKeep = 2
	res := agent.Run(e, e.ROM(), agent.NewScriptedPlanner(objs...), b)
	if res.Stop != agent.StopStuck || res.Rounds != 4 {
		t.Fatalf("Stop = %d Rounds = %d, want StopStuck at round 4 (the test needs four rounds written)",
			res.Stop, res.Rounds)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", dir, err)
	}
	if len(entries) != 2 {
		var names []string
		for _, en := range entries {
			names = append(names, en.Name())
		}
		t.Fatalf("%d checkpoints on disk, want exactly 2 (the ring must evict): %v", len(entries), names)
	}
	if !strings.Contains(entries[0].Name(), "round-003") || !strings.Contains(entries[1].Name(), "round-004") {
		t.Fatalf("ring kept %v, want the newest two rounds (003 and 004)",
			[]string{entries[0].Name(), entries[1].Name()})
	}
}

// replyPlanner is a scripted planner that also implements FeedbackPlanner:
// it can reject its first Next with a malformed-reply error (the shape of
// an LLM planner's "argument does not apply" rejection) and then answer.
// It records every feedback string it is handed so a test can assert the
// rejection text actually reached the re-prompt, and counts every ask.
type replyPlanner struct {
	objs      []agent.Objective
	rejectErr error // returned by the next `rejects` asks, Next and NextFeedback alike
	rejects   int
	next      int
	asks      int
	feedback  []string
}

func (p *replyPlanner) take() (agent.Objective, error) {
	if p.rejectErr != nil && p.rejects > 0 {
		p.rejects--
		return agent.Objective{}, p.rejectErr
	}
	if p.next >= len(p.objs) {
		return agent.Objective{}, agent.ErrDone
	}
	o := p.objs[p.next]
	p.next++
	return o, nil
}

func (p *replyPlanner) Next(agent.Observation, []agent.Objective) (agent.Objective, error) {
	p.asks++
	return p.take()
}

func (p *replyPlanner) NextFeedback(obs agent.Observation, offered []agent.Objective, feedback string) (agent.Objective, error) {
	p.asks++
	p.feedback = append(p.feedback, feedback)
	return p.take()
}

// TestRunRejectedReplyRecovers is the measured baseline bug: 9 of 9 sweep
// runs died on "argument does not apply" rejections, and a scoreboard
// reading 0/9 from that measures a schema bug, not a model. A planner that
// answers in the wrong shape once must not end the run: the round is
// re-asked with the rejection quoted back, the valid reply executes, and
// the retry is counted in the result.
func TestRunRejectedReplyRecovers(t *testing.T) {
	e := loadFixture(t)

	const msg = "agent: level argument 12 does not apply to go to route 1"
	p := &replyPlanner{
		rejectErr: errors.New(msg),
		rejects:   1, // the first ask is rejected; the re-ask answers normally
		objs: []agent.Objective{
			{Kind: agent.KindStarter},
		},
	}
	res := agent.Run(e, e.ROM(), p, testBudget())

	if res.Stop != agent.StopDone {
		t.Fatalf("Stop = %d, want StopDone (a malformed reply does not end the run): Err = %v", res.Stop, res.Err)
	}
	if len(res.Completed) != 1 || res.Completed[0].Kind != agent.KindStarter {
		t.Fatalf("Completed = %v, want the starter after the recovered round", res.Completed)
	}
	if res.ReplyRetries != 1 {
		t.Fatalf("ReplyRetries = %d, want 1 (one rejected reply, one re-ask)", res.ReplyRetries)
	}
	if p.asks != 3 {
		t.Fatalf("asks = %d, want 3 (initial + one re-ask + the ErrDone ask)", p.asks)
	}
	if len(p.feedback) != 1 || !strings.Contains(p.feedback[0], msg) {
		t.Fatalf("feedback = %v, want the rejection text quoted back into the re-prompt", p.feedback)
	}
}

// TestRunRejectedReplyExhaustsRetries is the other side of the bound: a
// planner that answers in the wrong shape every time stops with StopError
// after exactly MaxReplyRetries asks — not on the first rejection, and not
// any later. A retry loop with no proven bound is how a run burns an hour
// on one round.
func TestRunRejectedReplyExhaustsRetries(t *testing.T) {
	e := loadFixture(t)

	const msg = "agent: level argument 12 does not apply to go to route 1"
	// More rejections than any retry budget: the run must stop at exactly
	// MaxReplyRetries asks, not when the planner runs out of bad replies.
	p := &replyPlanner{rejectErr: errors.New(msg), rejects: 10}
	res := agent.Run(e, e.ROM(), p, testBudget())

	if res.Stop != agent.StopError {
		t.Fatalf("Stop = %d, want StopError (a model that cannot answer in shape is a finding)", res.Stop)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), msg) {
		t.Fatalf("Err = %v, want the last rejection kept", res.Err)
	}
	if p.asks != agent.MaxReplyRetries {
		t.Fatalf("asks = %d, want exactly MaxReplyRetries (%d): not on the first, not any later",
			p.asks, agent.MaxReplyRetries)
	}
	if res.ReplyRetries != agent.MaxReplyRetries-1 {
		t.Fatalf("ReplyRetries = %d, want %d (re-asks after the initial one)",
			res.ReplyRetries, agent.MaxReplyRetries-1)
	}
	if len(p.feedback) != agent.MaxReplyRetries-1 {
		t.Fatalf("feedback = %v, want %d re-prompts, each carrying the rejection",
			p.feedback, agent.MaxReplyRetries-1)
	}
	for i, fb := range p.feedback {
		if !strings.Contains(fb, msg) {
			t.Errorf("feedback[%d] = %q, want the rejection text quoted back", i, fb)
		}
	}
	if res.Rounds != 0 || len(res.Completed) != 0 {
		t.Fatalf("Rounds = %d Completed = %v, want 0/empty (no objective ran)", res.Rounds, res.Completed)
	}
}

// TestRunStuck repeats a GoTo to the place the player is already standing
// on. The first walk moves the player to Pallet Town; every repeat changes
// nothing, so the run must stop with StopStuck instead of looping.
func TestRunStuck(t *testing.T) {
	e := loadFixture(t)

	objs := make([]agent.Objective, 0, 12)
	for i := 0; i < 12; i++ {
		objs = append(objs, agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"})
	}
	p := agent.NewScriptedPlanner(objs...)
	b := testBudget()
	b.MaxRounds = 50
	res := agent.Run(e, e.ROM(), p, b)

	if res.Stop != agent.StopStuck {
		t.Fatalf("Stop = %d after %d rounds, want StopStuck (Completed: %v)",
			res.Stop, res.Rounds, res.Completed)
	}
	// Round 1 moves the player from Red's bedroom to Pallet Town; the next
	// defaultStuckAfter (3) unchanged repeats stop the run at round 4.
	if res.Rounds != 4 {
		t.Fatalf("Rounds = %d, want 4 (1 progress + 3 stuck repeats)", res.Rounds)
	}
}
