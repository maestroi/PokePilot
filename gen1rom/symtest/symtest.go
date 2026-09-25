// Package symtest checks a gen1rom.TableLayout against the vendored decomp
// symbol file it was transcribed from. It is test support only.
package symtest

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/gen1rom"
)

// Labels names the decomp label each layout symbol must resolve to.
func Labels(layout gen1rom.TableLayout, superRodLabel string) map[string]gen1rom.Symbol {
	return map[string]gen1rom.Symbol{
		"MapHeaderPointers":       layout.MapHeaderPointers,
		"MapHeaderBanks":          layout.MapHeaderBanks,
		"Tilesets":                layout.Tilesets,
		"TilePairCollisionsLand":  layout.TilePairCollisionsLand,
		"TilePairCollisionsWater": layout.TilePairCollisionsWater,
		"LedgeTiles":              layout.LedgeTiles,
		"WildDataPointers":        layout.WildDataPointers,
		"EvosMovesPointerTable":   layout.EvosMovesPointerTable,
		"Moves":                   layout.Moves,
		"TechnicalMachines":       layout.TechnicalMachines,
		"PokedexOrder":            layout.PokedexOrder,
		"BaseStats":               layout.BaseStats,
		"TradeMons":               layout.TradeMons,
		"TypeEffects":             layout.TypeEffects,
		"ItemUseOldRod":           layout.ItemUseOldRod,
		"GoodRodMons":             layout.GoodRodMons,
		superRodLabel:             layout.SuperRod,
	}
}

// Verify fails t for every layout symbol that disagrees with symPath.
func Verify(t *testing.T, symPath string, want map[string]gen1rom.Symbol) {
	t.Helper()
	got, err := read(symPath)
	if err != nil {
		t.Fatal(err)
	}
	for label, sym := range want {
		at, ok := got[label]
		if !ok {
			t.Errorf("%s: label %s not found", symPath, label)
			continue
		}
		if at != sym {
			t.Errorf("%s: %s = %02x:%04x, layout has %02x:%04x", symPath, label, at.Bank, at.Addr, sym.Bank, sym.Addr)
		}
	}
}

func read(path string) (map[string]gen1rom.Symbol, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]gen1rom.Symbol{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 || strings.HasPrefix(fields[0], ";") {
			continue
		}
		bank, addr, ok := strings.Cut(fields[0], ":")
		if !ok {
			continue
		}
		b, err := strconv.ParseUint(bank, 16, 8)
		if err != nil {
			continue
		}
		a, err := strconv.ParseUint(addr, 16, 16)
		if err != nil {
			return nil, fmt.Errorf("%s: %q: %w", path, fields[0], err)
		}
		out[fields[1]] = gen1rom.Symbol{Bank: uint8(b), Addr: uint16(a)}
	}
	return out, sc.Err()
}
