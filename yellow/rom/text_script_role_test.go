package rom

import (
	"testing"

	"github.com/maestroi/pokepilot/worldmodel"
)

func TestYellowSpecialInteractionRoles(t *testing.T) {
	for script, want := range map[byte]worldmodel.InteractionRole{
		textScriptPokemonCenterNurse: worldmodel.InteractionPokemonCenterNurse,
		textScriptMart:               worldmodel.InteractionMart,
		textScriptBillsPC:            worldmodel.InteractionBillsPC,
		textScriptPlayersPC:          worldmodel.InteractionPlayersPC,
		textScriptPokemonCenterPC:    worldmodel.InteractionPokemonCenterPC,
		textScriptPrizeVendor:        worldmodel.InteractionPrizeVendor,
		textScriptCableClub:          worldmodel.InteractionCableClub,
		textScriptVendingMachine:     worldmodel.InteractionVendingMachine,
	} {
		got, ok := specialInteractionRole(script)
		if !ok || got != want {
			t.Fatalf("specialInteractionRole(%#02x)=%q,%v, want %q,true", script, got, ok, want)
		}
	}
	if got, ok := specialInteractionRole(0); ok || got != "" {
		t.Fatalf("ordinary text script classified as %q,%v", got, ok)
	}
}
