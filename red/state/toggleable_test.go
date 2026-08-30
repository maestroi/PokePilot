package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestHiddenObjectIDs(t *testing.T) {
	var m Mem
	// Current-map object 2 uses global flag 10; object 4 uses flag 17.
	// Only flag 10 is set. The sentinel must stop decoding before the
	// deliberately set bytes that follow it.
	m[sym.ToggleableObjectList+0] = 2
	m[sym.ToggleableObjectList+1] = 10
	m[sym.ToggleableObjectList+2] = 4
	m[sym.ToggleableObjectList+3] = 17
	m[sym.ToggleableObjectList+4] = 0xff
	m[sym.ToggleableObjectList+5] = 0xff
	m[sym.ToggleableObjectList+6] = 9
	m[sym.ToggleableObjectList+7] = 18
	m[sym.ToggleableObjectFlags+10/8] = 1 << (10 % 8)
	m[sym.ToggleableObjectFlags+18/8] = 1 << (18 % 8)

	got := HiddenObjectIDs(&m)
	if !got[2] || got[4] || got[9] || len(got) != 1 {
		t.Fatalf("HiddenObjectIDs = %v, want only current-map object 2", got)
	}
}
