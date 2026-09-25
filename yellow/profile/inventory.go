package profile

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
)

func yellowBag(reader game.MemoryReader, romData []byte) []game.ProfileItem {
	raw := gen1.DecodeBag(reader, yellowRAMLayout)
	out := make([]game.ProfileItem, 0, len(raw))
	for _, item := range raw {
		name, err := yellowrom.ItemName(romData, item.ID)
		if err != nil || strings.TrimSpace(name) == "" {
			name = fmt.Sprintf("item_%02x", item.ID)
		}
		id := game.ItemID(game.CanonicalID(name))
		out = append(out, game.ProfileItem{
			ID: id, Name: string(id), Quantity: int(item.Quantity),
		})
	}
	return out
}
