package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/yellow/sym"
)

const (
	PalletTownMap uint8 = 0x00
	OaksLabMap    uint8 = 0x28
)

// OpeningFacts is the Yellow-owned semantic slice required to drive the
// scripted Pikachu opening. Native event/RAM layout stays in the profile;
// the agent only sees story facts and control state.
type OpeningFacts struct {
	Map        uint8
	X, Y       uint8
	PartyCount uint8

	InBattle bool
	// BattlePending is wCurOpponent: a script has queued a battle that the
	// overworld loop has not entered yet (the transition animation runs with
	// joypad ignore clear and wIsInBattle still zero).
	BattlePending bool
	Controllable  bool
	TextOpen      bool
	TextBoxID     uint8
	ChoicePrompt  bool

	OakAppeared      bool
	FollowedOak      bool
	OakAskedToChoose bool
	GotStarter       bool
	BattledRival     bool
}

// DecodeOpening keeps Yellow's native opening flags behind the profile
// boundary while reusing the shared semantic menu decoder for prompt safety.
func DecodeOpening(reader game.MemoryReader) OpeningFacts {
	if reader == nil {
		return OpeningFacts{}
	}
	_, choice := (&Profile{}).DecodeTwoOption(reader)
	nativeReader := native(reader)
	return OpeningFacts{
		Map:              nativeReader.Peek8(sym.CurMap),
		X:                nativeReader.Peek8(sym.XCoord),
		Y:                nativeReader.Peek8(sym.YCoord),
		PartyCount:       nativeReader.Peek8(sym.PartyCount),
		InBattle:         nativeReader.Peek8(sym.IsInBattle) != 0,
		BattlePending:    nativeReader.Peek8(sym.CurOpponent) != 0,
		Controllable:     yellowControllable(nativeReader),
		TextOpen:         nativeReader.Peek8(sym.FontLoaded) != 0,
		TextBoxID:        nativeReader.Peek8(sym.TextBoxID),
		ChoicePrompt:     choice,
		OakAppeared:      yellowHasEvent(nativeReader, eventOakAppearedInPallet),
		FollowedOak:      yellowHasEvent(nativeReader, eventFollowedOakIntoLab),
		OakAskedToChoose: yellowHasEvent(nativeReader, eventOakAskedToChooseMon),
		GotStarter:       yellowHasEvent(nativeReader, eventGotStarter),
		BattledRival:     yellowHasEvent(nativeReader, eventBattledRivalInOaksLab),
	}
}
