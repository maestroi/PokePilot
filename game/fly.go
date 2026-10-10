package game

// FlyMenuState is a semantic view of a game's destination picker.
// The profile decodes the rendered selection; generic execution never
// interprets RAM cursor offsets or assumes a game's Flypoint ordering.
type FlyMenuState struct {
	Visible       bool
	SelectedLabel string
}

// FlyDestinationDecoder maps a native target map to a rendered Fly label and
// observes the active game's visited-destination picker. A destination that
// is not offered by the cartridge must never be selected speculatively.
type FlyDestinationDecoder interface {
	FlyDestinationLabel(nativeMapID uint16) (string, bool)
	DecodeFlyMenu(MemoryReader) FlyMenuState
}
