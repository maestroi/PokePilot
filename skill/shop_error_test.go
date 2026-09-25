package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestMartTimeoutIsTyped(t *testing.T) {
	var mem state.Mem
	err := martTimeout("the item list", &mem)
	if !errors.Is(err, ErrShopMenuTimeout) {
		t.Fatalf("martTimeout = %v, want ErrShopMenuTimeout", err)
	}
}

func TestShopStabilizationPreservesOriginalCause(t *testing.T) {
	err := shopStabilizationFailure(ErrShopMenuTimeout, errors.New("still in menu"))
	if !errors.Is(err, ErrShopMenuTimeout) || !errors.Is(err, ErrShopStabilization) {
		t.Fatalf("joined error lost identity: %v", err)
	}
}

func TestQuantityBoxUpWhenHighlightedPriceDoesNotChange(t *testing.T) {
	var mem state.Mem
	mem[sym.Money] = 0x00
	mem[sym.Money+1] = 0x02
	mem[sym.Money+2] = 0x00
	mem[sym.MaxItemQuantity] = 99
	mem[sym.ItemQuantity] = 1
	if !quantityBoxUp(&mem, 200, 1) {
		t.Fatal("quantity box whose ¥200 total matches the highlighted Poké Ball was not recognized")
	}
	mem[sym.MaxItemQuantity] = 1
	if quantityBoxUp(&mem, 200, 1) {
		t.Fatal("priced item list with an unchanged ¥200 highlight was treated as the quantity box")
	}
}
