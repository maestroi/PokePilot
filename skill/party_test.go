package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestSetLeadOneMon is the fast half of S6-5a: a party of one needs no
// decisions, so SetLead(0) must be a verified no-op that leaves the lead
// and the player exactly where they were, and every other slot is rejected
// before any input is pressed. The reorder itself — a species actually
// moving into slot 0 — is proven by TestSetLeadAndVoluntarySwitch on a real
// two-mon party; no two-mon fixture exists yet, S6-3's catch is the only
// way to grow the party.
func TestSetLeadOneMon(t *testing.T) {
	// post_pokeballs carries the one-mon party (a level-15 SQUIRTLE); the
	// reds_bedroom fixture is pre-starter and has none.
	m := fixture.Load(t, "post_pokeballs")

	var mem state.Mem
	state.Snapshot(m, &mem)
	party := state.DecodeParty(&mem)
	if party.Count != 1 || party.Mons[0].Species != speciesSquirtle {
		t.Fatalf("fixture precondition: %+v, want a lone SQUIRTLE (%#02x)", party.Mons, speciesSquirtle)
	}
	lead := party.Mons[0].Species
	before := playerAt(t, m)

	if err := skill.SetLead(m, 0); err != nil {
		t.Fatalf("SetLead(0): %v", err)
	}
	state.Snapshot(m, &mem)
	if got := state.DecodeParty(&mem).Mons[0].Species; got != lead {
		t.Errorf("lead = species %#02x after SetLead(0), want unchanged %#02x", got, lead)
	}
	after := playerAt(t, m)
	if before.MapID != after.MapID || before.X != after.X || before.Y != after.Y {
		t.Errorf("player moved: before %+v, after %+v", before, after)
	}
	if !state.Controllable(&mem) {
		t.Error("player not controllable after SetLead(0)")
	}

	// Table: every slot outside [0, Count) is rejected, and a rejection
	// must not have touched the party.
	for _, slot := range []int{-1, int(party.Count), int(party.Count) + 3} {
		if err := skill.SetLead(m, slot); err == nil {
			t.Errorf("SetLead(%d) = nil, want out-of-range error (party of %d)", slot, party.Count)
		}
	}
	state.Snapshot(m, &mem)
	if got := state.DecodeParty(&mem).Mons[0].Species; got != lead {
		t.Errorf("lead changed to species %#02x after rejected SetLead calls", got)
	}
	if !state.Controllable(&mem) {
		t.Error("player not controllable after rejected SetLead calls")
	}
}

// TestSwitchActiveNoBattle: with no battle in progress SwitchActive must
// fail and leave the player exactly where they were — a failure assertion
// that also asserts nothing changed.
func TestSwitchActiveNoBattle(t *testing.T) {
	m := fixture.Load(t, "post_pokeballs")

	before := playerAt(t, m)
	for _, slot := range []int{0, 1} {
		if err := skill.SwitchActive(m, slot); err == nil {
			t.Errorf("SwitchActive(%d) = nil, want no-battle error", slot)
		}
	}
	after := playerAt(t, m)
	if before.MapID != after.MapID || before.X != after.X || before.Y != after.Y {
		t.Errorf("player moved: before %+v, after %+v", before, after)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	if !state.Controllable(&mem) {
		t.Error("player not controllable after failed SwitchActive")
	}
}

// TestSetLeadAndVoluntarySwitch is S6-5a's proof on a real two-mon party.
//
// The shape: the post_pokeballs fixture carries a level-15 SQUIRTLE and
// five balls. Catch a PIDGEY or RATTATA on Route 1 (S6-3), then:
//
//  1. SetLead(1) — the catch becomes the lead. POSITIVE postcondition:
//     the species that was in slot 1 is now Mons[0].
//  2. Enter a wild battle with the weak catch up front, then SwitchActive
//     back to SQUIRTLE through the battle menu's POKéMON branch. POSITIVE
//     postcondition: state.DecodeBattle's ActiveSpecies changed from the
//     catch to SQUIRTLE mid-battle.
//
// The voluntary switch is the screen Battle never drove before this task:
// A opens FIGHT/ITEM/PKMN/RUN, RIGHT lands the cursor on POKéMON (right
// column, asserted from wTopMenuItemX), A opens the party menu — the
// NORMAL_PARTY_MENU footer "Choose a #MON.", not the forced switch's
// "Bring out" — and the SWITCH/STATS/CANCEL box that follows is answered
// SWITCH.
//
// Slow and stochastic (a hunt plus an encounter), so it is guarded out of
// -short, exactly as TestBattleForcedSwitchAfterFaint:
//
//	POKEMON_RED_ROM=roms/pokemon_red.gb go test ./skill -run TestSetLeadAndVoluntarySwitch -v
func TestSetLeadAndVoluntarySwitch(t *testing.T) {
	if testing.Short() {
		t.Skip("full journey (catch plus a wild battle); run without -short, see the test docs")
	}

	const attempts = 3
	for attempt := 1; attempt <= attempts; attempt++ {
		if setLeadAttempt(t, attempt) {
			return
		}
	}
	t.Fatalf("no voluntary switch observed in %d attempts — read the failure state before changing anything", attempts)
}

// setLeadAttempt runs one full try from a fresh fixture and reports whether
// it observed both postconditions. A missed catch or a failed switch is a
// game outcome, not a bug: log it and let the caller retry from a fresh
// fixture state (a fresh five-ball bag and RNG phase).
func setLeadAttempt(t *testing.T, attempt int) bool {
	t.Helper()
	m := fixture.Load(t, "post_pokeballs")
	// Re-phase the encounter rolls so attempts are independent, as in
	// TestBattleForcedSwitchAfterFaint.
	m.StepFrames((attempt - 1) * 97)
	romData := m.ROM()
	policy := skill.StatAwareMove(romData)

	var mem state.Mem
	state.Snapshot(m, &mem)
	party0 := state.DecodeParty(&mem)
	if party0.Count != 1 || party0.Mons[0].Species != speciesSquirtle {
		t.Fatalf("attempt %d: fixture precondition: %+v, want a lone SQUIRTLE (%#02x)", attempt, party0.Mons, speciesSquirtle)
	}
	if q := bagQty(t, m, skill.ItemPokeBall); q != 5 {
		t.Fatalf("attempt %d: fixture precondition: expected five POKE BALLS, got %d", attempt, q)
	}

	// 1. A live reserve: catch a PIDGEY or RATTATA on Route 1.
	route1, ok := skill.Place("route 1")
	if !ok {
		t.Fatal(`Place "route 1" not found`)
	}
	if _, err := skill.Travel(m, romData, route1, policy, 20); err != nil {
		t.Fatalf("attempt %d: travel to Route 1: %v", attempt, err)
	}
	res, err := skill.Catch(m, romData, []uint8{speciesPidgey, speciesRattata}, policy, 5)
	if err != nil || res.Outcome != skill.OutcomeCaught {
		t.Logf("attempt %d: no catch (outcome=%d balls=%d encounters=%d err=%v); retrying", attempt, res.Outcome, res.BallsThrown, res.Encounters, err)
		return false
	}
	state.Snapshot(m, &mem)
	party1 := state.DecodeParty(&mem)
	if party1.Count != 2 {
		t.Fatalf("attempt %d: post-catch party = %+v, want two members", attempt, party1.Mons)
	}
	caught := party1.Mons[1].Species

	// 2. SetLead(1): the catch becomes the lead. POSITIVE postcondition:
	// the species that was in slot 1 is now Mons[0] — not that SetLead
	// returned nil.
	if err := skill.SetLead(m, 1); err != nil {
		t.Fatalf("attempt %d: SetLead(1): %v", attempt, err)
	}
	state.Snapshot(m, &mem)
	party2 := state.DecodeParty(&mem)
	if party2.Mons[0].Species != caught || party2.Mons[1].Species != speciesSquirtle {
		t.Fatalf("attempt %d: lead did not change: %+v, want [%#02x, %#02x]", attempt, party2.Mons, caught, speciesSquirtle)
	}

	// 3. A wild battle with the weak catch up front, then the VOLUNTARY
	// switch back to SQUIRTLE mid-battle.
	if err := skill.EnterWildBattle(m, 30); err != nil {
		t.Fatalf("attempt %d: EnterWildBattle: %v", attempt, err)
	}
	// The intro text ("A wild PIDGEY appeared!") does not auto-advance, and
	// wBattleMon is loaded only AFTER it — core.asm .playerSendOutFirstMon
	// runs after the intro, so a battle left on that box sits at species
	// 0x00 forever (measured). Advance it with A exactly as Battle's default
	// branch does, and stop the instant the active mon is in RAM — before
	// any A can land on the battle menu.
	for i := 0; i < 200; i++ {
		var s state.Mem
		state.Snapshot(m, &s)
		if b := state.DecodeBattle(&s); b != nil && b.ActiveSpecies != 0 {
			break
		}
		m.Tap(emu.A, 3, 7)
		m.StepFrames(10)
	}
	state.Snapshot(m, &mem)
	bs := state.DecodeBattle(&mem)
	if bs == nil {
		t.Fatalf("attempt %d: no battle in progress after EnterWildBattle", attempt)
	}
	if bs.ActiveSpecies != caught {
		t.Fatalf("attempt %d: battle started with active species %#02x, want the catch (%#02x)", attempt, bs.ActiveSpecies, caught)
	}

	if err := skill.SwitchActive(m, 1); err != nil {
		t.Logf("attempt %d: voluntary switch failed: %v; retrying", attempt, err)
		return false
	}
	state.Snapshot(m, &mem)
	bs2 := state.DecodeBattle(&mem)
	if bs2 == nil {
		t.Fatalf("attempt %d: the battle ended while switching — no active mon to assert", attempt)
	}
	if bs2.ActiveSpecies != speciesSquirtle {
		t.Fatalf("attempt %d: active species after the voluntary switch = %#02x, want SQUIRTLE (%#02x)", attempt, bs2.ActiveSpecies, speciesSquirtle)
	}
	t.Logf("attempt %d: SetLead put species %#02x in slot 0; the voluntary switch put SQUIRTLE (%#02x) active against enemy %#02x",
		attempt, caught, speciesSquirtle, bs2.EnemySpecies)

	// Finish the battle so nothing is left in progress. The outcome is a
	// game result, not part of this task's postcondition: the switch was
	// already asserted on species in RAM.
	if outcome, err := skill.Battle(m, policy); err != nil {
		t.Fatalf("attempt %d: finishing the battle: %v", attempt, err)
	} else {
		t.Logf("attempt %d: battle ended %v", attempt, outcome)
	}
	return true
}
