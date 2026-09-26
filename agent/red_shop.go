package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/skill"
)

// prepareRedMartPurchase owns the interaction boundary for an ordinary Red
// KindBuy objective. The planner can offer a purchase anywhere on a map whose
// ROM table exposes mart stock, but Buy deliberately starts one layer later:
// it assumes the player is already standing at and facing the clerk.
//
// Resolve the service actor from the cartridge, route to a legal interaction
// position, then face either the clerk directly or the counter tile between
// the player and clerk. This keeps shop menu execution reusable while making
// the game adapter responsible for reaching its native interaction boundary.
func prepareRedMartPurchase(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("agent: prepare Red mart purchase: nil emulator")
	}
	mapID := m.Peek8(sym.CurMap)
	clerkX, clerkY, err := rom.MartClerkPosition(romData, mapID)
	if err != nil {
		return fmt.Errorf("resolve mart clerk on map %#04x: %w", mapID, err)
	}

	if err := skill.GoTo(m, romData, skill.InteractionDestination(mapID, clerkX, clerkY)); err != nil {
		return fmt.Errorf("approach mart clerk at (%d,%d) on map %#04x: %w", clerkX, clerkY, mapID, err)
	}

	px, py := m.Peek8(sym.XCoord), m.Peek8(sym.YCoord)
	faceX, faceY, ok := redMartFacingTile(px, py, clerkX, clerkY)
	if !ok {
		return fmt.Errorf("mart clerk at (%d,%d) is not interactable from (%d,%d) on map %#04x",
			clerkX, clerkY, px, py, mapID)
	}
	if err := skill.Face(m, faceX, faceY); err != nil {
		return fmt.Errorf("face mart clerk from (%d,%d) toward (%d,%d): %w", px, py, faceX, faceY, err)
	}
	return nil
}

// redMartFacingTile returns the tile the player must face after routing to a
// mart interaction destination. Ordinary actors are one tile away. Service
// counters place the player two tiles away on the same axis, so the facing
// target is the counter tile between player and clerk.
func redMartFacingTile(px, py, clerkX, clerkY uint8) (uint8, uint8, bool) {
	dx := int(clerkX) - int(px)
	dy := int(clerkY) - int(py)

	switch {
	case dx == 0 && (dy == -1 || dy == 1):
		return clerkX, clerkY, true
	case dy == 0 && (dx == -1 || dx == 1):
		return clerkX, clerkY, true
	case dx == 0 && (dy == -2 || dy == 2):
		return px, uint8(int(py) + dy/2), true
	case dy == 0 && (dx == -2 || dx == 2):
		return uint8(int(px) + dx/2), py, true
	default:
		return 0, 0, false
	}
}
