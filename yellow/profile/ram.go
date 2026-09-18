package profile

import (
	"github.com/maestroi/pokepilot/gen1"
	"github.com/maestroi/pokepilot/yellow/sym"
)

var yellowRAMLayout = gen1.RAMLayout{
	PartyCount:     sym.PartyCount,
	PartyMon1:      sym.PartyMon1,
	PartyMonSize:   sym.PartyMonSize,
	NumBagItems:    sym.NumBagItems,
	BagItems:       sym.BagItems,
	PlayerMoney:    sym.PlayerMoney,
	ObtainedBadges: sym.ObtainedBadges,
}
