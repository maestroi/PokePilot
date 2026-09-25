package profile

import (
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/yellow/sym"
)

// Yellow's opening happens on two maps. Their ids match Red's, but they are
// Yellow facts here (pokeyellow/constants/map_constants.asm) so the Yellow
// story controller never reads them from a Red package.
const (
	PalletTownMap uint8 = 0x00 // PALLET_TOWN
	OaksLabMap    uint8 = 0x28 // OAKS_LAB
)

// OpeningFacts is the slice of Yellow's native RAM the opening controller
// needs to decide its next step: the five story flags the Pallet Town and
// Oak's Lab scripts set in order, and where/whether the player can act.
// Every field is read from the flags and variables the decomp's scripts
// write (pokeyellow/scripts/PalletTown.asm, OaksLab.asm), never inferred
// from dialogue.
type OpeningFacts struct {
	Map          uint8
	X, Y         uint8
	PartyCount   uint8
	InBattle     bool
	Controllable bool

	// OakAppeared is EVENT_OAK_APPEARED_IN_PALLET: the north-exit gate fired.
	OakAppeared bool
	// FollowedOak is EVENT_FOLLOWED_OAK_INTO_LAB: the Pikachu cutscene ended
	// with the player walked into the lab.
	FollowedOak bool
	// OakAskedToChoose is EVENT_OAK_ASKED_TO_CHOOSE_MON: the lab speech ran
	// and control came back so the player can go for the Poké Ball.
	OakAskedToChoose bool
	// GotStarter is EVENT_GOT_STARTER: Oak handed over Pikachu.
	GotStarter bool
	// BattledRival is EVENT_BATTLED_RIVAL_IN_OAKS_LAB: the lab rival battle
	// ended (won or lost; the script heals and continues either way).
	BattledRival bool
}

// DecodeOpening reads OpeningFacts from Yellow's native RAM. A canonical-view
// emulator is read natively (see native.go); a plain reader is taken as
// native already.
func DecodeOpening(reader game.MemoryReader) OpeningFacts {
	if reader == nil {
		return OpeningFacts{}
	}
	reader = native(reader)
	return OpeningFacts{
		Map:              reader.Peek8(sym.CurMap),
		X:                reader.Peek8(sym.XCoord),
		Y:                reader.Peek8(sym.YCoord),
		PartyCount:       reader.Peek8(sym.PartyCount),
		InBattle:         reader.Peek8(sym.IsInBattle) != 0,
		Controllable:     yellowControllable(reader),
		OakAppeared:      yellowHasEvent(reader, eventOakAppearedInPallet),
		FollowedOak:      yellowHasEvent(reader, eventFollowedOakIntoLab),
		OakAskedToChoose: yellowHasEvent(reader, eventOakAskedToChooseMon),
		GotStarter:       yellowHasEvent(reader, eventGotStarter),
		BattledRival:     yellowHasEvent(reader, eventBattledRivalInOaksLab),
	}
}
