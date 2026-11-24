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
	HeldItem        uint16 // Native party item id; zero means none.
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
	// ActiveTrapped is true when the active battler cannot currently be
	// recalled (Gen II Mean Look / Spider Web / Block / Arena Trap, or a
	// binding Wrap count). Adapters project cartridge-specific trap flags;
	// generic Battle must not attempt a voluntary SwitchActive while set.
	ActiveTrapped bool
	Party         []BattlePartyMon
	Bag           []InventoryItem
	// Medicines is the profile's own table of bag items whose battle effect
	// it can prove, ordered by HP restored (smallest first). Generic policy
	// selects medicine only from here and never names native item ids.
	Medicines []Medicine
}

// MedicineFullHP marks a medicine that restores the target to max HP.
const MedicineFullHP = 1 << 30

// MedicineCuresAny marks a medicine that cures every non-volatile status.
const MedicineCuresAny = "any"

// Medicine is one native item's battle effect: HP restored (MedicineFullHP
// for a full restore, zero for a pure cure) and the status it cures
// (MedicineCuresAny, a portable status name such as "frozen", or none).
type Medicine struct {
	NativeItemID uint16
	HP           int
	Cures        string
}

// CuresStatus reports whether m cures the portable status name.
func (m Medicine) CuresStatus(status string) bool {
	return status != "" && (m.Cures == MedicineCuresAny || m.Cures == status)
}

// Medicine returns the profile's table entry for a native item id.
func (s BattleResourcesState) Medicine(nativeItemID uint16) (Medicine, bool) {
	for _, med := range s.Medicines {
		if med.NativeItemID == nativeItemID {
			return med, true
		}
	}
	return Medicine{}, false
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

func (s BattleResourcesState) ItemIndex(nativeItemID uint16) (int, bool) {
	for i, item := range s.Bag {
		if item.NativeItemID == nativeItemID {
			return i, true
		}
	}
	return 0, false
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
