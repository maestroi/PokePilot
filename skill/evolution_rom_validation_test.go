package skill_test

import (
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

const (
	speciesJigglypuff uint8 = 0x64
	speciesWigglytuff uint8 = 0x65
	itemMoonStone     uint8 = 0x0A
)

func requireDexOwned(t *testing.T, romData []byte, mem *state.Mem, species uint8) {
	t.Helper()
	dex, err := rom.InternalSpeciesDexNumber(romData, species)
	if err != nil {
		t.Fatalf("dex number for species %#02x: %v", species, err)
	}
	for _, owned := range skill.RedAddresses().DecodePokedex(mem).Owned {
		if owned == dex {
			return
		}
	}
	t.Fatalf("species %#02x (dex %d) is in the evolved party but its Pokédex-owned bit is not set", species, dex)
}

func trainRoute1ToLevel(t *testing.T, e *emu.Emu, romData []byte, target int) {
	t.Helper()
	policy := skill.StatAwareMove(romData)
	grass := route1Grass(t, romData)
	const totalCap = 220
	totalBattles := 0
	for segment := 1; totalBattles < totalCap; segment++ {
		if _, err := skill.Travel(e, romData, grass, policy, 6); err != nil {
			t.Fatalf("Travel to Route 1 (segment %d): %v", segment, err)
		}
		res, err := skill.Train(e, romData, target, policy, 100)
		if err != nil {
			t.Fatalf("Train to %d (segment %d): %v", target, segment, err)
		}
		totalBattles += res.Battles
		if res.Reached {
			return
		}
		if res.Retreated {
			center, ok := skill.Place("viridian pokemon center")
			if !ok {
				t.Fatal("Place(viridian pokemon center) did not resolve")
			}
			if _, err := fixture.Travel(e, center, policy, 6); err != nil {
				t.Fatalf("travel to Viridian center (segment %d): %v", segment, err)
			}
			if err := skill.Heal(e); err != nil {
				t.Fatalf("heal after retreat (segment %d): %v", segment, err)
			}
		}
	}
	t.Fatalf("did not reach level %d within %d battles", target, totalCap)
}

// TestDexLevelEvolutionSetsPokedexOwned is the ROM-backed #43 acceptance proof
// for a normal level evolution. It starts from the real post-pokeballs fixture,
// levels Squirtle through its level-16 evolution, then requires both the party
// species and Wartortle's Pokédex-owned bit. A cutscene completing without that
// persistent ownership evidence is not success for Dex mode.
func TestDexLevelEvolutionSetsPokedexOwned(t *testing.T) {
	if testing.Short() {
		t.Skip("ROM-backed evolution journey; run without -short with POKEMON_RED_ROM")
	}
	e := fixture.Load(t, "post_pokeballs")
	romData := e.ROM()

	var mem state.Mem
	state.Snapshot(e, &mem)
	party := skill.RedAddresses().DecodeParty(&mem)
	if party.Count != 1 || party.Mons[0].Species != speciesSquirtle || party.Mons[0].Level < 15 {
		t.Fatalf("fixture precondition: party=%+v, want lone Squirtle lv>=15", party.Mons)
	}

	trainRoute1ToLevel(t, e, romData, 16)
	state.Snapshot(e, &mem)
	party = skill.RedAddresses().DecodeParty(&mem)
	if party.Count != 1 || party.Mons[0].Species != speciesWartortle {
		t.Fatalf("level evolution result party=%+v, want Wartortle (%#02x)", party.Mons, speciesWartortle)
	}
	requireDexOwned(t, romData, &mem, speciesWartortle)
}

// TestDexMoonStoneEvolutionSetsPokedexOwned is the finite-item counterpart.
// It catches a real Route 3 Jigglypuff, picks up Mt. Moon 1F's real Moon Stone,
// uses the production item-evolution controller, and requires Wigglytuff's
// persistent Pokédex-owned bit. The hunt is save-state phase retried only to
// keep the test deterministic enough for CI; every successful path is ordinary
// ROM gameplay and the stone is genuinely consumed.
func TestDexMoonStoneEvolutionSetsPokedexOwned(t *testing.T) {
	if testing.Short() {
		t.Skip("ROM-backed catch + Moon Stone evolution journey; run without -short with POKEMON_RED_ROM")
	}
	e := fixture.Load(t, "post_boulder")
	romData := e.ROM()
	policy := skill.StatAwareMove(romData)

	route3, ok := skill.Place("route 3")
	if !ok {
		t.Fatal("Place(route 3) did not resolve")
	}
	if _, err := fixture.Travel(e, route3, policy, 30); err != nil {
		t.Fatalf("travel to Route 3: %v", err)
	}
	if _, err := skill.EnsureProgressionPokeBalls(e, romData, policy); err != nil {
		t.Fatalf("stock Poké Balls for Jigglypuff: %v", err)
	}

	huntState, err := e.SaveState()
	if err != nil {
		t.Fatalf("save pre-hunt state: %v", err)
	}
	phases := [...]int{0, 29, 61, 97, 149, 211}
	caught := false
	for attempt, phase := range phases {
		if attempt > 0 {
			if err := e.LoadState(huntState); err != nil {
				t.Fatalf("restore Route 3 hunt attempt %d: %v", attempt+1, err)
			}
			e.StepFrames(phase)
		}
		res, catchErr := skill.Catch(e, romData, []uint8{speciesJigglypuff}, policy, 10)
		if catchErr == nil && res.Outcome == skill.OutcomeCaught && res.Species == speciesJigglypuff {
			caught = true
			break
		}
	}
	if !caught {
		t.Fatalf("did not catch Route 3 Jigglypuff across %d deterministic hunt phases", len(phases))
	}

	// Mt. Moon 1F object data places MOON_STONE at (2,2). Stand beside it,
	// then use the ordinary Pickup path so bag-space, dialogue and bag delta are
	// verified exactly as they are in a real run.
	if _, err := fixture.Travel(e, skill.Destination{Map: 0x3B, X: 3, Y: 2}, policy, 50); err != nil {
		t.Fatalf("travel to Mt. Moon Moon Stone: %v", err)
	}
	if err := skill.Pickup(e, romData, 2, 2, itemMoonStone, policy); err != nil {
		t.Fatalf("pick up Moon Stone: %v", err)
	}

	var mem state.Mem
	state.Snapshot(e, &mem)
	party := skill.RedAddresses().DecodeParty(&mem)
	slot := -1
	for i, mon := range party.Mons {
		if mon.Species == speciesJigglypuff {
			slot = i
			break
		}
	}
	if slot < 0 {
		t.Fatalf("caught Jigglypuff missing from party: %+v", party.Mons)
	}
	if err := skill.UseEvolutionItem(e, romData, itemMoonStone, slot, speciesWigglytuff); err != nil {
		t.Fatalf("UseEvolutionItem(Jigglypuff, Moon Stone): %v", err)
	}

	state.Snapshot(e, &mem)
	party = skill.RedAddresses().DecodeParty(&mem)
	if slot >= len(party.Mons) || party.Mons[slot].Species != speciesWigglytuff {
		t.Fatalf("stone evolution party=%+v, want Wigglytuff (%#02x) in slot %d", party.Mons, speciesWigglytuff, slot)
	}
	requireDexOwned(t, romData, &mem, speciesWigglytuff)
}
