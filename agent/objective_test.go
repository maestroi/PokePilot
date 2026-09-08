package agent_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// loadFixture restores the reds_bedroom fixture: a freshly booted game at a
// controllable overworld (map 0x26, Red's bedroom). It skips when
// POKEMON_RED_ROM is not set.
func loadFixture(t *testing.T) *emu.Emu {
	t.Helper()
	return fixture.Load(t, "reds_bedroom")
}

// TestExecuteStarter runs the KindStarter objective from a fresh boot and
// checks the party gained the mon the objective asked for.
func TestExecuteStarter(t *testing.T) {
	e := loadFixture(t)

	const speciesCharmander uint8 = 0xB0 // ROM pokemon index, not dex number
	o := agent.Objective{Kind: agent.KindStarter, Starter: skill.StarterCharmander}
	got, err := agent.Execute(e, e.ROM(), o)
	if err != nil {
		t.Fatalf("Execute %s: %v", o, err)
	}
	if got.Objective != o {
		t.Fatalf("result Objective = %v, want %v", got.Objective, o)
	}
	if got.Outcome != agent.OutcomeCompleted {
		t.Fatalf("result Outcome = %q, want completed", got.Outcome)
	}

	var mem state.Mem
	state.Snapshot(e, &mem)
	party := state.DecodeParty(&mem)
	if party.Count < 1 {
		t.Fatalf("party count = %d, want >= 1", party.Count)
	}
	if party.Mons[0].Species != speciesCharmander {
		t.Fatalf("lead species = %#04x, want %#04x (charmander): the objective said charmander",
			party.Mons[0].Species, speciesCharmander)
	}
}

func TestExecuteTalkWalksToMapObject(t *testing.T) {
	e := loadFixture(t)
	if _, err := agent.Execute(e, e.ROM(), agent.Objective{Kind: agent.KindStarter, Starter: skill.StarterCharmander}); err != nil {
		t.Fatalf("Execute starter: %v", err)
	}

	// The lab girl is map-wide offer data, not adjacent to the post-starter
	// position. KindTalk must approach her before facing and interacting.
	if _, err := agent.Execute(e, e.ROM(), agent.Objective{Kind: agent.KindTalk, X: 1, Y: 9}); err != nil {
		t.Fatalf("Execute distant talk: %v", err)
	}
}

// TestExecuteTalkCrossesTallGrass is the end-to-end proof of S10-2: a talk
// whose approach crosses tall grass must complete, with the encounters on
// the way fled rather than fought. Before the fix the approach was a raw
// walk that aborts on the first wild battle ("skill: battle interrupted
// movement"), and the objective failed on every retry until the run's
// failure budget ran out — make run-llm died at round 18 on exactly this
// objective (talk at (5,24) on Route 1), and three farm runs died on its
// neighbours. The postconditions are both halves: the player must END UP on
// Route 1 beside the NPC (the talk happened where it was offered, not in a
// town after a blackout), and the lead's HP must be unchanged (the approach
// fled; a fought encounter would have damaged it).
func TestExecuteTalkCrossesTallGrass(t *testing.T) {
	e := fixture.Load(t, "route1")

	var mem state.Mem
	state.Snapshot(e, &mem)
	if cur := mem.U8(sym.CurMap); cur != 0x0c {
		t.Fatalf("fixture start = map %#04x, want 0x0c (route 1)", cur)
	}
	hpBefore := state.DecodeParty(&mem).Mons[0].HP

	// The NPC at (5,24) is the route's talk target; the approach from the
	// fixture's (5,14) crosses tall grass.
	if _, err := agent.Execute(e, e.ROM(), agent.Objective{Kind: agent.KindTalk, X: 5, Y: 24}); err != nil {
		t.Fatalf("Execute talk across grass: %v", err)
	}

	state.Snapshot(e, &mem)
	if cur := mem.U8(sym.CurMap); cur != 0x0c {
		t.Fatalf("after talk = map %#04x, want 0x0c (route 1): the approach must not have blacked out", cur)
	}
	x, y := int(mem.U8(sym.XCoord)), int(mem.U8(sym.YCoord))
	beside := (x == 5 && (y == 23 || y == 25)) || (y == 24 && (x == 4 || x == 6))
	if !beside {
		t.Fatalf("after talk = (%d,%d), want orthogonally adjacent to (5,24): the talk must have happened beside the NPC", x, y)
	}
	if hp := state.DecodeParty(&mem).Mons[0].HP; hp != hpBefore {
		t.Fatalf("lead HP = %d after, was %d before: the approach fought an encounter instead of fleeing it", hp, hpBefore)
	}
}

func TestExecuteTrainCharmanderToOfferedLevel(t *testing.T) {
	e := loadFixture(t)
	for _, objective := range []agent.Objective{
		{Kind: agent.KindStarter, Starter: skill.StarterCharmander},
		{Kind: agent.KindErrand},
		{Kind: agent.KindGoTo, Place: "route 1"},
	} {
		if _, err := agent.Execute(e, e.ROM(), objective); err != nil {
			t.Fatalf("Execute %q: %v", objective, err)
		}
	}
	got, err := agent.Execute(e, e.ROM(), agent.Objective{Kind: agent.KindTrain, Level: 12})
	if got.Train == nil {
		t.Fatal("structured Train result = nil")
	}
	obs := agent.Observe(e, e.ROM())
	if len(obs.Party) == 0 {
		t.Fatal("training removed the party")
	}
	if got.Train.EndLevel != int(obs.Party[0].Level) {
		t.Fatalf("Train.EndLevel = %d, observation lead level = %d", got.Train.EndLevel, obs.Party[0].Level)
	}
	if obs.Party[0].Level >= 12 {
		if err != nil {
			t.Fatalf("train reached level %d but returned %v", obs.Party[0].Level, err)
		}
		if got.Outcome != agent.OutcomeCompleted || !got.Train.Reached {
			t.Fatalf("completed train result = %+v", got)
		}
		return
	}
	if err == nil {
		t.Fatalf("train stopped at level %d but reported success for level 12", obs.Party[0].Level)
	}
	if got.Outcome != agent.OutcomeBlocked {
		t.Fatalf("incomplete train Outcome = %q, want blocked", got.Outcome)
	}
	// A short session ends one of two ways in this measured fixture: a true
	// blackout (the lead fainted in a battle) or the retreat line (it stopped
	// while the party was still alive). Both keep the complete TrainResult.
	if !errors.Is(err, skill.ErrBlackedOut) && !errors.Is(err, skill.ErrTrainRetreat) {
		t.Fatalf("incomplete train error = %v, want a typed session ending (blackout or retreat)", err)
	}
}

// TestExecuteGoToPallet runs the starter objective, then walks to Pallet
// Town. Besides the RAM check, it pins the public structured-result contract:
// exact final destination plus the full TravelResult survive Execute.
func TestExecuteGoToPallet(t *testing.T) {
	e := loadFixture(t)

	if _, err := agent.Execute(e, e.ROM(), agent.Objective{Kind: agent.KindStarter, Starter: skill.StarterCharmander}); err != nil {
		t.Fatalf("Execute starter: %v", err)
	}

	o := agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"}
	got, err := agent.Execute(e, e.ROM(), o)
	if err != nil {
		t.Fatalf("Execute %s: %v", o, err)
	}
	if got.Objective != o {
		t.Fatalf("result Objective = %v, want %v", got.Objective, o)
	}
	if got.Outcome != agent.OutcomeCompleted {
		t.Fatalf("result Outcome = %q, want completed", got.Outcome)
	}
	if got.Travel == nil {
		t.Fatal("result Travel = nil")
	}
	dest, ok := skill.Place("pallet town")
	if !ok {
		t.Fatal("Place(pallet town) did not resolve")
	}
	if got.Final.Map != dest.Map || got.Final.X != dest.X || got.Final.Y != dest.Y || !got.Final.Controllable || got.Final.InBattle {
		t.Fatalf("result Final = map %02x at (%d,%d), controllable=%v battle=%v; want map %02x at (%d,%d) controllable overworld",
			got.Final.Map, got.Final.X, got.Final.Y, got.Final.Controllable, got.Final.InBattle,
			dest.Map, dest.X, dest.Y)
	}

	var mem state.Mem
	state.Snapshot(e, &mem)
	if cur := mem.U8(sym.CurMap); cur != 0x00 {
		t.Fatalf("wCurMap = %#04x, want 0x00 (pallet town): at (%d,%d)",
			cur, mem.U8(sym.XCoord), mem.U8(sym.YCoord))
	}
}

// TestExecuteUnknownPlace checks that a nonsense place name is an error that
// names the place, not a silent fallback to a default.
func TestExecuteUnknownPlace(t *testing.T) {
	e := loadFixture(t)

	const name = "atlantis"
	o := agent.Objective{Kind: agent.KindGoTo, Place: name}
	got, err := agent.Execute(e, e.ROM(), o)
	if err == nil {
		t.Fatalf("Execute %s: want error, got nil", o)
	}
	if got.Objective != o || got.Outcome == agent.OutcomeCompleted {
		t.Fatalf("structured failed result = %+v", got)
	}
	if !strings.Contains(err.Error(), name) {
		t.Fatalf("error does not name the place: %v", err)
	}
}

// TestExecuteHealTravelsToTheNamedCenter drives the path Offer hands a hurt
// party in the field: one objective that walks to a named center and heals
// there. The postconditions are both halves — the player must END UP on the
// center map, and every mon must be at full HP, so a travel that stopped
// short cannot pass as a heal.
func TestExecuteHealTravelsToTheNamedCenter(t *testing.T) {
	if testing.Short() {
		t.Skip("full journey; not part of the -short gate")
	}
	e := fixture.Load(t, "post_errand") // Viridian City, outdoors

	o := agent.Objective{Kind: agent.KindHeal, Place: "viridian pokemon center"}
	got, err := agent.Execute(e, e.ROM(), o)
	if err != nil {
		t.Fatalf("Execute %s: %v", o, err)
	}
	if got.Outcome != agent.OutcomeCompleted || got.Travel == nil {
		t.Fatalf("structured heal result = %+v", got)
	}

	var mem state.Mem
	state.Snapshot(e, &mem)
	if cur := mem.U8(sym.CurMap); cur != 0x29 {
		t.Fatalf("wCurMap = %#04x, want 0x29 (viridian pokecenter): at (%d,%d)",
			cur, mem.U8(sym.XCoord), mem.U8(sym.YCoord))
	}
	for i, mon := range state.DecodeParty(&mem).Mons {
		if mon.HP != mon.MaxHP {
			t.Errorf("party[%d] HP = %d/%d after the heal, want full", i, mon.HP, mon.MaxHP)
		}
	}
	// The heal is also the checkpoint: SetLastBlackoutMap runs on YES to the
	// nurse, so a blackout after this lands in Viridian City, not Pallet Town.
	if got := mem.U8(sym.LastBlackoutMap); got != 0x01 {
		t.Errorf("wLastBlackoutMap = %#04x after healing in Viridian, want 0x01 (VIRIDIAN_CITY)", got)
	}
}

// TestExecuteGoToFleesWildEncounters is the end-to-end proof of the flee
// argument: a journey leg through grass executed with Flee set must run
// wild encounters instead of fighting them. The Execute call proves the
// wiring (a leg that fell back to Travel would fight, and a leg that met a
// wild and lost it would blackout); the TravelResult counters prove the
// policy — "it did not error" is satisfied by a run that fought everything,
// which is the behaviour being replaced. Fixture replays are bit-identical,
// so the encounter outcome on these legs is deterministic.
func TestExecuteGoToFleesWildEncounters(t *testing.T) {
	if testing.Short() {
		t.Skip("full journey; not part of the -short gate")
	}
	e := fixture.Load(t, "post_starter")

	// Leg 1: post_starter -> Viridian City crosses Route 1's and Route 2's
	// tall grass; S8-7 measured one wild on this leg. Driven directly so the
	// TravelResult counters can be read: a wild met on it must show up as a
	// flee, never as a battle.
	dest, ok := skill.Place("viridian city")
	if !ok {
		t.Fatal("Place(viridian city) did not resolve")
	}
	res, err := skill.TravelFlee(e, e.ROM(), dest, skill.StatAwareMove(e.ROM()), 20)
	if err != nil {
		t.Fatalf("TravelFlee to viridian city: %v (flees=%d battles=%d blackedOut=%v)", err, res.Flees, res.Battles, res.BlackedOut)
	}
	if res.Flees == 0 {
		t.Errorf("Flees = 0, want > 0: the leg met no wild to flee")
	}
	if res.Battles != 0 {
		t.Errorf("Battles = %d, want 0: a wild was fought instead of fled", res.Battles)
	}

	// Leg 2: through Execute, with Flee set. The returned TravelResult is the
	// proof that the public objective boundary no longer flattens the journey.
	o := agent.Objective{Kind: agent.KindGoTo, Place: "route 1", Flee: true}
	got, err := agent.Execute(e, e.ROM(), o)
	if err != nil {
		t.Fatalf("Execute %s: %v", o, err)
	}
	if got.Travel == nil {
		t.Fatal("Execute Travel = nil")
	}
	if got.Travel.Battles != 0 {
		t.Errorf("Execute Travel.Battles = %d, want 0 with flee policy", got.Travel.Battles)
	}
	var mem state.Mem
	state.Snapshot(e, &mem)
	if cur := mem.U8(sym.CurMap); cur != 0x0c {
		t.Fatalf("wCurMap = %#04x, want 0x0c (route 1): at (%d,%d)",
			cur, mem.U8(sym.XCoord), mem.U8(sym.YCoord))
	}
}

// TestExecuteCatchMissIsAFailure is the S10-2d pin for the KindCatch
// branch: a hunt that ended without the species in the party is a FAILURE,
// not a quietly recorded done round. The route1 fixture's bag holds no
// balls, so the hunt runs out of balls after the first non-target
// encounter — a deterministic miss on replay, and the exact shape a
// planner sees when the grass does not cooperate. Before the fix this
// outcome returned nil: the round was recorded DONE, Knowledge counted it
// in Completed, and the next round's menu line grew a "(done 1x)" for a
// catch that never happened.
func TestExecuteCatchMissIsAFailure(t *testing.T) {
	e := fixture.Load(t, "route1")

	o := agent.Objective{Kind: agent.KindCatch, Species: agent.SpeciesID("pidgey")}
	got, err := agent.Execute(e, e.ROM(), o)
	if err == nil {
		t.Fatalf("Execute %s: want an error (the hunt did not end with a PIDGEY in the party), got nil", o)
	}
	if got.Outcome != agent.OutcomeBlocked {
		t.Fatalf("missed catch Outcome = %q, want blocked", got.Outcome)
	}
	if !strings.Contains(err.Error(), "no PIDGEY caught") {
		t.Fatalf("error = %v, want the missed-hunt text naming the species and the outcome", err)
	}

	// The failure left the world where it can continue: the party did not
	// grow, and the player is still standing on Route 1.
	var mem state.Mem
	state.Snapshot(e, &mem)
	if party := state.DecodeParty(&mem); party.Count != 1 {
		t.Fatalf("party count = %d, want 1 (the hunt added no mon)", party.Count)
	}
	if cur := mem.U8(sym.CurMap); cur != 0x0c {
		t.Fatalf("wCurMap = %#04x, want 0x0c (route 1): the miss did not black out", cur)
	}
}

// TestExecuteUseItemHealsTheTarget is the end-to-end proof of KindUseItem:
// from a fixture, damage taken, Execute the objective, and assert from RAM
// that the target's HP ROSE. A returned nil is not evidence: UseFieldItem
// enforces its own postcondition (ErrFieldItemNoEffect), but this test
// re-reads the party independently, so a regression that made the skill
// return nil without healing cannot pass here.
func TestExecuteUseItemHealsTheTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("emulator journey; not part of the -short gate")
	}
	e := fixture.Load(t, "viridian_mart") // post-errand: start menu carries the pokedex entry
	policy := skill.StatAwareMove(e.ROM())

	// The hidden POTION at (1,18) on map 0x33 is the only potion reachable
	// this early in the story (S8-5's setup): no shop before Pewter stocks
	// it. Hidden events fire on A while FACING the tile, not on stepping on
	// it, so walk to (1,17) and face down at it.
	forest, ok := skill.Place("viridian forest")
	if !ok {
		t.Fatal(`Place "viridian forest" not found`)
	}
	if err := travelWithBlackouts(t, e, policy, forest); err != nil {
		t.Fatalf("travel to the forest: %v", err)
	}
	if err := travelWithBlackouts(t, e, policy, skill.Destination{Map: 0x33, X: 1, Y: 17}); err != nil {
		t.Fatalf("travel to (1,17) on the forest corridor: %v", err)
	}
	if err := skill.Face(e, 1, 18); err != nil {
		t.Fatalf("face the hidden item tile: %v", err)
	}
	if _, err := skill.Talk(e); err != nil {
		t.Fatalf("take the hidden potion: %v", err)
	}
	var mem state.Mem
	state.Snapshot(e, &mem)
	if qty := bagQuantity(&mem, 0x14); qty != 1 {
		t.Fatalf("precondition: bag POTION = %d after the hidden item, want 1", qty)
	}

	// A wild battle on Route 1 damages the lead. A loss blackouts — the
	// party is fully healed and respawned in Pallet Town — so the leg back
	// to Route 1 is paid again; bounded at four battles, far more than the
	// post-errand lead needs against Pidgey.
	r1, ok := skill.Place("route 1")
	if !ok {
		t.Fatal(`Place "route 1" not found`)
	}
	if err := travelWithBlackouts(t, e, policy, r1); err != nil {
		t.Fatalf("travel to Route 1: %v", err)
	}
	for tries := 0; tries < 4; tries++ {
		state.Snapshot(e, &mem)
		if lead := state.DecodeParty(&mem).Mons[0]; lead.HP > 0 && lead.HP < lead.MaxHP {
			break // damaged and alive
		}
		if e.Peek8(sym.CurMap) != r1.Map {
			if err := travelWithBlackouts(t, e, policy, r1); err != nil {
				t.Fatalf("travel back to Route 1: %v", err)
			}
		}
		if err := skill.EnterWildBattle(e, 3); err != nil {
			t.Fatalf("enter wild battle (try %d): %v", tries+1, err)
		}
		outcome, err := skill.Battle(e, policy)
		if err != nil {
			t.Fatalf("battle (try %d): %v", tries+1, err)
		}
		if outcome == state.ResultLost {
			t.Logf("lost the damage battle (try %d); settling the blackout", tries+1)
			settleRespawn(t, e)
		}
	}
	state.Snapshot(e, &mem)
	before := state.DecodeParty(&mem).Mons[0]
	if before.HP == 0 || before.HP >= before.MaxHP {
		t.Fatalf("precondition: lead not damaged (HP %d/%d)", before.HP, before.MaxHP)
	}

	o := agent.Objective{Kind: agent.KindUseItem, Item: agent.ItemID("potion"), Slot: 0}
	got, err := agent.Execute(e, e.ROM(), o)
	if err != nil {
		t.Fatalf("Execute %s: %v", o, err)
	}
	if got.Outcome != agent.OutcomeCompleted {
		t.Fatalf("UseItem Outcome = %q, want completed", got.Outcome)
	}

	// The postcondition is HP RISING, read from RAM — not the nil error.
	state.Snapshot(e, &mem)
	after := state.DecodeParty(&mem).Mons[0]
	if after.HP <= before.HP {
		t.Fatalf("postcondition: lead HP did not rise: %d -> %d (max %d)", before.HP, after.HP, after.MaxHP)
	}
	if qty := bagQuantity(&mem, 0x14); qty != 0 {
		t.Errorf("postcondition: bag POTION = %d, want 0 (the one was used)", qty)
	}
	if !state.Controllable(&mem) {
		t.Error("postcondition: player is not controllable after using the item")
	}
}

// travelWithBlackouts runs one Travel leg with bounded blackout retries:
// a lost wild battle on a grass leg is an ordinary outcome (the party is
// fully healed and respawned in the last town), and Travel resumes from
// there after the respawn warp settles. The same pattern S8-5's test uses.
func travelWithBlackouts(t *testing.T, e *emu.Emu, policy skill.MovePolicy, dest skill.Destination) error {
	t.Helper()
	for attempt := 0; ; attempt++ {
		_, err := skill.Travel(e, e.ROM(), dest, policy, 20)
		if err == nil || attempt >= 3 || !errors.Is(err, skill.ErrBlackedOut) {
			return err
		}
		t.Logf("blackout on the way (attempt %d); settling the respawn", attempt+1)
		settleRespawn(t, e)
	}
}

// settleRespawn steps until the party is whole and the player controllable:
// the respawn warp's postcondition.
func settleRespawn(t *testing.T, e *emu.Emu) {
	t.Helper()
	for i := 0; i < 200; i++ {
		e.StepFrames(25)
		var mem state.Mem
		state.Snapshot(e, &mem)
		if !state.Controllable(&mem) {
			continue
		}
		lead := state.DecodeParty(&mem).Mons[0]
		if int(lead.HP) == int(lead.MaxHP) && lead.Status == 0 {
			return
		}
	}
	t.Fatal("settleRespawn: the respawn warp did not land within 5000 frames")
}

// bagQuantity reads one item's count straight out of the decoded bag.
func bagQuantity(mem *state.Mem, id uint8) int {
	for _, it := range state.DecodeInventory(mem).Items {
		if it.ID == id {
			return int(it.Quantity)
		}
	}
	return 0
}

// TestString is the one test that needs no ROM: it pins the plain, stable
// one-line descriptions a planner will see.
func TestString(t *testing.T) {
	cases := []struct {
		o    agent.Objective
		want string
	}{
		{agent.Objective{Kind: agent.KindGoTo, Place: "viridian pokemon center"}, "go to viridian pokemon center"},
		{agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"}, "go to pallet town"},
		{agent.Objective{Kind: agent.KindGoTo, Place: "mt moon 1f", Flee: true}, "go to mt moon 1f, fleeing wild battles"},
		{agent.Objective{Kind: agent.KindHeal, Place: "viridian pokemon center", Flee: true}, "heal the party at VIRIDIAN POKEMON CENTER, fleeing wild battles"},
		{agent.Objective{Kind: agent.KindTalk, X: 3, Y: 1}, "talk at (3,1)"},
		{agent.Objective{Kind: agent.KindStarter, Starter: skill.StarterCharmander}, "take the charmander starter"},
		{agent.Objective{Kind: agent.KindStarter, Starter: skill.StarterSquirtle}, "take the squirtle starter"},
		{agent.Objective{Kind: agent.KindErrand}, "deliver oak's parcel"},
		{agent.Objective{Kind: agent.KindTrain, Level: 10}, "train the lead to level 10"},
		{agent.Objective{Kind: agent.KindHeal}, "heal the party"},
		{agent.Objective{Kind: agent.KindHeal, Place: "viridian pokemon center"}, "heal the party at VIRIDIAN POKEMON CENTER"},
		{agent.Objective{Kind: agent.KindGym}, "beat the gym leader here"},
		{agent.Objective{Kind: agent.KindCatch, Species: agent.SpeciesID("caterpie")}, "catch a CATERPIE here"},
		{agent.Objective{Kind: agent.KindBuy, Item: agent.ItemID("potion"), Qty: 3}, "buy 3 POTION"},
		{agent.Objective{Kind: agent.KindUseItem, Item: agent.ItemID("potion"), Slot: 0}, "use a POTION on party slot 0"},
		{agent.Objective{Kind: agent.KindUseItem, Item: agent.ItemID("antidote"), Slot: 2}, "use an ANTIDOTE on party slot 2"},
		{agent.Objective{Kind: 99}, "unknown kind 99"},
	}
	for _, c := range cases {
		if got := c.o.String(); got != c.want {
			t.Errorf("String() = %q, want %q", got, c.want)
		}
	}
}

// TestValidateRejects pins only game-agnostic objective shape/range rules.
// Concrete Pokémon Red vocabulary is adapter-owned: generic validation must
// not know which species/items/places exist in one game.
func TestValidateRejects(t *testing.T) {
	reject := []struct {
		name string
		o    agent.Objective
		want string
	}{
		{"empty go-to place", agent.Objective{Kind: agent.KindGoTo}, "empty place id"},
		{"train level 0", agent.Objective{Kind: agent.KindTrain, Level: 0}, "out of range"},
		{"train level 101", agent.Objective{Kind: agent.KindTrain, Level: 101}, "out of range"},
		{"catch empty species", agent.Objective{Kind: agent.KindCatch}, "empty species id"},
		{"buy zero quantity", agent.Objective{Kind: agent.KindBuy, Item: agent.ItemID("potion"), Qty: 0}, "out of range"},
		{"buy negative quantity", agent.Objective{Kind: agent.KindBuy, Item: agent.ItemID("potion"), Qty: -1}, "out of range"},
		{"buy 150 quantity", agent.Objective{Kind: agent.KindBuy, Item: agent.ItemID("potion"), Qty: 150}, "out of range"},
		{"buy empty item", agent.Objective{Kind: agent.KindBuy, Qty: 1}, "empty item id"},
		{"unknown starter", agent.Objective{Kind: agent.KindStarter, Starter: skill.Starter(4)}, "unknown starter"},
		{"use empty item", agent.Objective{Kind: agent.KindUseItem, Slot: 0}, "empty item id"},
		{"use an item on slot -1", agent.Objective{Kind: agent.KindUseItem, Item: agent.ItemID("potion"), Slot: -1}, "out of range"},
		{"use an item on slot 6", agent.Objective{Kind: agent.KindUseItem, Item: agent.ItemID("potion"), Slot: 6}, "out of range"},
	}
	for _, c := range reject {
		if err := c.o.Validate(); err == nil {
			t.Errorf("%s: Validate = nil, want an error", c.name)
		} else if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q does not name the problem (%q)", c.name, err, c.want)
		}
	}

	// Game-specific names are intentionally legal at this layer. The Red
	// adapter rejects unsupported Red vocabulary before execution.
	for _, o := range []agent.Objective{
		{Kind: agent.KindCatch, Species: agent.SpeciesID("mewthree")},
		{Kind: agent.KindBuy, Item: agent.ItemID("mystery item"), Qty: 1},
		{Kind: agent.KindHeal, Place: "atlantis"},
	} {
		if err := o.Validate(); err != nil {
			t.Errorf("portable Validate rejected adapter-owned vocabulary for %s: %v", o, err)
		}
	}

	// The portable boundaries themselves are legal.
	accept := []agent.Objective{
		{Kind: agent.KindTrain, Level: 1},
		{Kind: agent.KindTrain, Level: 100},
		{Kind: agent.KindCatch, Species: agent.SpeciesID("caterpie")},
		{Kind: agent.KindBuy, Item: agent.ItemID("potion"), Qty: 1},
		{Kind: agent.KindBuy, Item: agent.ItemID("potion"), Qty: 99},
		{Kind: agent.KindStarter, Starter: skill.StarterCharmander},
		{Kind: agent.KindGoTo, Place: "pallet town"},
		{Kind: agent.KindHeal},
		{Kind: agent.KindHeal, Place: "viridian pokemon center"},
		{Kind: agent.KindUseItem, Item: agent.ItemID("potion"), Slot: 0},
		{Kind: agent.KindUseItem, Item: agent.ItemID("antidote"), Slot: 5},
	}
	for _, o := range accept {
		if err := o.Validate(); err != nil {
			t.Errorf("%s: Validate = %v, want nil", o, err)
		}
	}
}

// TestSpeciesAndItemTables separates the planner-facing semantic vocabulary
// from Pokémon Red's raw reverse lookup. Name resolution returns portable IDs;
// raw bytes remain an implementation detail of the Red adapter.
func TestSpeciesAndItemTables(t *testing.T) {
	if id, ok := agent.SpeciesByName("CATERPIE"); !ok || id != agent.SpeciesID("caterpie") {
		t.Errorf("SpeciesByName(CATERPIE) = %q, %v; want caterpie", id, ok)
	}
	if name, ok := agent.SpeciesName(0x24); !ok || name != "pidgey" {
		t.Errorf("SpeciesName(0x24) = %q, %v; want pidgey", name, ok)
	}
	if id, ok := agent.SpeciesByName("snorlax"); !ok || id != agent.SpeciesID("snorlax") {
		t.Errorf("SpeciesByName(snorlax) = %q, %v; want snorlax", id, ok)
	}
	if id, ok := agent.SpeciesByName("fearow"); !ok || id != agent.SpeciesID("fearow") {
		t.Errorf("SpeciesByName(fearow) = %q, %v; want fearow", id, ok)
	}
	if n := agent.SpeciesCount(); n != 151 {
		t.Errorf("species table holds %d names, want 151", n)
	}
	if _, ok := agent.SpeciesByName("mewthree"); ok {
		t.Error("SpeciesByName(mewthree) resolved; it is not a species")
	}
	if id, ok := agent.ItemByName("potion"); !ok || id != agent.ItemID("potion") {
		t.Errorf("ItemByName(potion) = %q, %v; want potion", id, ok)
	}
	if name, ok := agent.ItemName(0x0B); !ok || name != "antidote" {
		t.Errorf("ItemName(0x0B) = %q, %v; want antidote", name, ok)
	}
	if _, ok := agent.ItemByName("master ball"); ok {
		t.Error("ItemByName(master ball) resolved; it is not in the table")
	}
}
