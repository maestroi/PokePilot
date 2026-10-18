package skill

import (
	"errors"
	"os"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func TestReachabilityAfterFieldBridge(t *testing.T) {
	bridge := Destination{Map: 0x15, X: 9, Y: 44}
	gate := &world.RouteBlockedError{Blockages: []gameruntime.TransitionBlockage{{
		Transition: gameruntime.Transition{ID: "test:gate"}, Missing: []gameruntime.CapabilityID{"can_open_gate"},
	}}}
	tests := []struct {
		name       string
		live       error
		bridges    []Destination
		fromBridge error
		want       error
		consulted  bool
	}{
		{name: "live route wins", live: nil, bridges: []Destination{bridge}, want: nil},
		{name: "no bridge keeps no route", live: world.ErrNoRoute, want: world.ErrNoRoute},
		{name: "bridge opens route", live: world.ErrNoRoute, bridges: []Destination{bridge}, fromBridge: nil, want: nil, consulted: true},
		{name: "bridge reaches only a gate", live: world.ErrNoRoute, bridges: []Destination{bridge}, fromBridge: gate, want: gate, consulted: true},
		{name: "live semantic gate is kept", live: gate, bridges: []Destination{bridge}, want: gate},
		{name: "other errors are kept", live: world.ErrNoPath, bridges: []Destination{bridge}, want: world.ErrNoPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			consulted := false
			got := reachabilityAfterFieldBridge(tt.live, func() []Destination { return tt.bridges }, func(from Destination) error {
				consulted = true
				if from != bridge {
					t.Fatalf("routed from %+v, want bridge %+v", from, bridge)
				}
				return tt.fromBridge
			})
			if !errors.Is(got, tt.want) && got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			if consulted != tt.consulted {
				t.Fatalf("bridge consulted=%v, want %v", consulted, tt.consulted)
			}
		})
	}
}

// TestRoute10PowerPlantPocketReachabilityRealROM pins run-d6dokr184ky81
// (#2770): standing in Route 10's Power Plant pocket with Surf usable, the
// pocket's land component reaches no map exit, so component routing reported
// every off-map journey unroutable while Travel surfed out. The offer menu
// collapsed to the Power Plant and Route 10 and the run ping-ponged between
// them until the stagnation watchdog stopped it.
//
// POKEPILOT_ROUTE10_POCKET_STATE is that run's attempt-289
// stall-frame-0218892629-Restock-15-pokeballs-...-.state (stock Red ROM, CGB).
func TestRoute10PowerPlantPocketReachabilityRealROM(t *testing.T) {
	path := os.Getenv("POKEPILOT_ROUTE10_POCKET_STATE")
	if path == "" {
		t.Skip("set POKEPILOT_ROUTE10_POCKET_STATE")
	}
	m := openEmuCGB(t)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.LoadState(b); err != nil {
		t.Fatal(err)
	}
	route10, ok := Place("route 10")
	if !ok {
		t.Fatal("missing place route 10")
	}
	if got := m.Peek8(sym.CurMap); got != route10.Map {
		t.Fatalf("prepared state on map %#02x, want Route 10 %#02x", got, route10.Map)
	}
	planner, err := NewRoutePlanner(m, m.ROM())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cerulean city", "lavender town", "rock tunnel pokemon center"} {
		dest, ok := Place(name)
		if !ok {
			t.Fatalf("missing place %q", name)
		}
		if err := planner.Reachability(dest); err != nil {
			t.Errorf("Reachability(%s) from the Power Plant pocket: %v", name, err)
		}
	}
}
