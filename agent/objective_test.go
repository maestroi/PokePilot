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
	if err := agent.Execute(e, e.ROM(), o); err != nil {
		t.Fatalf("Execute %s: %v", o, err)
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
	if err := agent.Execute(e, e.ROM(), agent.Objective{Kind: agent.KindStarter, Starter: skill.StarterCharmander}); err != nil {
		t.Fatalf("Execute starter: %v", err)
	}

	// The lab girl is map-wide offer data, not adjacent to the post-starter
	// position. KindTalk must approach her before facing and interacting.
	if err := agent.Execute(e, e.ROM(), agent.Objective{Kind: agent.KindTalk, X: 1, Y: 9}); err != nil {
		t.Fatalf("Execute distant talk: %v", err)
	}
}

func TestExecuteTrainCharmanderToOfferedLevel(t *testing.T) {
	e := loadFixture(t)
	for _, objective := range []agent.Objective{
		{Kind: agent.KindStarter, Starter: skill.StarterCharmander},
		{Kind: agent.KindErrand},
		{Kind: agent.KindGoTo, Place: "route 1"},
	} {
		if err := agent.Execute(e, e.ROM(), objective); err != nil {
			t.Fatalf("Execute %q: %v", objective, err)
		}
	}
	err := agent.Execute(e, e.ROM(), agent.Objective{Kind: agent.KindTrain, Level: 12})
	obs := agent.Observe(e, e.ROM())
	if len(obs.Party) == 0 {
		t.Fatal("training removed the party")
	}
	if obs.Party[0].Level >= 12 {
		if err != nil {
			t.Fatalf("train reached level %d but returned %v", obs.Party[0].Level, err)
		}
		return
	}
	if err == nil {
		t.Fatalf("train stopped at level %d but reported success for level 12", obs.Party[0].Level)
	}
	// A short session ends one of two ways: a true blackout (the lead
	// fainted in a battle) or the retreat line (it stopped while the party
	// was still alive). Both are typed consequences, not bare errors — the
	// assertion is about the typing, and from this state the retreat line
	// is the measured ending.
	if !errors.Is(err, skill.ErrBlackedOut) && !errors.Is(err, skill.ErrTrainRetreat) {
		t.Fatalf("incomplete train error = %v, want a typed session ending (blackout or retreat)", err)
	}
}

// TestExecuteGoToPallet runs the starter objective, then walks to Pallet
// Town. MEASURED on main: Place("pallet town") resolves and the walk from
// Oak's lab back to Pallet Town works, so wCurMap must land on 0x00.
func TestExecuteGoToPallet(t *testing.T) {
	e := loadFixture(t)

	if err := agent.Execute(e, e.ROM(), agent.Objective{Kind: agent.KindStarter, Starter: skill.StarterCharmander}); err != nil {
		t.Fatalf("Execute starter: %v", err)
	}

	o := agent.Objective{Kind: agent.KindGoTo, Place: "pallet town"}
	if err := agent.Execute(e, e.ROM(), o); err != nil {
		t.Fatalf("Execute %s: %v", o, err)
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
	err := agent.Execute(e, e.ROM(), o)
	if err == nil {
		t.Fatalf("Execute %s: want error, got nil", o)
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
	if err := agent.Execute(e, e.ROM(), o); err != nil {
		t.Fatalf("Execute %s: %v", o, err)
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

	// Leg 2: through Execute, with Flee set. Viridian City -> Route 1 crosses
	// the grass again; the wiring is proven by the leg arriving without an
	// error (a fallen-back-to-Travel leg that met a wild would fight it, and
	// a lost battle would surface as a blackout error).
	o := agent.Objective{Kind: agent.KindGoTo, Place: "route 1", Flee: true}
	if err := agent.Execute(e, e.ROM(), o); err != nil {
		t.Fatalf("Execute %s: %v", o, err)
	}
	var mem state.Mem
	state.Snapshot(e, &mem)
	if cur := mem.U8(sym.CurMap); cur != 0x0c {
		t.Fatalf("wCurMap = %#04x, want 0x0c (route 1): at (%d,%d)",
			cur, mem.U8(sym.XCoord), mem.U8(sym.YCoord))
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

	o := agent.Objective{Kind: agent.KindUseItem, Item: 0x14, Slot: 0}
	if err := agent.Execute(e, e.ROM(), o); err != nil {
		t.Fatalf("Execute %s: %v", o, err)
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
		{agent.Objective{Kind: agent.KindCatch, Species: 0x7B}, "catch a CATERPIE here"},
		{agent.Objective{Kind: agent.KindBuy, Item: 0x14, Qty: 3}, "buy 3 POTION"},
		{agent.Objective{Kind: agent.KindUseItem, Item: 0x14, Slot: 0}, "use a POTION on party slot 0"},
		{agent.Objective{Kind: agent.KindUseItem, Item: 0x0B, Slot: 2}, "use an ANTIDOTE on party slot 2"},
		{agent.Objective{Kind: 99}, "unknown kind 99"},
	}
	for _, c := range cases {
		if got := c.o.String(); got != c.want {
			t.Errorf("String() = %q, want %q", got, c.want)
		}
	}
}

// TestValidateRejects is the argument safety net: every value a planner can
// supply is checked against a stated range, and an out-of-range or unknown
// value is REJECTED with a typed error — never clamped, never best-matched.
// No ROM needed: Validate is pure.
func TestValidateRejects(t *testing.T) {
	reject := []struct {
		name string
		o    agent.Objective
		want string // substring the error must carry
	}{
		{"train level 0", agent.Objective{Kind: agent.KindTrain, Level: 0}, "out of range"},
		{"train level 101", agent.Objective{Kind: agent.KindTrain, Level: 101}, "out of range"},
		{"catch unknown species", agent.Objective{Kind: agent.KindCatch, Species: 0x00}, "unknown species"},
		{"buy zero quantity", agent.Objective{Kind: agent.KindBuy, Item: 0x14, Qty: 0}, "out of range"},
		{"buy negative quantity", agent.Objective{Kind: agent.KindBuy, Item: 0x14, Qty: -1}, "out of range"},
		{"buy 150 quantity", agent.Objective{Kind: agent.KindBuy, Item: 0x14, Qty: 150}, "out of range"},
		{"buy unknown item", agent.Objective{Kind: agent.KindBuy, Item: 0x00, Qty: 1}, "unknown item"},
		{"unknown starter", agent.Objective{Kind: agent.KindStarter, Starter: skill.Starter(4)}, "unknown starter"},
		{"heal at an unknown place", agent.Objective{Kind: agent.KindHeal, Place: "atlantis"}, "unknown place"},
		{"heal at a mart", agent.Objective{Kind: agent.KindHeal, Place: "viridian mart"}, "not a Pokemon Center"},
		{"use an unknown item", agent.Objective{Kind: agent.KindUseItem, Item: 0x00, Slot: 0}, "unknown item"},
		{"use an item on slot -1", agent.Objective{Kind: agent.KindUseItem, Item: 0x14, Slot: -1}, "out of range"},
		{"use an item on slot 6", agent.Objective{Kind: agent.KindUseItem, Item: 0x14, Slot: 6}, "out of range"},
	}
	for _, c := range reject {
		if err := c.o.Validate(); err == nil {
			t.Errorf("%s: Validate = nil, want an error", c.name)
		} else if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q does not name the problem (%q)", c.name, err, c.want)
		}
	}

	// The boundaries themselves are legal: 1 and 100 train, 1 and 99 buy.
	accept := []agent.Objective{
		{Kind: agent.KindTrain, Level: 1},
		{Kind: agent.KindTrain, Level: 100},
		{Kind: agent.KindCatch, Species: 0x7B},
		{Kind: agent.KindBuy, Item: 0x14, Qty: 1},
		{Kind: agent.KindBuy, Item: 0x14, Qty: 99},
		{Kind: agent.KindStarter, Starter: skill.StarterCharmander},
		{Kind: agent.KindGoTo, Place: "pallet town"}, // no arguments, nothing to check
		{Kind: agent.KindHeal},                       // "" is the center you are standing in
		{Kind: agent.KindHeal, Place: "viridian pokemon center"},
		// The slot boundaries themselves are legal: 0 and 5 (the party caps
		// at six), for a known item.
		{Kind: agent.KindUseItem, Item: 0x14, Slot: 0},
		{Kind: agent.KindUseItem, Item: 0x0B, Slot: 5},
	}
	for _, o := range accept {
		if err := o.Validate(); err != nil {
			t.Errorf("%s: Validate = %v, want nil", o, err)
		}
	}
}

// TestSpeciesAndItemTables pins the argument vocabulary a planner can aim
// at: names resolve to ROM indexes and back, and unknown names do not.
func TestSpeciesAndItemTables(t *testing.T) {
	if id, ok := agent.SpeciesByName("CATERPIE"); !ok || id != 0x7B {
		t.Errorf("SpeciesByName(CATERPIE) = %#04x, %v; want 0x7B", id, ok)
	}
	if name, ok := agent.SpeciesName(0x24); !ok || name != "pidgey" {
		t.Errorf("SpeciesName(0x24) = %q, %v; want pidgey", name, ok)
	}
	// The whole ROM roster, not a hand-picked subset: a species the table
	// omits is a species the planner is not allowed to want, and the map's
	// wild table names plenty the old 28-entry list did not contain.
	if _, ok := agent.SpeciesByName("snorlax"); !ok {
		t.Error("SpeciesByName(snorlax) did not resolve; all 151 should be nameable")
	}
	if id, ok := agent.SpeciesByName("fearow"); !ok || id != 0x23 {
		t.Errorf("SpeciesByName(fearow) = %#04x, %v; want 0x23 (the old table misspelled this one)", id, ok)
	}
	if n := agent.SpeciesCount(); n != 151 {
		t.Errorf("species table holds %d names, want 151", n)
	}
	if _, ok := agent.SpeciesByName("mewthree"); ok {
		t.Error("SpeciesByName(mewthree) resolved; it is not a species")
	}
	if id, ok := agent.ItemByName("potion"); !ok || id != 0x14 {
		t.Errorf("ItemByName(potion) = %#04x, %v; want 0x14", id, ok)
	}
	if name, ok := agent.ItemName(0x0B); !ok || name != "antidote" {
		t.Errorf("ItemName(0x0B) = %q, %v; want antidote", name, ok)
	}
	if _, ok := agent.ItemByName("master ball"); ok {
		t.Error("ItemByName(master ball) resolved; it is not in the table")
	}
}
