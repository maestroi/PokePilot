package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func gsFieldActionControllable(mem *fakeMemory) {
	mem[sym.MapWidth] = 10
	mem[sym.MapHeight] = 10
	mem[sym.MapStatus] = gen2MapStatusHandle
	mem[sym.MapEventStatus] = gen2MapEventsOn
	mem[sym.ScriptMode] = gen2ScriptOff
}

func gsDrawTextbox(mem *fakeMemory) {
	mem[sym.TileMap+12*20+0] = 0x79
	mem[sym.TileMap+12*20+19] = 0x7b
	mem[sym.TileMap+17*20+0] = 0x7d
	mem[sym.TileMap+17*20+19] = 0x7e
}

func TestGSFieldActionProjectsLiveEffects(t *testing.T) {
	var mem fakeMemory
	gsFieldActionControllable(&mem)
	mem[sym.PlayerState] = gen2PlayerSurf
	mem[sym.BikeFlags] = gen2StrengthActiveBit
	mem[sym.TimeOfDayPalset] = gen2DarknessPalset
	mem[sym.StatusFlags] = gen2FlashActiveBit
	mem[sym.FieldMoveSucceeded] = gen2FieldMoveSucceeded

	state := NewGold().DecodeFieldAction(&mem)
	if !state.Controllable || !state.Surfing || !state.StrengthActive || !state.Lit || !state.ActionSucceeded {
		t.Fatalf("field action state=%+v, want live effects projected", state)
	}
	if state.CutTargetKnown || state.BoulderTargetKnown {
		t.Fatalf("unimplemented facing-target decoder claimed authority: %+v", state)
	}
}

func TestGSFieldActionDistinguishesDarknessFromNormalMaps(t *testing.T) {
	var mem fakeMemory
	gsFieldActionControllable(&mem)

	mem[sym.TimeOfDayPalset] = gen2DarknessPalset
	if got := NewSilver().DecodeFieldAction(&mem); got.Lit {
		t.Fatalf("dark cave without Flash decoded lit: %+v", got)
	}

	mem[sym.StatusFlags] = gen2FlashActiveBit
	if got := NewSilver().DecodeFieldAction(&mem); !got.Lit {
		t.Fatalf("dark cave with Flash did not decode lit: %+v", got)
	}

	mem[sym.StatusFlags] = 0
	mem[sym.TimeOfDayPalset] = 0
	if got := NewSilver().DecodeFieldAction(&mem); !got.Lit {
		t.Fatalf("ordinary palette decoded as dark: %+v", got)
	}
}

func TestGSFieldActionRecognizesSurfPikachuAndResultText(t *testing.T) {
	var mem fakeMemory
	mem[sym.PlayerState] = gen2PlayerSurfPika
	gsDrawTextbox(&mem)

	state := NewGold().DecodeFieldAction(&mem)
	if !state.Surfing {
		t.Fatalf("surfing Pikachu state not recognized: %+v", state)
	}
	if !state.ResultTextActive {
		t.Fatalf("visible standard field-action textbox not recognized: %+v", state)
	}
}

func TestGSFieldActionResultUsesOverworldAliasNotBattleState(t *testing.T) {
	var mem fakeMemory
	mem[sym.FieldMoveSucceeded] = 2
	if got := NewGold().DecodeFieldAction(&mem); got.ActionSucceeded {
		t.Fatalf("non-success field result decoded successful: %+v", got)
	}
	mem[sym.FieldMoveSucceeded] = 1
	if got := NewGold().DecodeFieldAction(&mem); !got.ActionSucceeded {
		t.Fatalf("success field result not decoded: %+v", got)
	}
}

func TestGSProfilesImplementFieldActionAndMoveContracts(t *testing.T) {
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		if _, ok := any(p).(game.FieldActionDecoder); !ok {
			t.Fatalf("%s missing FieldActionDecoder", p.ID())
		}
		if _, ok := any(p).(game.FieldMoveProfile); !ok {
			t.Fatalf("%s missing FieldMoveProfile", p.ID())
		}
	}
}
