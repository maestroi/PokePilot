package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/gs/sym"
)

func TestDecodeOpeningProjectsWideMapFacingAndParty(t *testing.T) {
	mem := fakeGSReader{
		sym.MapGroup:        24,
		sym.MapNumber:       5,
		sym.XCoord:          7,
		sym.YCoord:          4,
		sym.PlayerDirection: 0x04,
		sym.MapWidth:        5,
		sym.MapHeight:       6,
		sym.MapStatus:       gen2MapStatusHandle,
		sym.MapEventStatus:  gen2MapEventsOn,
		sym.PartyCount:      1,
		sym.PartyMon1:       0x9b,
	}
	facts := NewGold().DecodeOpening(mem)
	if facts.NativeMapID != 0x1805 {
		t.Fatalf("map = %#04x, want 0x1805", facts.NativeMapID)
	}
	if facts.X != 7 || facts.Y != 4 || facts.Facing != "up" {
		t.Fatalf("player = (%d,%d) facing=%q, want (7,4) up", facts.X, facts.Y, facts.Facing)
	}
	if !facts.Controllable || facts.ScriptActive || facts.InBattle {
		t.Fatalf("control facts = %+v, want stable overworld", facts)
	}
	if !facts.HasSpecies("cyndaquil") {
		t.Fatalf("party = %v, want cyndaquil", facts.Party)
	}
}

func TestDecodeOpeningMarksKnownScriptAsOwnedState(t *testing.T) {
	mem := fakeGSReader{
		sym.MapGroup:       24,
		sym.MapNumber:      6,
		sym.MapWidth:       5,
		sym.MapHeight:      4,
		sym.MapStatus:      gen2MapStatusHandle,
		sym.MapEventStatus: gen2MapEventsOn,
		sym.ScriptMode:     1,
	}
	facts := NewGold().DecodeOpening(mem)
	if facts.Controllable || !facts.ScriptActive {
		t.Fatalf("facts = %+v, want active opening script", facts)
	}
}

func TestDecodeOpeningProjectsPostStarterStoryAndRivalName(t *testing.T) {
	mem := fakeGSReader{
		sym.MapGroup:               24,
		sym.MapNumber:              5,
		sym.MapWidth:               5,
		sym.MapHeight:              6,
		sym.MapStatus:              gen2MapStatusHandle,
		sym.MapEventStatus:         gen2MapEventsOn,
		sym.StatusFlags:            statusFlagsPokedexMask,
		sym.MrPokemonsHouseSceneID: sceneMrPokemonsHouseNoop,
		sym.CherrygroveCitySceneID: sceneCherrygroveNoop,
		sym.ElmsLabSceneID:         sceneElmsLabNoop,
		sym.BattleResult:           1,
	}
	setGSEvent(mem, eventGotPokemonFromElm)
	setGSEvent(mem, eventGotMysteryEggMrPokemon)
	setGSEvent(mem, eventGaveMysteryEggToElm)
	putGSBytes(mem, sym.RivalName, encodeGSName("SILVER"))

	facts := NewGold().DecodeOpening(mem)
	if !facts.GotStarter || !facts.GotMysteryEgg || !facts.HasPokedex ||
		!facts.MrPokemonVisitComplete || !facts.CherrygroveRivalResolved ||
		!facts.RivalNamed || !facts.GaveMysteryEggToElm {
		t.Fatalf("story facts = %+v, want completed opening errand", facts)
	}
	if facts.RivalName != "SILVER" || facts.RivalNamePrompt {
		t.Fatalf("rival name = %q prompt=%v, want SILVER/false", facts.RivalName, facts.RivalNamePrompt)
	}
	if facts.BattleResult != 1 {
		t.Fatalf("battle result = %d, want 1", facts.BattleResult)
	}
}

func TestDecodeOpeningRecognizesRivalNamingScreen(t *testing.T) {
	mem := fakeGSReader{
		sym.MapGroup:         24,
		sym.MapNumber:        5,
		sym.MapWidth:         5,
		sym.MapHeight:        6,
		sym.MapStatus:        gen2MapStatusHandle,
		sym.MapEventStatus:   gen2MapEventsOn,
		sym.ScriptMode:       1,
		sym.NamingScreenType: 2,
		sym.RivalName:        0xf2,
	}
	facts := NewGold().DecodeOpening(mem)
	if !facts.ScriptActive || !facts.RivalNamePrompt {
		t.Fatalf("facts = %+v, want active rival-name prompt", facts)
	}

	putGSBytes(mem, sym.RivalName, encodeGSName("SILVER"))
	facts = NewGold().DecodeOpening(mem)
	if facts.RivalNamePrompt || facts.RivalName != "SILVER" {
		t.Fatalf("facts = %+v, want settled SILVER name", facts)
	}
}
