package rom

// Red layout symbols under the names the synthetic-ROM tests patch.
var (
	wildDataPointersBank    = redTables.WildDataPointers.Bank
	wildDataPointersAddr    = redTables.WildDataPointers.Addr
	evosMovesBank           = redTables.EvosMovesPointerTable.Bank
	evosMovesAddr           = redTables.EvosMovesPointerTable.Addr
	superRodDataBank        = redTables.SuperRod.Bank
	superRodDataAddr        = redTables.SuperRod.Addr
	goodRodMonsBank         = redTables.GoodRodMons.Bank
	goodRodMonsAddr         = redTables.GoodRodMons.Addr
	itemUseOldRodBank       = redTables.ItemUseOldRod.Bank
	itemUseOldRodAddr       = redTables.ItemUseOldRod.Addr
	tradeMonsBank           = redTables.TradeMons.Bank
	tradeMonsAddr           = redTables.TradeMons.Addr
	movesOffset             = mustOffset(redTables.Moves)
	technicalMachinesOffset = mustOffset(redTables.TechnicalMachines)
	pokedexOrderOffset      = mustOffset(redTables.PokedexOrder)
	baseStatsOffset         = mustOffset(redTables.BaseStats)
)

func mustOffset(s interface{ Offset() (int, error) }) int {
	off, err := s.Offset()
	if err != nil {
		panic(err)
	}
	return off
}
