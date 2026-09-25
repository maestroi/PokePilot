package profile

import (
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
)

// DecodePartyMenu classifies the active Red/Blue party-selection surface.
// The SWITCH/STATS/CANCEL overlay takes ownership of input even though the
// underlying party footer remains in the tilemap, so it intentionally reports
// no visible party menu while that overlay is up.
func (*Profile) DecodePartyMenu(reader game.MemoryReader) game.PartyMenuState {
	if reader == nil {
		return game.PartyMenuState{}
	}
	var mem state.Mem
	reader.PeekInto(0, mem[:])
	text := state.ScreenText(&mem)
	if strings.Contains(text, "SWITCH") {
		return game.PartyMenuState{}
	}

	kind := game.PartyMenuKind("")
	switch {
	case strings.Contains(text, "Bring out"):
		kind = game.PartyMenuForcedBattle
	case state.DecodeBattle(&mem) != nil && strings.Contains(text, "Choose"):
		kind = game.PartyMenuVoluntaryBattle
	case strings.Contains(text, "Use item"):
		kind = game.PartyMenuItemUse
	default:
		return game.PartyMenuState{}
	}

	party := state.DecodeParty(&mem)
	max := int(party.Count) - 1
	menu := state.DecodeMenu(&mem)
	return game.PartyMenuState{
		Visible: true,
		Kind:    kind,
		Cursor: game.MenuCursorState{
			Current: menu.Current,
			Max:     max,
		},
	}
}
