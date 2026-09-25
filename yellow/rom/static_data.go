package rom

import "github.com/maestroi/pokepilot/gen1rom"

const (
	yellowWildPointersBank uint8  = 0x03
	yellowWildPointersAddr uint16 = 0x4B95
	yellowMapCount                = 0xF9

	yellowMovesOffset     = 0x38000
	yellowMoveEntryLen    = 6
	yellowMoveNamesOffset = 0xBC000
	yellowMoveCount       = 0xA5

	yellowSpeciesNamesOffset = 0xE8000
	yellowSpeciesNameLen     = 10
	yellowSpeciesCount       = 0xBE
	yellowPokedexOrderOffset = 0x410B1
	yellowPokedexOrderLen    = 0xBE
	yellowBaseStatsOffset    = 0x383DE
	yellowBaseStatsEntryLen  = 28

	yellowItemNamesOffset = 0x45B7
	yellowItemCount       = 0x53

	yellowTrainerPointersBank uint8  = 0x0E
	yellowTrainerPointersAddr uint16 = 0x5DD1
	yellowTrainerClassCount          = 0x2F
)

var yellowWildLayout = gen1rom.WildLayout{
	PointerBank: yellowWildPointersBank,
	PointerAddr: yellowWildPointersAddr,
	MapCount:    yellowMapCount,
}

var yellowMoveLayout = gen1rom.MoveLayout{
	Offset:   yellowMovesOffset,
	EntryLen: yellowMoveEntryLen,
}

var yellowSpeciesLayout = gen1rom.SpeciesLayout{
	NamesOffset:        yellowSpeciesNamesOffset,
	NameLength:         yellowSpeciesNameLen,
	InternalCount:      yellowSpeciesCount,
	PokedexOrderOffset: yellowPokedexOrderOffset,
	PokedexOrderLen:    yellowPokedexOrderLen,
	BaseStatsOffset:    yellowBaseStatsOffset,
	BaseStatsEntryLen:  yellowBaseStatsEntryLen,
}

var yellowTrainerLayout = gen1rom.TrainerLayout{
	PointerBank:  yellowTrainerPointersBank,
	PointerAddr:  yellowTrainerPointersAddr,
	TrainerCount: yellowTrainerClassCount,
}

func WildEncounters(romData []byte) ([]gen1rom.WildEncounter, error) {
	return gen1rom.WildEncounters(romData, yellowWildLayout)
}

func LookupMove(romData []byte, id uint8) (gen1rom.Move, error) {
	return gen1rom.LookupMove(romData, id, yellowMoveLayout)
}

func MoveName(romData []byte, id uint8) (string, error) {
	return gen1rom.ListName(romData, id, gen1rom.StringListLayout{
		Offset: yellowMoveNamesOffset,
		Count:  yellowMoveCount,
	})
}

func SpeciesName(romData []byte, species uint8) (string, error) {
	return gen1rom.SpeciesName(romData, species, yellowSpeciesLayout)
}

func InternalSpeciesDexNumber(romData []byte, species uint8) (uint8, error) {
	return gen1rom.InternalSpeciesDexNumber(romData, species, yellowSpeciesLayout)
}

func DexNumberInternalSpecies(romData []byte, dex uint8) (uint8, error) {
	return gen1rom.DexNumberInternalSpecies(romData, dex, yellowSpeciesLayout)
}

func LookupSpeciesBaseStats(romData []byte, species uint8) (gen1rom.SpeciesBaseStats, error) {
	return gen1rom.LookupSpeciesBaseStats(romData, species, yellowSpeciesLayout)
}

func ItemName(romData []byte, id uint8) (string, error) {
	return gen1rom.ListName(romData, id, gen1rom.StringListLayout{
		Offset: yellowItemNamesOffset,
		Count:  yellowItemCount,
	})
}

func TrainerParty(romData []byte, class, set uint8) ([]gen1rom.TrainerMon, error) {
	return gen1rom.TrainerParty(romData, class, set, yellowTrainerLayout)
}

func MartItems(romData []byte, mapID uint8) ([]uint8, error) {
	h, err := ParseMap(romData, mapID)
	if err != nil {
		return nil, err
	}
	return gen1rom.MartItems(romData, gen1rom.MapHeader(h))
}

func MartClerkPosition(romData []byte, mapID uint8) (uint8, uint8, error) {
	h, err := ParseMap(romData, mapID)
	if err != nil {
		return 0, 0, err
	}
	return gen1rom.MartClerkPosition(romData, gen1rom.MapHeader(h))
}
