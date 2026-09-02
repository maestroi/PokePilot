package sym

import "testing"

func TestProgressionAddressesMatchSymbolFile(t *testing.T) {
	symbols := loadSym(t)
	for _, p := range []struct {
		name string
		got  uint16
	}{
		{"wTileInFrontOfPlayer", TileInFrontOfPlayer},
		{"wFieldMoves", FieldMoves},
		{"wNumFieldMoves", NumFieldMoves},
		{"wActionResultOrTookBattleTurn", ActionResult},
		{"wFirstLockTrashCanIndex", FirstLockTrashCanIndex},
		{"wSecondLockTrashCanIndex", SecondLockTrashCanIndex},
	} {
		want, ok := symbols[p.name]
		if !ok {
			t.Errorf("%s not found in vendored symbol file", p.name)
			continue
		}
		if p.got != want {
			t.Errorf("%s = 0x%04X, want 0x%04X", p.name, p.got, want)
		}
	}
}
