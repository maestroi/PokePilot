package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

// DecodeMenu is a raw decode: it reports whatever the RAM says. It does not
// gate on FontLoaded; a caller that cares whether a menu is open checks
// FontLoaded itself.
func TestDecodeMenuRawValues(t *testing.T) {
	var m Mem
	m[sym.CurrentMenuItem] = 2
	m[sym.MaxMenuItem] = 7

	ms := DecodeMenu(&m)
	if ms.Current != 2 || ms.Max != 7 {
		t.Errorf("DecodeMenu = %+v, want {Current:2 Max:7}", ms)
	}
}

// A two-item yes/no box is an ordinary menu: index 0 = YES, index 1 = NO,
// and wMaxMenuItem is the highest valid index, INCLUSIVE. It reports 1,
// not 2.
func TestDecodeMenuYesNoShape(t *testing.T) {
	var m Mem
	m[sym.CurrentMenuItem] = 0
	m[sym.MaxMenuItem] = 1

	ms := DecodeMenu(&m)
	if ms.Current != 0 || ms.Max != 1 {
		t.Errorf("DecodeMenu = %+v, want yes/no shape {Current:0 Max:1}", ms)
	}
}
