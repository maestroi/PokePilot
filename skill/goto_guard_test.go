package skill

import (
	"errors"
	"strings"
	"testing"
)

func TestNavigationGuardRejectsRepeatedState(t *testing.T) {
	dest := Destination{Map: 0x02, X: 14, Y: 8}
	start := navigationState{Map: 0x0D, X: 3, Y: 43}
	g := newNavigationGuard(dest, start)

	gate := navigationState{Map: 0x32, X: 4, Y: 7}
	if err := g.observe(gate); err != nil {
		t.Fatalf("first transition: %v", err)
	}
	err := g.observe(start)
	if !errors.Is(err, ErrNavigationStalled) {
		t.Fatalf("repeat error = %v, want ErrNavigationStalled", err)
	}
	for _, want := range []string{"map 0d at (3,43)", "map 02 at (14,8)", "2 transition", "0d(3,43) -> 32(4,7) -> 0d(3,43)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}

func TestNavigationGuardAllowsSameMapAtDifferentTile(t *testing.T) {
	g := newNavigationGuard(Destination{Map: 0x02}, navigationState{Map: 0x0D, X: 3, Y: 43})
	for _, s := range []navigationState{
		{Map: 0x32, X: 4, Y: 7},
		{Map: 0x33, X: 17, Y: 47},
		{Map: 0x2F, X: 4, Y: 7},
		{Map: 0x0D, X: 3, Y: 11},
	} {
		if err := g.observe(s); err != nil {
			t.Fatalf("observe %+v: %v", s, err)
		}
	}
}

func TestNavigationGuardBoundsSuccessfulTransitions(t *testing.T) {
	g := newNavigationGuard(Destination{Map: 0xF0}, navigationState{})
	for i := 1; i <= maxNavigationTransitions; i++ {
		if err := g.observe(navigationState{Map: uint8(i)}); err != nil {
			t.Fatalf("transition %d: %v", i, err)
		}
	}
	err := g.observe(navigationState{Map: maxNavigationTransitions + 1})
	if !errors.Is(err, ErrNavigationStalled) {
		t.Fatalf("transition ceiling error = %v, want ErrNavigationStalled", err)
	}
	if !strings.Contains(err.Error(), "exceeded 64 successful map transitions") {
		t.Fatalf("ceiling error = %q", err)
	}
}
