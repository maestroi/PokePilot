package game

// BattlePartyMove is one learned move projected for battle resource strategy.
// NativeMoveID is intentionally opaque to generic code; generation-specific
// strategy adapters own ROM metadata and mechanics interpretation.
type BattlePartyMove struct {
	NativeMoveID uint16
	PP           uint8
}

// BattlePartyMon is the portable roster view needed by switching, PP recovery,
// and medicine selection. Profiles expose current calculated stats so strategy
// does not need to reconstruct cartridge-specific party layouts.
type BattlePartyMon struct {
	NativeSpeciesID uint16
	Level           uint8
	HP              uint16
	MaxHP           uint16
	Status          string
	Type1           uint16
	Type2           uint16
	Attack          uint16
	Defense         uint16
	Speed           uint16
	Special         uint16
	SpecialAttack   uint16
	SpecialDefense  uint16
	Moves           [4]BattlePartyMove
}

func (m BattlePartyMon) Fainted() bool { return m.HP == 0 }

func (m BattlePartyMon) HasCurrentPP() bool {
	for _, move := range m.Moves {
		if move.NativeMoveID != 0 && move.PP > 0 {
			return true
		}
	}
	return false
}

// BattleResourcesState is the portable live roster/inventory view used by
// battle resource policy. It contains no menu cursor or execution state.
type BattleResourcesState struct {
	InBattle   bool
	ActiveSlot int
	Party      []BattlePartyMon
	Bag        []InventoryItem
}

func (s BattleResourcesState) ItemQuantity(nativeItemID uint16) int {
	total := 0
	for _, item := range s.Bag {
		if item.NativeItemID == nativeItemID {
			total += item.Quantity
		}
	}
	return total
}

func (s BattleResourcesState) FirstLivePartySlot() int {
	for i, mon := range s.Party {
		if !mon.Fainted() {
			return i
		}
	}
	return -1
}

func (s BattleResourcesState) PPRecoverySlot() (int, bool) {
	for i, mon := range s.Party {
		if i != s.ActiveSlot && !mon.Fainted() && mon.HasCurrentPP() {
			return i, true
		}
	}
	return 0, false
}

func (s BattleResourcesState) LivePartyHasCurrentPP() bool {
	for _, mon := range s.Party {
		if !mon.Fainted() && mon.HasCurrentPP() {
			return true
		}
	}
	return false
}

// BattleResourcesDecoder hides party/bag RAM layout from reusable battle
// resource policy.
type BattleResourcesDecoder interface {
	DecodeBattleResources(MemoryReader) BattleResourcesState
}

type BattleResourcesProfile interface {
	GameProfile
	BattleResourcesDecoder
}
