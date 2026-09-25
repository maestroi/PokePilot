package skill

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

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

// A slow emulator (paced play, a high-latency peer) is not a stall: only a
// frame count that stops moving is.
func TestAwaitLinkProgressToleratesSlowProgressButNotAFrozenFrame(t *testing.T) {
	var frame atomic.Uint64
	done := make(chan linkTradeOutcome, 1)
	stop := make(chan struct{})
	go func() {
		for i := 0; i < 20; i++ {
			time.Sleep(5 * time.Millisecond)
			frame.Add(1)
		}
		done <- linkTradeOutcome{result: LinkTradeResult{Trades: 1}}
		close(stop)
	}()
	o, err := awaitLinkProgress(frame.Load, done, 30*time.Millisecond, time.Millisecond)
	if err != nil || o.result.Trades != 1 {
		t.Fatalf("slow but progressing trade: outcome=%+v err=%v, want completed", o, err)
	}
	<-stop

	_, err = awaitLinkProgress(frame.Load, make(chan linkTradeOutcome), 30*time.Millisecond, time.Millisecond)
	if !errors.Is(err, ErrLinkStalled) {
		t.Fatalf("frozen frame: err = %v, want ErrLinkStalled", err)
	}
}
