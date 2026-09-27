package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
)

func TestProfileSatisfiesCartridgeContract(t *testing.T) {
	if err := game.ValidateCartridgeProfileContract(New()); err != nil {
		t.Fatal(err)
	}
}

func TestDetectExactWorldRev1Fingerprint(t *testing.T) {
	p := New()
	if !p.Detect(game.ROMInfo{SHA1: ROMSHA1, SHA256: ROMSHA256}) {
		t.Fatal("expected exact Tetris World Rev 1 fingerprint to match")
	}
	for _, info := range []game.ROMInfo{
		{SHA1: ROMSHA1},
		{SHA256: ROMSHA256},
		{SHA1: "0000000000000000000000000000000000000000", SHA256: ROMSHA256},
		{SHA1: ROMSHA1, SHA256: "0000000000000000000000000000000000000000000000000000000000000000"},
	} {
		if p.Detect(info) {
			t.Fatalf("unexpected partial/mismatched fingerprint match: %#v", info)
		}
	}
}
