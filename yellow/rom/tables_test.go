package rom

import (
	"testing"

	"github.com/maestroi/pokepilot/gen1rom"
	"github.com/maestroi/pokepilot/gen1rom/symtest"
	redrom "github.com/maestroi/pokepilot/red/rom"
)

func TestYellowTablesMatchDecompSymbols(t *testing.T) {
	symtest.Verify(t, "../../pokeyellow/pokeyellow.sym", symtest.Labels(Tables, "SuperRodFishingSlots"))
	if Tables.SuperRodFormat != gen1rom.SuperRodInline {
		t.Fatal("Yellow's SuperRodFishingSlots is the inline format")
	}
}

func TestYellowTablesBindOnlyTheYellowCartridge(t *testing.T) {
	// A synthetic image is not the Yellow cartridge, so shared decoders keep
	// reading it with Red's layout.
	if _, ok := gen1rom.TableLayoutFor(make([]byte, 0x8000)); ok {
		t.Fatal("synthetic image bound to a registered layout")
	}
	if got := redrom.Tables(make([]byte, 0x8000)).WildDataPointers; got == Tables.WildDataPointers {
		t.Fatalf("synthetic image resolved Yellow's WildDataPointers %+v", got)
	}
}

func TestYellowValidMapsAgreeWithHeaderRefs(t *testing.T) {
	for id := 0; id < Tables.MapCount; id++ {
		if Tables.ValidMap(uint8(id)) != validMapID(uint8(id)) {
			t.Fatalf("map %02x validity diverges", id)
		}
	}
	if !Tables.ValidMap(0xF8) {
		t.Fatal("SUMMER_BEACH_HOUSE is a Yellow map")
	}
}

func TestYellowTrainerEventFlagsTranslateToCanonical(t *testing.T) {
	// Native Yellow event bit 0 is canonical bit 0; both arrays start one
	// byte apart, so the canonical byte is the canonical array base.
	addr, mask, ok := Tables.EventFlag(0xD746, 0)
	if !ok || addr != 0xD747 || mask != 1 {
		t.Fatalf("EventFlag(native wEventFlags,0) = %#04x,%#02x,%v want 0xd747,0x01,true", addr, mask, ok)
	}
}

func TestIsCartridgeNeedsTheProbeAndTheExactImage(t *testing.T) {
	image := make([]byte, 0x100000)
	if IsCartridge(image) {
		t.Fatal("blank image accepted")
	}
	banks, _ := Tables.MapHeaderBanks.Offset()
	ptrs, _ := Tables.MapHeaderPointers.Offset()
	image[banks], image[ptrs], image[ptrs+1] = 0x06, 0xA1, 0x42
	if IsCartridge(image) {
		t.Fatal("structural probe alone accepted an image with the wrong SHA-1")
	}
	if IsCartridge(image[:banks]) {
		t.Fatal("truncated image accepted")
	}
}
