package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
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
	interactionPlaces["celadon mart 4f stones"] = Destination{
		Map: celadonMart4FMap,
		X:   celadonMart4FClerkX,
		Y:   celadonMart4FClerkY + 1,
	}
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
	if err := Face(m, celadonMart4FClerkX, celadonMart4FClerkY); err != nil {
		return travel, fmt.Errorf("skill: BuyEvolutionStone: face clerk: %w", err)
	}
	if err := Buy(m, item, 1); err != nil {
		return travel, fmt.Errorf("skill: BuyEvolutionStone: %w", err)
	}
	return travel, nil
}
