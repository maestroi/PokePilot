package gen1

import (
	"fmt"
	"testing"
)

func TestDecodeNameAndTiles(t *testing.T) {
	if got := DecodeTiles([]byte{0x8e, 0x80, 0x8a, 0x9c, 0x7f, 0x87, 0x88}); got != "OAK: HI" {
		t.Fatalf("DecodeTiles = %q, want OAK: HI", got)
	}
	if got := DecodeName([]byte{0x91, 0x84, 0x83, 0x50, 0x80, 0x92, 0x87}); got != "RED" {
		t.Fatalf("DecodeName = %q, want RED", got)
	}
}

func TestPresetMenuNames(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"NAME NEW NAME RED ASH JACK First, what is your name?", []string{"RED", "ASH", "JACK"}},
		{"NAME NEW NAME BLUE GARY JOHN ...Erm, what is his name again?", []string{"BLUE", "GARY", "JOHN"}},
	}
	for _, tc := range cases {
		if got := PresetMenuNames(tc.text); fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Fatalf("PresetMenuNames(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestNormalizeDisplayText(t *testing.T) {
	if got := NormalizeDisplayText("AAAAAAAAAAAAA used CUT!"); got != "A×13 used CUT!" {
		t.Fatalf("NormalizeDisplayText = %q", got)
	}
}
