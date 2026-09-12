package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/romtool"
)

func TestParseRegionsAndLabel(t *testing.T) {
	regions, err := parseRegions("wram:C000-DFFF,hram:FF80-FFFE")
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 2 || regions[0].Start != 0xC000 || regions[1].End != 0xFFFE {
		t.Fatalf("regions=%+v", regions)
	}
	label, err := parseLabel("wram@0xD362=player.x:tile coordinate")
	if err != nil {
		t.Fatal(err)
	}
	if label.Region != "wram" || label.Address != 0xD362 || label.Name != "player.x" || label.Note != "tile coordinate" {
		t.Fatalf("label=%+v", label)
	}
}

func TestDiffCommandJSONAndSymbols(t *testing.T) {
	dir := t.TempDir()
	before := testSnapshot("before", 0)
	after := testSnapshot("after", 1)
	bp := filepath.Join(dir, "before.json")
	ap := filepath.Join(dir, "after.json")
	sp := filepath.Join(dir, "symbols.json")
	writeSnap(t, bp, before)
	writeSnap(t, ap, after)
	var out bytes.Buffer
	err := run([]string{"diff", "-json", "-delta", "1", "-label", "0xC000=player.x:test", "-symbols", sp, bp, ap}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"address_hex": "0xC000"`) {
		t.Fatalf("output=%s", out.String())
	}
	b, err := os.ReadFile(sp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"name": "player.x"`) {
		t.Fatalf("symbols=%s", b)
	}
}

func testSnapshot(name string, v byte) romtool.Snapshot {
	return romtool.Snapshot{SchemaVersion: romtool.SchemaVersion, Name: name, ROM: romtool.ROMIdentity{SHA256: "abc"}, Banks: romtool.BankContext{WRAMBank: 1}, Regions: []romtool.RegionSnapshot{{Name: "wram", Start: 0xC000, End: 0xC000, Data: []byte{v}}}}
}
func writeSnap(t *testing.T, path string, s romtool.Snapshot) {
	t.Helper()
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if e := romtool.WriteSnapshot(f, s); e != nil {
		t.Fatal(e)
	}
}
