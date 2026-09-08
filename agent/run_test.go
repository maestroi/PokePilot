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

// TestStopZeroValueIsUnset pins the invariant the S7-4 failure was built on:
// the zero value of Stop must mean "no reason set yet", never a real reason.
// StopDone used to be Stop(0), so a finished planner read as "keep going" in
// a "stop != 0" check and Execute ran on an empty Objective. If a Stop
// reason is ever renumbered ahead of StopUnset, this test fails instead of
// the whole suite dying with StopFailed.
func TestStopZeroValueIsUnset(t *testing.T) {
	if agent.Stop(0) != agent.StopUnset {
		t.Fatalf("Stop(0) = %d, want StopUnset (the zero value must mean unset)", agent.Stop(0))
	}
	for _, s := range []agent.Stop{agent.StopDone, agent.StopStuck, agent.StopBudget, agent.StopFailed, agent.StopError} {
		if s == agent.StopUnset {
			t.Fatalf("Stop reason %d collides with StopUnset: a reported reason must not be the zero value", s)
		}
	}
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

// TestRunCarriesProgressSamplesAtTwoPoints runs starter -> Pallet Town and
// expects the run to carry BOTH progress samples: one before the first
// objective ran and one at the stop. The pair is what lets a dump of one
// run answer "did this move?" — a single end-of-run snapshot cannot,
// because a run that stalled at round 3 and one that progressed steadily
// can stop looking identical.
func TestRunCarriesProgressSamplesAtTwoPoints(t *testing.T) {
	e := loadFixture(t)

	var log bytes.Buffer
	p := agent.NewScriptedPlanner(
		agent.Objective{Kind: agent.KindStarter},
		agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"},
	)
	b := testBudget()
	b.Log = &log
	res := agent.Run(e, e.ROM(), p, b)

	if res.Stop != agent.StopDone {
		t.Fatalf("Stop = %d, want StopDone", res.Stop)
	}
	if res.ProgressEarly == nil || res.ProgressFinal == nil {
		t.Fatalf("a run that played must carry both progress samples: early=%v final=%v", res.ProgressEarly, res.ProgressFinal)
	}
	early, final := res.ProgressEarly, res.ProgressFinal
	if early.Round != 0 {
		t.Errorf("early sample Round = %d, want 0 (taken before the first objective ran)", early.Round)
	}
	if final.Round != res.Rounds {
		t.Errorf("final sample Round = %d, want %d (the run's last round)", final.Round, res.Rounds)
	}
	if final.Maps < early.Maps {
		t.Errorf("Maps shrank: early=%d final=%d", early.Maps, final.Maps)
	}
	if final.Events < early.Events {
		t.Errorf("Events shrank: early=%d final=%d", early.Events, final.Events)
	}
	if final.Maps < 2 {
		t.Errorf("final Maps = %d, want >= 2 (the run stood on the bedroom and Pallet Town)", final.Maps)
	}
	if final.Map != res.Final.Map {
		t.Errorf("final sample Map = %#04x, want the run's final map %#04x", final.Map, res.Final.Map)
	}
}

func TestRunThatNeverPlayedCarriesNoProgressSamples(t *testing.T) {
	e := loadFixture(t)
	p := agent.NewScriptedPlanner(agent.Objective{Kind: agent.KindStarter})
	res := agent.Run(e, e.ROM(), p, agent.Budget{})
	if res.Stop != agent.StopError {
		t.Fatalf("Stop = %d, want StopError (a zero budget is not unlimited)", res.Stop)
	}
	if res.ProgressEarly != nil || res.ProgressFinal != nil {
		t.Fatalf("a run that never observed must carry nil samples: early=%v final=%v", res.ProgressEarly, res.ProgressFinal)
	}
}

type errPlanner struct{ err error }

func (p *errPlanner) Next(agent.Observation, []agent.Objective) (agent.Objective, error) {
	return agent.Objective{}, p.err
}

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

func TestRunObservationCarriesHistoryAndMoves(t *testing.T) {
	e := loadFixture(t)
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
	if len(p.seen) != 3 {
		t.Fatalf("len(seen) = %d, want 3 (initial + one per round)", len(p.seen))
	}
	second := p.seen[1]
	if len(second.History) != 1 {
		t.Fatalf("History = %+v, want exactly the completed starter round", second.History)
	}
	if second.History[0].Objective != "take the charmander starter" || second.History[0].Outcome != "done" {
		t.Errorf("History[0] = %+v, want {take the charmander starter done}", second.History[0])
	}
	if len(second.LeadMoves) != 2 || second.LeadMoves[0] != (agent.Move{Power: 40, Type: "normal"}) {
		t.Errorf("LeadMoves = %+v, want SCRATCH (power 40 normal) first", second.LeadMoves)
	}
	if len(second.RecentDialogue) == 0 {
		t.Fatalf("RecentDialogue is empty after a story full of text boxes")
	}
	third := p.seen[2]
	if len(third.History) != 2 || third.History[0].Objective != "take the charmander starter" {
		t.Errorf("third History = %+v, want both rounds, oldest first", third.History)
	}
}

func TestRunCarriesIntentAcrossRounds(t *testing.T) {
	e := loadFixture(t)

	const first, second = "earn the boulder badge", "catch a pidgey on route 1"
	p := &capturePlanner{objs: []agent.Objective{
		{Kind: agent.KindStarter, Intent: first},
		{Kind: agent.KindGoTo, Place: "pallet town"},
		{Kind: agent.KindGoTo, Place: "pallet town", Intent: second},
	}}
	res := agent.Run(e, e.ROM(), p, testBudget())

	if res.Stop != agent.StopDone {
		t.Fatalf("Stop = %d, want StopDone (Err = %v)", res.Stop, res.Err)
	}
	if len(p.seen) != 4 {
		t.Fatalf("len(seen) = %d, want 4 (initial + one per round)", len(p.seen))
	}
	if p.seen[0].Intent != "" || p.seen[0].IntentAge != 0 {
		t.Errorf("seen[0] = Intent %q Age %d, want empty/0 before the planner speaks", p.seen[0].Intent, p.seen[0].IntentAge)
	}
	if p.seen[1].Intent != first || p.seen[1].IntentAge != 0 {
		t.Errorf("seen[1] = Intent %q Age %d, want %q/0", p.seen[1].Intent, p.seen[1].IntentAge, first)
	}
	if p.seen[2].Intent != first || p.seen[2].IntentAge != 1 {
		t.Errorf("seen[2] = Intent %q Age %d, want %q/1", p.seen[2].Intent, p.seen[2].IntentAge, first)
	}
	if p.seen[3].Intent != second || p.seen[3].IntentAge != 0 {
		t.Errorf("seen[3] = Intent %q Age %d, want %q/0", p.seen[3].Intent, p.seen[3].IntentAge, second)
	}
}

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
	if !strings.Contains(h.Outcome, "no party to heal") {
		t.Errorf("History[0].Outcome = %q, want the skill's failure text", h.Outcome)
	}
	if !second.Controllable {
		t.Errorf("not controllable after the failed objective: %+v", second)
	}
	if res.Final.PartyCount != 1 {
		t.Errorf("Final.PartyCount = %d, want 1 (the starter started from the failure's aftermath)", res.Final.PartyCount)
	}
}

func TestRunRefusedPurchaseIsAFailureNotADoneRound(t *testing.T) {
	e := fixture.Load(t, "viridian_mart")

	p := &capturePlanner{objs: []agent.Objective{
		{Kind: agent.KindBuy, Item: agent.ItemID("potion"), Qty: 1},
	}}
	res := agent.Run(e, e.ROM(), p, testBudget())

	if res.Stop != agent.StopDone {
		t.Fatalf("Stop = %d, want StopDone (a refused purchase does not end the run)", res.Stop)
	}
	if len(res.Completed) != 0 {
		t.Fatalf("Completed = %v, want empty (the purchase did not happen)", res.Completed)
	}
	if res.Err != nil {
		t.Fatalf("Err = %v, want nil (the refusal was recovered from, not terminal)", res.Err)
	}
	if len(p.seen) != 2 {
		t.Fatalf("len(seen) = %d, want 2 (initial + one per round)", len(p.seen))
	}
	obs := p.seen[1]
	if len(obs.History) != 1 {
		t.Fatalf("History = %+v, want the refused purchase round", obs.History)
	}
	h := obs.History[0]
	if h.Objective != "buy 1 POTION" || !strings.HasPrefix(h.Outcome, "failed: ") {
		t.Fatalf("History[0] = %+v, want {buy 1 POTION failed: ...}", h)
	}
	if !strings.Contains(h.Outcome, "does not stock") {
		t.Errorf("History[0].Outcome = %q, want the clerk's refusal quoted back", h.Outcome)
	}
}

func TestRunRepeatedFailureStops(t *testing.T) {
	e := loadFixture(t)

	objs := make([]agent.Objective, 10)
	for i := range objs {
		objs[i] = agent.Objective{Kind: agent.KindHeal}
	}
	b := testBudget()
	b.MaxRounds = 50
	res := agent.Run(e, e.ROM(), agent.NewScriptedPlanner(objs...), b)

	if res.Err == nil {
		t.Error("Err = nil on a run stopped by the failure budget; the failure that ended it is the run's error")
	}
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
	if len(res.Final.History) != 2 {
		t.Fatalf("Final.History = %+v, want both failed rounds", res.Final.History)
	}
	for i, h := range res.Final.History {
		if h.Objective != "heal the party" || !strings.HasPrefix(h.Outcome, "failed: ") {
			t.Errorf("History[%d] = %+v, want {heal the party failed: ...}", i, h)
		}
	}
}

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

func TestRunBlackoutDoesNotStopTheRun(t *testing.T) {
	e := fixture.Load(t, "post_starter")

	dest, ok := skill.Place("route 1")
	if !ok {
		t.Fatal("Place(route 1) did not resolve")
	}
	if _, err := fixture.Travel(e, dest, skill.StatAwareMove(e.ROM()), 20); err != nil {
		t.Fatalf("setup: travel to route 1: %v", err)
	}

	p := &capturePlanner{objs: []agent.Objective{
		{Kind: agent.KindTrain, Level: 100},
		{Kind: agent.KindHeal},
		{Kind: agent.KindGoTo, Place: "atlantis"},
	}}
	res := agent.Run(e, e.ROM(), p, testBudget())

	if res.Stop != agent.StopDone {
		t.Fatalf("Stop = %d after %d rounds, want StopDone (the blackout is not a failure-budget event)", res.Stop, res.Rounds)
	}
	if res.Err != nil {
		t.Errorf("Err = %v on a run that recovered and finished; failed rounds belong in History, not in Err", res.Err)
	}
	if res.Rounds != 3 {
		t.Fatalf("Rounds = %d, want 3", res.Rounds)
	}
	if len(res.Completed) != 0 {
		t.Fatalf("Completed = %v, want empty", res.Completed)
	}
	if len(p.seen) != 4 {
		t.Fatalf("len(seen) = %d, want 4", len(p.seen))
	}
	if !p.seen[1].BlackedOut {
		t.Errorf("seen[1].BlackedOut = false, want true")
	}
	if len(p.seen[1].History) != 1 || !strings.Contains(p.seen[1].History[0].Outcome, "blacked out") {
		t.Errorf("seen[1].History = %+v, want the blackout failure recorded", p.seen[1].History)
	}
	if got := p.seen[1].History[0].Outcome; !strings.Contains(got, "respawned in PALLET_TOWN") {
		t.Errorf("blackout outcome = %q, want it to name the respawn place", got)
	}
	if p.seen[1].RespawnPlace != "PALLET_TOWN" {
		t.Errorf("seen[1].RespawnPlace = %q, want PALLET_TOWN", p.seen[1].RespawnPlace)
	}
	if p.seen[0].Money == 0 {
		t.Fatal("the run started the blackout round with no money")
	}
	if want := p.seen[0].Money / 2; p.seen[1].Money != want {
		t.Errorf("money %d -> %d across the blackout, want %d", p.seen[0].Money, p.seen[1].Money, want)
	}
	if len(res.Final.History) != 3 {
		t.Fatalf("Final.History = %+v, want all three failed rounds", res.Final.History)
	}
}

func TestRunTrainRetreatTwiceDoesNotStopTheRun(t *testing.T) {
	e := fixture.Load(t, "post_errand")

	dest, ok := skill.Place("route 2")
	if !ok {
		t.Fatal("Place(route 2) did not resolve")
	}
	if _, err := fixture.Travel(e, dest, skill.StatAwareMove(e.ROM()), 6); err != nil {
		t.Fatalf("setup: travel to route 2: %v", err)
	}
	res, err := skill.Train(e, e.ROM(), 99, skill.StatAwareMove(e.ROM()), 40)
	if err != nil {
		t.Fatalf("setup Train: %v", err)
	}
	if !res.Retreated {
		t.Fatalf("setup: Retreated = false")
	}

	p := &capturePlanner{objs: []agent.Objective{
		{Kind: agent.KindTrain, Level: 100},
		{Kind: agent.KindTrain, Level: 100},
	}}
	run := agent.Run(e, e.ROM(), p, testBudget())

	if run.Stop != agent.StopDone {
		t.Fatalf("Stop = %d after %d rounds, want StopDone", run.Stop, run.Rounds)
	}
	if run.Rounds != 2 {
		t.Fatalf("Rounds = %d, want 2", run.Rounds)
	}
	if len(run.Final.History) != 2 {
		t.Fatalf("Final.History = %+v, want both failed rounds", run.Final.History)
	}
	h0, h1 := run.Final.History[0].Outcome, run.Final.History[1].Outcome
	for i, h := range []string{h0, h1} {
		if !strings.Contains(h, "stopped while the party was alive") {
			t.Errorf("Final.History[%d] = %q, want the retreat outcome", i, h)
		}
	}
	if h0 != h1 {
		t.Errorf("the two retreat failures differ (%q vs %q)", h0, h1)
	}
	if len(p.seen) != 3 || !strings.Contains(p.seen[1].History[0].Outcome, "stopped while the party was alive") {
		t.Errorf("p.seen = %d observation(s); seen[1] must carry the first retreat", len(p.seen))
	}
	var mem state.Mem
	state.Snapshot(e, &mem)
	if lead := state.DecodeParty(&mem).Mons[0]; lead.HP == 0 {
		t.Errorf("lead HP = 0 in RAM after the run")
	}
}

func TestRunTrainRetreatStreakStopsTheRun(t *testing.T) {
	e := fixture.Load(t, "post_errand")

	dest, ok := skill.Place("route 2")
	if !ok {
		t.Fatal("Place(route 2) did not resolve")
	}
	if _, err := fixture.Travel(e, dest, skill.StatAwareMove(e.ROM()), 6); err != nil {
		t.Fatalf("setup: travel to route 2: %v", err)
	}
	res, err := skill.Train(e, e.ROM(), 99, skill.StatAwareMove(e.ROM()), 40)
	if err != nil {
		t.Fatalf("setup Train: %v", err)
	}
	if !res.Retreated {
		t.Fatalf("setup: Retreated = false")
	}

	objs := make([]agent.Objective, 6)
	for i := range objs {
		objs[i] = agent.Objective{Kind: agent.KindTrain, Level: 100}
	}
	p := &capturePlanner{objs: objs}
	b := testBudget()
	b.MaxRounds = 6
	run := agent.Run(e, e.ROM(), p, b)

	if run.Stop != agent.StopFailed {
		t.Fatalf("Stop = %d after %d rounds, want StopFailed", run.Stop, run.Rounds)
	}
	if run.Rounds != 3 {
		t.Fatalf("Rounds = %d, want 3", run.Rounds)
	}
	if run.Err == nil || !strings.Contains(run.Err.Error(), "stopped while the party was alive") {
		t.Errorf("Err = %v, want the retreat error", run.Err)
	}
}

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
		if strings.HasPrefix(en.Name(), prefix) && strings.HasSuffix(en.Name(), ".state") {
			return filepath.Join(dir, en.Name())
		}
	}
	t.Fatalf("no checkpoint for round %d in %s (have %v)", round, dir, names)
	return ""
}

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

	initial := p.seen[0]
	if gotMap, gotX, gotY := restoreCheckpoint(t, rom, checkpointForRound(t, dir, 1)); gotMap != initial.Map || gotX != int(initial.X) || gotY != int(initial.Y) {
		t.Errorf("round-1 checkpoint = map %#04x (%d,%d), want map %#04x (%d,%d)", gotMap, gotX, gotY, initial.Map, initial.X, initial.Y)
	}
	second := p.seen[1]
	if gotMap, gotX, gotY := restoreCheckpoint(t, rom, checkpointForRound(t, dir, 2)); gotMap != second.Map || gotX != int(second.X) || gotY != int(second.Y) {
		t.Errorf("round-2 checkpoint = map %#04x (%d,%d), want map %#04x (%d,%d)", gotMap, gotX, gotY, second.Map, second.X, second.Y)
	}
}

func TestRunCheckpointRingIsBounded(t *testing.T) {
	e := loadFixture(t)
	dir := t.TempDir()

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
		t.Fatalf("Stop = %d Rounds = %d, want StopStuck at round 4", res.Stop, res.Rounds)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", dir, err)
	}
	var states []string
	for _, en := range entries {
		if strings.HasSuffix(en.Name(), ".state") {
			states = append(states, en.Name())
		}
	}
	if len(states) != 2 {
		t.Fatalf("%d checkpoint states on disk, want exactly 2: %v", len(states), entries)
	}
	if !strings.Contains(states[0], "round-003") || !strings.Contains(states[1], "round-004") {
		t.Fatalf("ring kept %v, want rounds 003 and 004", states)
	}
	for _, st := range states {
		base := strings.TrimSuffix(st, ".state")
		found := false
		for _, en := range entries {
			if strings.HasPrefix(en.Name(), base+".") && strings.Contains(en.Name(), "knowledge") && strings.HasSuffix(en.Name(), ".json") {
				found = true
			}
		}
		if !found {
			t.Fatalf("no knowledge file beside %s", st)
		}
	}
}

type replyPlanner struct {
	objs      []agent.Objective
	rejectErr error
	rejects   int
	next      int
	asks      int
	feedback  []string
	retries   []agent.Retry
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

func (p *replyPlanner) NextRetry(obs agent.Observation, offered []agent.Objective, r agent.Retry) (agent.Objective, error) {
	p.asks++
	p.retries = append(p.retries, r)
	if r.Feedback != "" {
		p.feedback = append(p.feedback, r.Feedback)
	}
	return p.take()
}

func TestRunRejectedReplyRecovers(t *testing.T) {
	e := loadFixture(t)

	const msg = "agent: level argument 12 does not apply to go to route 1"
	p := &replyPlanner{
		rejectErr: errors.New(msg),
		rejects:   1,
		objs:      []agent.Objective{{Kind: agent.KindStarter}},
	}
	res := agent.Run(e, e.ROM(), p, testBudget())

	if res.Stop != agent.StopDone {
		t.Fatalf("Stop = %d, want StopDone: Err = %v", res.Stop, res.Err)
	}
	if len(res.Completed) != 1 || res.Completed[0].Kind != agent.KindStarter {
		t.Fatalf("Completed = %v, want the starter", res.Completed)
	}
	if res.ReplyRetries != 1 {
		t.Fatalf("ReplyRetries = %d, want 1", res.ReplyRetries)
	}
	if p.asks != 3 {
		t.Fatalf("asks = %d, want 3", p.asks)
	}
	if len(p.feedback) != 1 || !strings.Contains(p.feedback[0], msg) {
		t.Fatalf("feedback = %v, want rejection text", p.feedback)
	}
}

func TestRunRejectedReplyExhaustsRetries(t *testing.T) {
	e := loadFixture(t)

	const msg = "agent: level argument 12 does not apply to go to route 1"
	p := &replyPlanner{rejectErr: errors.New(msg), rejects: 10}
	res := agent.Run(e, e.ROM(), p, testBudget())

	if res.Stop != agent.StopError {
		t.Fatalf("Stop = %d, want StopError", res.Stop)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), msg) {
		t.Fatalf("Err = %v, want the last rejection", res.Err)
	}
	if p.asks != agent.MaxReplyRetries {
		t.Fatalf("asks = %d, want %d", p.asks, agent.MaxReplyRetries)
	}
	if res.ReplyRetries != agent.MaxReplyRetries-1 {
		t.Fatalf("ReplyRetries = %d, want %d", res.ReplyRetries, agent.MaxReplyRetries-1)
	}
	if res.Rounds != 0 || len(res.Completed) != 0 {
		t.Fatalf("Rounds = %d Completed = %v, want 0/empty", res.Rounds, res.Completed)
	}
}

func TestRunModelMismatchNotRetried(t *testing.T) {
	e := loadFixture(t)

	p := &replyPlanner{
		rejectErr: fmt.Errorf("server: %w: requested %q but %q answered", agent.ErrModelMismatch, "qwen3.5-4b", "other"),
		rejects:   10,
	}
	res := agent.Run(e, e.ROM(), p, testBudget())

	if res.Stop != agent.StopError {
		t.Fatalf("Stop = %d, want StopError: Err = %v", res.Stop, res.Err)
	}
	if p.asks != 1 {
		t.Fatalf("asks = %d, want 1", p.asks)
	}
	if res.ReplyRetries != 0 {
		t.Fatalf("ReplyRetries = %d, want 0", res.ReplyRetries)
	}
}

func TestRunLengthTruncationRetriedWithLargerBudget(t *testing.T) {
	e := loadFixture(t)

	p := &replyPlanner{
		rejectErr: fmt.Errorf("agent: llm planner: %w: finish_reason %q", agent.ErrNotFinished, "length"),
		rejects:   1,
		objs:      []agent.Objective{{Kind: agent.KindStarter}},
	}
	res := agent.Run(e, e.ROM(), p, testBudget())

	if res.Stop != agent.StopDone {
		t.Fatalf("Stop = %d, want StopDone: Err = %v", res.Stop, res.Err)
	}
	if res.ReplyRetries != 1 {
		t.Fatalf("ReplyRetries = %d, want 1", res.ReplyRetries)
	}
	if len(p.retries) != 1 {
		t.Fatalf("retries = %v, want exactly one", p.retries)
	}
	r := p.retries[0]
	if r.MaxTokensFactor != 2 {
		t.Errorf("MaxTokensFactor = %d, want 2", r.MaxTokensFactor)
	}
	if r.Feedback != "" || r.Temperature != nil {
		t.Errorf("retry = %+v, want only larger token budget", r)
	}
}

func TestRunOtherFinishReasonNotRetried(t *testing.T) {
	e := loadFixture(t)
	p := &replyPlanner{
		rejectErr: fmt.Errorf("agent: llm planner: %w: finish_reason %q", agent.ErrNotFinished, "content_filter"),
		rejects:   10,
	}
	res := agent.Run(e, e.ROM(), p, testBudget())
	if res.Stop != agent.StopError {
		t.Fatalf("Stop = %d, want StopError: Err = %v", res.Stop, res.Err)
	}
	if p.asks != 1 || res.ReplyRetries != 0 {
		t.Fatalf("asks=%d retries=%d, want 1/0", p.asks, res.ReplyRetries)
	}
}

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
		t.Fatalf("Stop = %d after %d rounds, want StopStuck", res.Stop, res.Rounds)
	}
	if res.Rounds != 4 {
		t.Fatalf("Rounds = %d, want 4", res.Rounds)
	}
}

type offerCapturingPlanner struct {
	script  *agent.ScriptedPlanner
	offered *[]agent.Objective
}

func (p *offerCapturingPlanner) Next(obs agent.Observation, off []agent.Objective) (agent.Objective, error) {
	if p.offered != nil && *p.offered == nil {
		*p.offered = append([]agent.Objective(nil), off...)
	}
	return p.script.Next(obs, off)
}

func latestCheckpoint(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	last := ""
	for _, en := range entries {
		if strings.HasSuffix(en.Name(), ".state") && en.Name() > last {
			last = en.Name()
		}
	}
	if last == "" {
		t.Fatalf("no checkpoint .state file in %s", dir)
	}
	base := strings.TrimSuffix(last, ".state")
	for _, en := range entries {
		if strings.HasPrefix(en.Name(), base+".") && strings.Contains(en.Name(), "knowledge") && strings.HasSuffix(en.Name(), ".json") {
			return filepath.Join(dir, last)
		}
	}
	t.Fatalf("no knowledge file beside %s", last)
	return ""
}

func TestRunResumesKnowledge(t *testing.T) {
	e := fixture.Load(t, "post_starter")
	dir := t.TempDir()

	var firstOffered []agent.Objective
	p := &offerCapturingPlanner{
		script: agent.NewScriptedPlanner(
			agent.Objective{Kind: agent.KindTalk, X: 2, Y: 1},
			agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"},
		),
		offered: &firstOffered,
	}
	b := testBudget()
	b.CheckpointDir = dir
	res := agent.Run(e, e.ROM(), p, b)
	if res.Stop != agent.StopDone {
		t.Fatalf("first run: Stop = %d, err = %v; want StopDone", res.Stop, res.Err)
	}
	if len(res.Completed) != 2 {
		t.Fatalf("first run: Completed = %v, want both objectives", res.Completed)
	}
	foundTalk := false
	for _, o := range firstOffered {
		if o.Kind == agent.KindTalk && o.X == 2 && o.Y == 1 {
			foundTalk = true
		}
	}
	if !foundTalk {
		t.Fatalf("first run's Offer did not offer the talk at (2,1): %v", firstOffered)
	}

	last := latestCheckpoint(t, dir)
	e2 := fixture.Load(t, "post_starter")
	var resumedOffered []agent.Objective
	p2 := &offerCapturingPlanner{script: agent.NewScriptedPlanner(), offered: &resumedOffered}
	b2 := testBudget()
	b2.ResumeFrom = last
	res2 := agent.Run(e2, e2.ROM(), p2, b2)

	if res2.Stop != agent.StopDone {
		t.Fatalf("resumed run: Stop = %d, err = %v; want StopDone", res2.Stop, res2.Err)
	}
	if len(resumedOffered) == 0 {
		t.Fatal("resumed run: Offer was empty")
	}
	if res2.Final.Map != 0x28 {
		t.Fatalf("resumed run: Final.Map = %#04x, want 0x28", res2.Final.Map)
	}
	for _, o := range resumedOffered {
		if o.Kind == agent.KindTalk && o.X == 2 && o.Y == 1 {
			t.Fatalf("resumed run re-offers the one-shot talk at (2,1): %v", resumedOffered)
		}
	}
}

const (
	superNerdPage1Captured = "only in forests and caves."
	superNerdPage2Captured = "everywhere to get different kinds!"
)

func TestRunHearsRequirementLive(t *testing.T) {
	e := fixture.Load(t, "forest_north_gate")

	p := &capturePlanner{objs: []agent.Objective{
		{Kind: agent.KindTalk, X: 3, Y: 2},
	}}
	res := agent.Run(e, e.ROM(), p, testBudget())
	if res.Stop != agent.StopDone {
		t.Fatalf("Stop = %d, err = %v; want StopDone", res.Stop, res.Err)
	}
	if len(res.Completed) != 1 {
		t.Fatalf("Completed = %v, want the single talk", res.Completed)
	}

	got := map[string]bool{}
	for _, line := range res.Final.RecentDialogue {
		got[line] = true
	}
	for _, want := range []string{superNerdPage1Captured, superNerdPage2Captured} {
		if !got[want] {
			t.Fatalf("RecentDialogue = %q, want captured page %q", res.Final.RecentDialogue, want)
		}
	}
	if len(res.Final.Requirements) != 0 {
		t.Fatalf("Requirements = %v, want empty", res.Final.Requirements)
	}
}
