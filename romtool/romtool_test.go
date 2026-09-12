package romtool

import (
	"bytes"
	"strings"
	"testing"
)

type fakeMemory [1 << 16]byte

func (m *fakeMemory) Peek8(addr uint16) byte           { return m[addr] }
func (m *fakeMemory) PeekInto(addr uint16, dst []byte) { copy(dst, m[int(addr):int(addr)+len(dst)]) }

func TestCaptureDiffAndBankContext(t *testing.T) {
	var m fakeMemory
	m[0xFF70] = 3
	before, err := Capture(&m, "before", CaptureMeta{ROM: ROMIdentity{SHA256: "abc"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m[0xD361] = 7
	m[0xFF90] = 2
	after, err := Capture(&m, "after", CaptureMeta{ROM: ROMIdentity{SHA256: "abc"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := Diff(before, after, DiffFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("len=%d %#v", len(changes), changes)
	}
	if changes[0].Address != 0xD361 || changes[0].Bank == nil || *changes[0].Bank != 3 {
		t.Fatalf("wram change=%+v", changes[0])
	}
	if changes[1].Address != 0xFF90 || changes[1].Bank == nil || *changes[1].Bank != 0 {
		t.Fatalf("hram change=%+v", changes[1])
	}
}

func TestDiffFiltersValuesAndRange(t *testing.T) {
	var m fakeMemory
	before, _ := Capture(&m, "before", CaptureMeta{}, nil)
	m[0xC010] = 1
	m[0xC011] = 2
	after, _ := Capture(&m, "after", CaptureMeta{}, nil)
	start, end := uint16(0xC010), uint16(0xC010)
	delta := int16(1)
	changes, err := Diff(before, after, DiffFilter{Region: "wram", Start: &start, End: &end, Delta: &delta})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Address != 0xC010 {
		t.Fatalf("changes=%+v", changes)
	}
}

func TestRankRepeatedExperimentsAndControlNoise(t *testing.T) {
	bank := 0
	experiments := []Experiment{
		{Name: "right-1", Changes: []Change{{Region: "wram", Address: 0xD362, AddressHex: "0xD362", Bank: &bank, Before: 4, After: 5, Delta: 1}, {Region: "hram", Address: 0xFF90, AddressHex: "0xFF90", Before: 1, After: 2, Delta: 1}}},
		{Name: "right-2", Changes: []Change{{Region: "wram", Address: 0xD362, AddressHex: "0xD362", Bank: &bank, Before: 5, After: 6, Delta: 1}, {Region: "hram", Address: 0xFF90, AddressHex: "0xFF90", Before: 2, After: 3, Delta: 1}}},
		{Name: "down", Changes: []Change{{Region: "wram", Address: 0xD361, AddressHex: "0xD361", Bank: &bank, Before: 8, After: 9, Delta: 1}}},
	}
	exclude := map[string]struct{}{ChangeKey("hram", 0xFF90): {}}
	got := Rank(experiments, exclude)
	if len(got) != 2 {
		t.Fatalf("candidates=%+v", got)
	}
	if got[0].Address != 0xD362 || got[0].StableDelta == nil || *got[0].StableDelta != 1 || !got[0].ConsistentDelta {
		t.Fatalf("top=%+v", got[0])
	}
	if got[1].Address != 0xD361 {
		t.Fatalf("second=%+v", got[1])
	}
}

func TestExportSymbols(t *testing.T) {
	bank := 0
	candidates := []Candidate{{Region: "wram", Address: 0xD362, AddressHex: "0xD362", Bank: &bank}}
	sf, err := ExportSymbols(ROMIdentity{SHA256: "abc"}, candidates, []Label{{Address: 0xD362, Name: "player.x", Note: "tile x"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(sf.Symbols) != 1 || sf.Symbols[0].Name != "player.x" {
		t.Fatalf("symbols=%+v", sf.Symbols)
	}
}

func TestSnapshotJSONRoundTrip(t *testing.T) {
	var m fakeMemory
	snap, _ := Capture(&m, "baseline", CaptureMeta{Frame: 7}, nil)
	var buf bytes.Buffer
	if err := WriteSnapshot(&buf, snap); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"name": "baseline"`) {
		t.Fatalf("json=%s", buf.String())
	}
	got, err := ReadSnapshot(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got.Frame != 7 || len(got.Regions) != 2 {
		t.Fatalf("got=%+v", got)
	}
}
