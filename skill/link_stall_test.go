package skill

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestLinkExchangeStalledPoisonsTheMachine(t *testing.T) {
	err := linkExchangeStalled(2134513)
	if !errors.Is(err, ErrLinkStalled) {
		t.Fatalf("err = %v, want ErrLinkStalled", err)
	}
	if !errors.Is(err, game.ErrMachineUnusable) {
		t.Fatalf("err = %v, want game.ErrMachineUnusable", err)
	}
}
