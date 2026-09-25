package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/sym"
)

const (
	celadonMart4FMap    uint8 = 0x7D
	celadonMart4FClerkX uint8 = 5
	celadonMart4FClerkY uint8 = 7
)

func init() {
	// Shopping is an interaction-owned destination: reaching the fourth floor
	// by itself is not a useful standalone objective, and Buy expects Red to be
	// standing beside/facing the clerk when it opens the mart menu.
	interactionPlaces["celadon mart 4f stones"] = InteractionDestination(
		celadonMart4FMap,
		celadonMart4FClerkX,
		celadonMart4FClerkY,
	)
}

// BuyEvolutionStone travels to Celadon Mart 4F, proves from the ROM-backed mart
// table that this clerk stocks item, faces the clerk, and buys exactly one.
// Buy owns the menu controller and positively verifies both the bag increment
// and the money debit before returning to a controllable overworld boundary.
func BuyEvolutionStone(m *emu.Emu, romData []byte, item uint8, policy MovePolicy) (TravelResult, error) {
	if policy == nil {
		return TravelResult{}, fmt.Errorf("skill: BuyEvolutionStone: nil policy")
	}
	stock, err := rom.MartItems(romData, celadonMart4FMap)
	if err != nil {
		return TravelResult{}, fmt.Errorf("skill: BuyEvolutionStone: read Celadon Mart 4F stock: %w", err)
	}
	stocked := false
	for _, id := range stock {
		if id == item {
			stocked = true
			break
		}
	}
	if !stocked {
		return TravelResult{}, fmt.Errorf("skill: BuyEvolutionStone: %w: item %#02x at Celadon Mart 4F", ErrNotInStock, item)
	}

	dest, ok := Place("celadon mart 4f stones")
	if !ok {
		return TravelResult{}, fmt.Errorf("skill: BuyEvolutionStone: Celadon Mart 4F destination missing")
	}
	travel, err := TravelFlee(m, romData, dest, policy, 80)
	if err != nil {
		return travel, fmt.Errorf("skill: BuyEvolutionStone: reach Celadon Mart 4F: %w", err)
	}
	// The clerk stands behind a service counter, so arriving at the
	// interaction destination parks Red two tiles away with the counter
	// between. Face must turn toward that counter tile; facing the clerk's own
	// tile is not adjacent and fails outright (run-jxh8lk19wv6on: 45 identical
	// "tile (5,7) is not orthogonally adjacent to (5,5)" failures from a
	// Celadon City checkpoint). This is the same rule TalkAt uses, so it lives
	// in one place rather than being re-derived per interaction.
	cur := m.Peek8(sym.CurMap)
	h, err := rom.ParseMap(romData, cur)
	if err != nil {
		return travel, fmt.Errorf("skill: BuyEvolutionStone: parse map %#04x: %w", cur, err)
	}
	faceX, faceY, facing := interactionFacingTile(m, romData, h, celadonMart4FClerkX, celadonMart4FClerkY)
	if !facing {
		px, py := playerXY(m)
		return travel, fmt.Errorf("skill: BuyEvolutionStone: not in an interaction position for the clerk at (%d,%d): standing at (%d,%d) on map %#04x",
			celadonMart4FClerkX, celadonMart4FClerkY, px, py, cur)
	}
	if err := Face(m, faceX, faceY); err != nil {
		return travel, fmt.Errorf("skill: BuyEvolutionStone: face clerk: %w", err)
	}
	if err := Buy(m, item, 1); err != nil {
		return travel, fmt.Errorf("skill: BuyEvolutionStone: %w", err)
	}
	return travel, nil
}
