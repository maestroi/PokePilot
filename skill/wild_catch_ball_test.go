package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
)

// A better ball is the largest lever on a throw's catch chance, so ordinary
// wild catches throw the strongest ball in the bag and keep the Master Ball
// for one-time encounters.
func TestWildCatchBallPrefersStrongestOrdinaryBall(t *testing.T) {
	const master, ultra, great, poke = 0x01, 0x02, 0x03, ItemPokeBall
	var mem state.Mem
	if _, ok := wildCatchBall(&mem); ok {
		t.Fatal("empty bag offered a ball")
	}
	setTestBag(&mem, [2]uint8{master, 1})
	if ball, ok := wildCatchBall(&mem); ok {
		t.Fatalf("wild catch spent the Master Ball (%#02x)", ball)
	}
	setTestBag(&mem, [2]uint8{poke, 5}, [2]uint8{great, 2}, [2]uint8{master, 1})
	if ball, _ := wildCatchBall(&mem); ball != great {
		t.Fatalf("ball = %#02x, want Great Ball", ball)
	}
	if n := wildBallCount(&mem); n != 7 {
		t.Fatalf("wild ball count = %d, want 7 (Master Ball excluded)", n)
	}
	setTestBag(&mem, [2]uint8{poke, 5}, [2]uint8{ultra, 1}, [2]uint8{great, 2})
	if ball, _ := wildCatchBall(&mem); ball != ultra {
		t.Fatalf("ball = %#02x, want Ultra Ball", ball)
	}
}
