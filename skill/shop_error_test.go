package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/red/state"
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
