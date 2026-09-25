package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/yellow/sym"
)

func TestDecodeOpeningReadsYellowNativeFlags(t *testing.T) {
	var mem fakeMemory
	mem[sym.CurMap] = OaksLabMap
	mem[sym.XCoord], mem[sym.YCoord] = 5, 3
	mem[sym.PartyCount] = 1
	mem[sym.CurMapWidth], mem[sym.CurMapHeight] = 5, 6
	setYellowEvent(&mem, eventOakAppearedInPallet)
	setYellowEvent(&mem, eventFollowedOakIntoLab)
	setYellowEvent(&mem, eventOakAskedToChooseMon)
	setYellowEvent(&mem, eventGotStarter)

	f := DecodeOpening(&mem)
	if f.Map != OaksLabMap || f.X != 5 || f.Y != 3 || f.PartyCount != 1 {
		t.Fatalf("position/party = %+v", f)
	}
	if !f.Controllable || f.InBattle {
		t.Fatalf("control = %+v, want controllable and out of battle", f)
	}
	if !f.OakAppeared || !f.FollowedOak || !f.OakAskedToChoose || !f.GotStarter {
		t.Fatalf("opening flags missing: %+v", f)
	}
	if f.BattledRival {
		t.Fatal("rival battle flag read before it was set")
	}

	mem[sym.JoyIgnore] = 0xff
	mem[sym.IsInBattle] = 2
	setYellowEvent(&mem, eventBattledRivalInOaksLab)
	f = DecodeOpening(&mem)
	if f.Controllable || !f.InBattle || !f.BattledRival {
		t.Fatalf("scripted/battle state = %+v", f)
	}
}

func TestDecodeOpeningNilReader(t *testing.T) {
	if f := DecodeOpening(nil); f != (OpeningFacts{}) {
		t.Fatalf("nil reader decoded %+v", f)
	}
}
