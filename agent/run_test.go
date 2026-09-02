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
	// The signal says what progress happened: the run took a starter and
	// walked to Pallet Town, so the final sample must show at least as
	// much progress as the early one, on the maps it actually stood on.
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

// TestRunThatNeverPlayedCarriesNoProgressSamples gives Run a zero budget,
// which stops it before its first observation: both samples must be nil.
// Nil means "never played", never "played and moved nothing" — a zero
// sample would be indistinguishable from a fresh game at round 0.
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

// TestRunCarriesIntentAcrossRounds is the deliverable: what the planner
// sent with its choice on round N-1 must be visible on round N's
// observation, and IntentAge must count up while it goes unchanged and
// reset when it changes. Run only carries the sentence — it never writes
// or edits one, so a scripted planner that says nothing sees the carried
// intent age rather than being answered for.
func TestRunCarriesIntentAcrossRounds(t *testing.T) {
	e := loadFixture(t)

	const first, second = "earn the boulder badge", "catch a pidgey on route 1"
	p := &capturePlanner{objs: []agent.Objective{
		{Kind: agent.KindStarter, Intent: first},
		// Round two says nothing: the carried intent must survive and age.
		{Kind: agent.KindGoTo, Place: "pallet town"},
		// Round three changes purpose: the observation must reset.
		{Kind: agent.KindGoTo, Place: "pallet town", Intent: second},
	}}
	res := agent.Run(e, e.ROM(), p, testBudget())

	if res.Stop != agent.StopDone {
		t.Fatalf("Stop = %d, want StopDone (Err = %v)", res.Stop, res.Err)
	}
	if len(p.seen) != 4 {
		t.Fatalf("len(seen) = %d, want 4 (initial + one per round)", len(p.seen))
	}

	// Round one's observation carries nothing yet: the planner has not said
	// a word.
	if p.seen[0].Intent != "" || p.seen[0].IntentAge != 0 {
		t.Errorf("seen[0] = Intent %q Age %d, want empty/0 before the planner speaks",
			p.seen[0].Intent, p.seen[0].IntentAge)
	}

	// Round two's observation is what the planner sent on round one: the
	// sentence verbatim, age 0 (set last round).
	if p.seen[1].Intent != first {
		t.Errorf("seen[1].Intent = %q, want %q (what round one sent)", p.seen[1].Intent, first)
	}
	if p.seen[1].IntentAge != 0 {
		t.Errorf("seen[1].IntentAge = %d, want 0 (set last round)", p.seen[1].IntentAge)
	}

	// Round three: the planner said nothing on round two, so the SAME
	// sentence comes back with age counting up.
	if p.seen[2].Intent != first {
		t.Errorf("seen[2].Intent = %q, want %q (unchanged, still carried)", p.seen[2].Intent, first)
	}
	if p.seen[2].IntentAge != 1 {
		t.Errorf("seen[2].IntentAge = %d, want 1 (counting up while unchanged)", p.seen[2].IntentAge)
	}

	// Round four: the planner changed purpose on round three, so the new
	// sentence comes back with the age reset.
	if p.seen[3].Intent != second {
		t.Errorf("seen[3].Intent = %q, want %q (what round three sent)", p.seen[3].Intent, second)
	}
	if p.seen[3].IntentAge != 0 {
		t.Errorf("seen[3].IntentAge = %d, want 0 (reset when it changes)", p.seen[3].IntentAge)
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

// TestRunRefusedPurchaseIsAFailureNotADoneRound is the Run-level half of
// S10-2d: a purchase the clerk refuses must land in the failure tally the
// planner reads next round, not in Completed. The viridian_mart fixture
// stands the player facing the clerk, and the Viridian Mart stocks POKe
// BALL, ANTIDOTE, PARLYZ HEAL and BURN HEAL — no POTION — so "buy 1
// POTION" is a deterministic refusal (ErrNotInStock) on replay. Before the
// fix the round came back nil: history said "done", Knowledge counted the
// refused purchase in Completed, and the menu line grew a "(done 1x)" for
// something the clerk had refused — the planner then had no reason to stop
// choosing it, which is exactly the loop that killed the measured run four
// rounds later (skill/shop.go's ErrNotInStock comment keeps the record).
func TestRunRefusedPurchaseIsAFailureNotADoneRound(t *testing.T) {
	e := fixture.Load(t, "viridian_mart")

	p := &capturePlanner{objs: []agent.Objective{
		{Kind: agent.KindBuy, Item: 0x14, Qty: 1}, // POTION: not stocked in Viridian
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

	// The next round's observation carries the refusal as a FAILED round
	// with the game's own words, so the planner can react (go elsewhere)
	// instead of re-choosing a done-marked purchase.
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

	// The failure that ends the run IS the run's error: StopFailed and a
	// nil Err would leave the caller with no reason to print.
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
	// Err means why the run STOPPED. This run did not stop on a failure, so
	// it has no error — even though three of its rounds failed. Setting it
	// on every failure made a healthy run print "error: ... blacked out"
	// at the end, and made the farm file a recovered blackout as the run's
	// failure detail, so the wall counted recovered runs as failed ones.
	if res.Err != nil {
		t.Errorf("Err = %v on a run that recovered and finished; failed rounds belong in History, not in Err", res.Err)
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
	// The record must name what the loss cost, or a healed party in a town
	// reads to the planner as a free reset. The respawn is PALLET_TOWN
	// because this run never healed at a Center, and the money is halved.
	if got := p.seen[1].History[0].Outcome; !strings.Contains(got, "respawned in PALLET_TOWN") {
		t.Errorf("blackout outcome = %q, want it to name the respawn place", got)
	}
	if p.seen[1].RespawnPlace != "PALLET_TOWN" {
		t.Errorf("seen[1].RespawnPlace = %q, want PALLET_TOWN (no Center was used, so wLastBlackoutMap is still its new-game value)", p.seen[1].RespawnPlace)
	}
	// Halved exactly, not just "went down": ResetStatusAndHalveMoneyOnBlackout
	// (home/overworld.asm:767) is the whole reason a blackout loop is
	// expensive, and a zero-money start would make a "went down" check pass
	// while proving nothing.
	if p.seen[0].Money == 0 {
		t.Fatal("the run started the blackout round with no money; the halving cannot be observed")
	}
	if want := p.seen[0].Money / 2; p.seen[1].Money != want {
		t.Errorf("money %d -> %d across the blackout, want %d (halved)", p.seen[0].Money, p.seen[1].Money, want)
	}
	t.Logf("blackout cost: respawned in %s, money %d -> %d", p.seen[1].RespawnPlace, p.seen[0].Money, p.seen[1].Money)
	if len(res.Final.History) != 3 {
		t.Fatalf("Final.History = %+v, want all three failed rounds", res.Final.History)
	}
	for i, h := range res.Final.History {
		if !strings.HasPrefix(h.Outcome, "failed: ") {
			t.Errorf("Final.History[%d] = %+v, want a failed round", i, h)
		}
	}
}

// TestRunTrainRetreatTwiceDoesNotStopTheRun: Train's fourth exit must be
// exempt from the failure accounting exactly the way a blackout is. The
// setup leaves the lead below the retreat line, so the two scripted train
// objectives fail with the same objective and the same error text —
// precisely the same-failure-twice case that ends a run with StopFailed.
// Without the exemption this run dies on round 2; with it, both rounds are
// recorded in history and the planner's end finishes the run. The lead is
// never sent into a battle: both objectives stop before they fight.
func TestRunTrainRetreatTwiceDoesNotStopTheRun(t *testing.T) {
	e := fixture.Load(t, "post_errand")

	dest, ok := skill.Place("route 2")
	if !ok {
		t.Fatal("Place(route 2) did not resolve")
	}
	if _, err := fixture.Travel(e, dest, skill.StatAwareMove(e.ROM()), 6); err != nil {
		t.Fatalf("setup: travel to route 2: %v", err)
	}
	// Pre-damage: one grind session takes the lead below the retreat line
	// and stops there (the behaviour under test, pinned by
	// TestTrainRetreatsBeforeBlackout). From here on every train objective
	// ends before it fights, at the level it started.
	res, err := skill.Train(e, e.ROM(), 99, skill.StatAwareMove(e.ROM()), 40)
	if err != nil {
		t.Fatalf("setup Train: %v", err)
	}
	if !res.Retreated {
		t.Fatalf("setup: Retreated = false (reached=%v blackedOut=%v battles=%d); the test needs the lead below the retreat line", res.Reached, res.BlackedOut, res.Battles)
	}

	p := &capturePlanner{objs: []agent.Objective{
		{Kind: agent.KindTrain, Level: 100},
		{Kind: agent.KindTrain, Level: 100},
	}}
	run := agent.Run(e, e.ROM(), p, testBudget())

	if run.Stop != agent.StopDone {
		t.Fatalf("Stop = %d after %d rounds, want StopDone (a retreat is exempt from the failure accounting, the way a blackout is)", run.Stop, run.Rounds)
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
	// The exemption is only exercised if the two failures were identical:
	// same objective, same error text. If the strings differ, this test
	// would pass even without the exemption, because the same-failure-twice
	// check compares both.
	if h0 != h1 {
		t.Errorf("the two retreat failures differ (%q vs %q); this test no longer exercises the same-failure-twice path", h0, h1)
	}
	// The planner saw the first retreat before choosing the second one.
	if len(p.seen) != 3 || !strings.Contains(p.seen[1].History[0].Outcome, "stopped while the party was alive") {
		t.Errorf("p.seen = %d observation(s); seen[1] must carry the first retreat in its history", len(p.seen))
	}
	// The lead is still alive: neither round fought a battle.
	var mem state.Mem
	state.Snapshot(e, &mem)
	if lead := state.DecodeParty(&mem).Mons[0]; lead.HP == 0 {
		t.Errorf("lead HP = 0 in RAM after the run; neither round should have fought")
	}
	t.Logf("two identical retreats: %q — the run survived both", h0)
}

// TestRunTrainRetreatStreakStopsTheRun: MEASURED on the live GPU farm
// (2026-08-30, /tmp/pokepilot-run-llm-gpu-9b.log), a lead that retreats at
// the same level every attempt gets offered — and repeats — the identical
// train objective 23 rounds running: retreated skips StopFailed AND never
// touches `stuck` (success-path only), so nothing in Run ever noticed. Two
// identical retreats must still survive (TestRunTrainRetreatTwiceDoesNotStopTheRun
// pins that), but the streak needs a ceiling. This pins maxConsecFailures as
// that ceiling: one more identical retreat than the default
// (defaultMaxConsecutiveFailures = 3) stops the run with StopFailed instead
// of burning the rest of the round budget on a choice that will never
// change the world.
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
		t.Fatalf("setup: Retreated = false (reached=%v blackedOut=%v battles=%d); the test needs the lead below the retreat line", res.Reached, res.BlackedOut, res.Battles)
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
		t.Fatalf("Stop = %d after %d rounds, want StopFailed (the retreat exemption must have a ceiling)", run.Stop, run.Rounds)
	}
	if run.Rounds != 3 {
		t.Fatalf("Rounds = %d, want 3 (defaultMaxConsecutiveFailures=3: the streak reaches the cap on the 3rd identical retreat)", run.Rounds)
	}
	if run.Err == nil || !strings.Contains(run.Err.Error(), "stopped while the party was alive") {
		t.Errorf("Err = %v, want the retreat error", run.Err)
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
		// A checkpoint is a state file with its knowledge file beside it;
		// this helper names the STATE, so skip the knowledge half.
		if strings.HasPrefix(en.Name(), prefix) && strings.HasSuffix(en.Name(), ".state") {
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
// the last CheckpointKeep checkpoints, not one per round forever. A run that
// executes four rounds with keep=2 leaves exactly the last two checkpoints
// on disk — each a state file with its knowledge file beside it — proof the
// bound evicts rather than grows, and evicts the pair as one.
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
	var states []string
	for _, en := range entries {
		if strings.HasSuffix(en.Name(), ".state") {
			states = append(states, en.Name())
		}
	}
	if len(states) != 2 {
		t.Fatalf("%d checkpoint states on disk, want exactly 2 (the ring must evict): %v", len(states), entries)
	}
	if !strings.Contains(states[0], "round-003") || !strings.Contains(states[1], "round-004") {
		t.Fatalf("ring kept %v, want the newest two rounds (003 and 004)", states)
	}
	// Each surviving state has its knowledge file beside it, and no orphaned
	// knowledge survived the eviction.
	for _, st := range states {
		base := strings.TrimSuffix(st, ".state")
		found := false
		for _, en := range entries {
			if strings.HasPrefix(en.Name(), base+".") && strings.Contains(en.Name(), "knowledge") && strings.HasSuffix(en.Name(), ".json") {
				found = true
			}
		}
		if !found {
			t.Fatalf("no knowledge file beside %s (entries: %v)", st, entries)
		}
	}
}

// replyPlanner is a scripted planner that also implements FeedbackPlanner:
// it can reject its first Next with a malformed-reply error (the shape of
// an LLM planner's "argument does not apply" rejection) and then answer.
// It records every Retry it is handed so a test can assert what the re-ask
// differs by (feedback text, temperature, max_tokens), and counts every ask.
type replyPlanner struct {
	objs      []agent.Objective
	rejectErr error // returned by the next `rejects` asks, Next and NextRetry alike
	rejects   int
	next      int
	asks      int
	feedback  []string      // the Feedback half of every Retry handed to NextRetry
	retries   []agent.Retry // every Retry handed to NextRetry, in order
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

// TestRunModelMismatchNotRetried is the class that must NOT be retried: a
// reply answered by a different model than the one requested is a serving
// defect, and no re-ask changes which model the server loads. The old
// mechanism spent MaxReplyRetries identical asks on it; the run must stop
// after exactly one ask.
func TestRunModelMismatchNotRetried(t *testing.T) {
	e := loadFixture(t)

	// More rejections than any retry budget: if the class were misread as
	// retryable, the run would burn MaxReplyRetries asks.
	p := &replyPlanner{
		rejectErr: fmt.Errorf("server: %w: requested %q but %q answered", agent.ErrModelMismatch, "qwen3.5-4b", "other"),
		rejects:   10,
	}
	res := agent.Run(e, e.ROM(), p, testBudget())

	if res.Stop != agent.StopError {
		t.Fatalf("Stop = %d, want StopError (a mismatched model is a finding): Err = %v", res.Stop, res.Err)
	}
	if p.asks != 1 {
		t.Fatalf("asks = %d, want 1: a model mismatch cannot change on a re-ask", p.asks)
	}
	if res.ReplyRetries != 0 {
		t.Fatalf("ReplyRetries = %d, want 0 (nothing was re-asked)", res.ReplyRetries)
	}
}

// TestRunLengthTruncationRetriedWithLargerBudget pins the "length" design:
// finish_reason "length" means the reply was cut off at the completion
// budget, so the retry differs by REQUEST, not by prompt — max_tokens goes
// up and the prompt is unchanged (no feedback quoted). A re-ask of the same
// request truncates again at the same token.
func TestRunLengthTruncationRetriedWithLargerBudget(t *testing.T) {
	e := loadFixture(t)

	p := &replyPlanner{
		rejectErr: fmt.Errorf("agent: llm planner: %w: finish_reason %q", agent.ErrNotFinished, "length"),
		rejects:   1, // the first ask truncates; the re-ask answers normally
		objs:      []agent.Objective{{Kind: agent.KindStarter}},
	}
	res := agent.Run(e, e.ROM(), p, testBudget())

	if res.Stop != agent.StopDone {
		t.Fatalf("Stop = %d, want StopDone (the re-ask answered): Err = %v", res.Stop, res.Err)
	}
	if res.ReplyRetries != 1 {
		t.Fatalf("ReplyRetries = %d, want 1", res.ReplyRetries)
	}
	if len(p.retries) != 1 {
		t.Fatalf("retries = %v, want exactly one re-ask", p.retries)
	}
	r := p.retries[0]
	if r.MaxTokensFactor != 2 {
		t.Errorf("MaxTokensFactor = %d, want 2 (the budget is what must change)", r.MaxTokensFactor)
	}
	if r.Feedback != "" {
		t.Errorf("Feedback = %q, want empty: the prompt is unchanged, the budget differs", r.Feedback)
	}
	if r.Temperature != nil {
		t.Errorf("Temperature = %v, want nil: sampling is not the defect", r.Temperature)
	}
}

// TestRunOtherFinishReasonNotRetried: a non-stop finish_reason that is not
// "length" (a content filter, ...) is deterministic at temperature 0 with an
// unchanged prompt — a re-ask returns the same bytes, so it is not retried.
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
	if p.asks != 1 {
		t.Fatalf("asks = %d, want 1: a non-length non-stop finish cannot change on a re-ask", p.asks)
	}
	if res.ReplyRetries != 0 {
		t.Fatalf("ReplyRetries = %d, want 0", res.ReplyRetries)
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

// offerCapturingPlanner wraps a scripted planner and records the offered
// menu of its first round, so a test can assert what the run's Offer built
// without reaching into Run. It then hands control back to the script.
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

// latestCheckpoint returns the newest .state file in a checkpoint ring's
// directory and asserts its knowledge file sits beside it: that pairing is
// what makes resume safe, so a ring without it has broken.
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
	t.Fatalf("no knowledge file beside %s (entries: %v)", last, entries)
	return ""
}

// TestRunResumesKnowledge is the behaviour a human would actually notice:
// a run that completed a one-shot and then died does not offer it again
// after a resume. The first run talks to the person at (2,1) on Pallet Town
// (a one-shot: once talked to, Offer stops offering them), completes a
// second objective so its LAST checkpoint was taken with that talk already
// in the knowledge, and finishes. A fresh emulator resumes from that last
// checkpoint, and its first Offer must not re-offer the talk - while the
// first run's own Offer did offer it, which is what makes the absence mean
// something.
//
// (2,1) was chosen for a reason: on the post-talk save state it is STILL
// offered with empty knowledge (measured), so its absence from the resumed
// Offer can only come from the restored Talked record, not from the game
// hiding the sprite. The (6,3) person does hide after a talk, which would
// let this test pass for the wrong reason.
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

	// A FRESH emulator: whatever the resumed run knows must come from the
	// checkpoint pair, not from the first run's process.
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
		t.Fatal("resumed run: Offer was empty; nothing to assert about")
	}
	if res2.Final.Map != 0x28 {
		t.Fatalf("resumed run: Final.Map = %#04x, want 0x28 (the checkpoint's map)", res2.Final.Map)
	}
	for _, o := range resumedOffered {
		if o.Kind == agent.KindTalk && o.X == 2 && o.Y == 1 {
			t.Fatalf("resumed run re-offers the one-shot talk at (2,1) the first run completed: %v", resumedOffered)
		}
	}
}

// The Viridian Forest North Gate Super Nerd says, across two pages of one
// utterance:
//
//	page 1: "Many POKéMON live only in forests and caves."
//	page 2: "You need to look everywhere to get different kinds!"
//
// page 2 carries the "you need" shape. But the Gen 1 dialogue box shows only
// TWO text rows and scrolls (pokered/home/text.asm ScrollTextUpOneLine:
// "move both rows of text in the normal text box up one row"), so a three-
// line page is never all on screen at once: when line 3 starts typing, line
// 1 is erased. The per-frame dialogue tape settles on the FINAL window of
// each page (two consecutive identical samples), so it keeps the scrolled
// line with the first line of the page gone. MEASURED 2026-09-01 on this
// fixture: the tape captures the two pages as
//
//	"only in forests and caves."
//	"everywhere to get different kinds!"
//
// — "You need to look" scrolled off before the tape settled, so the captured
// line does not contain "you need" and the requirement is not harvested.
// These are the verbatim strings the live tape produced on the run this test
// asserts.
const (
	superNerdPage1Captured = "only in forests and caves."
	superNerdPage2Captured = "everywhere to get different kinds!"
)

// TestRunHearsRequirementLive is the chain S9-6 proved link by link in unit
// tests, running live for the first time: a real Talk, through the per-frame
// dialogue tape, into RecentDialogue, through SawDialogue, into
// Knowledge.Requirements, out to Observation.Requirements. S9-12 measured
// Requirements empty in every observation of every earlier live run — no
// requirement sentence was in range. The forest_north_gate fixture (S10-8)
// ends two steps from the Super Nerd, so one talk reaches him.
//
// FINDING (the reason this test asserts what it does): the chain runs live
// end-to-end — the talk is heard, the lines reach RecentDialogue, and
// SawDialogue runs over them — but the requirement is NOT harvested. The
// Super Nerd's "you need" phrase sits on the FIRST line of page 2, and the
// dialogue box's two-row scroll erases that line before the tape settles on
// the page's final window, so the captured line is "everywhere to get
// different kinds!" — no "you need", no match. This is not a shape-list gap
// ("you need" is present in requirementShapes and is the right idiom) and
// not an emulator bug (the two-row scroll is real Gen 1 behaviour); it is
// the tape not accumulating across the box's scroll. See the S10-9 RUNNOTES
// for the follow-up (teach the tape to keep the line that scrolls off).
//
// Written with the same suspicion as TestRunResumesKnowledge, which
// documents a real "passes for the wrong reason" hazard: each assertion
// must be able to fail for the reason this task cares about. StopDone plus
// one Completed rules out a RecentDialogue entry that arrived without the
// utterance ever happening (a failed talk stops the run before that); the
// exact-string assertions pin the VERBATIM lines the tape produced, so the
// test fails if the capture changes (a scroll fix that recovers "You need
// to look" would change superNerdPage2Captured and fail this test, which is
// the point — the finding is then re-examined); the empty-Requirements
// assertion is the guard that the filter has not quietly become "keep
// everything" (it must still drop the chatter page even though it heard it).
func TestRunHearsRequirementLive(t *testing.T) {
	e := fixture.Load(t, "forest_north_gate")

	p := &capturePlanner{objs: []agent.Objective{
		{Kind: agent.KindTalk, X: 3, Y: 2}, // the north gate's Super Nerd
	}}
	res := agent.Run(e, e.ROM(), p, testBudget())
	if res.Stop != agent.StopDone {
		t.Fatalf("Stop = %d, err = %v; want StopDone", res.Stop, res.Err)
	}
	if len(res.Completed) != 1 {
		t.Fatalf("Completed = %v, want the single talk", res.Completed)
	}

	// The utterance was actually heard by the live tape this run. Both pages
	// arrive as separate RecentDialogue entries (the box closes between
	// them), each truncated to the page's final two-row window. Without this
	// the Requirements assertion could be satisfied by a line that reached
	// the observation by some other route.
	got := map[string]bool{}
	for _, line := range res.Final.RecentDialogue {
		got[line] = true
	}
	for _, want := range []string{superNerdPage1Captured, superNerdPage2Captured} {
		if !got[want] {
			t.Fatalf("RecentDialogue = %q, want the verbatim captured page %q to have been heard live", res.Final.RecentDialogue, want)
		}
	}

	// The finding: the requirement was NOT harvested. The "you need" phrase
	// is on the first line of page 2, which the box's two-row scroll erased
	// before the tape settled, so the captured line carries no shape. This
	// assertion is the guard against a filter that has quietly become "keep
	// everything": even though the run heard two dialogue lines, neither may
	// land in Requirements (the chatter page is dropped for lack of a shape,
	// and the requirement page for the same reason — its shape scrolled off).
	if len(res.Final.Requirements) != 0 {
		t.Fatalf("Requirements = %v, want empty: the box's two-row scroll erases "+
			"the \"you need\" line before the tape settles, so no captured line carries a shape", res.Final.Requirements)
	}
}
