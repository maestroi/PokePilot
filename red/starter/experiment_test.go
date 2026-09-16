package starter

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
)

func TestResolveFixedMewtwo(t *testing.T) {
	sel, err := Resolve("mewtwo", 7)
	if err != nil {
		t.Fatal(err)
	}
	if sel.Mode != ModeFixed || sel.Species != "mewtwo" || sel.Raw != 0x83 || sel.Slot != "squirtle" {
		t.Fatalf("selection = %+v", sel)
	}
}

func TestResolveAliases(t *testing.T) {
	for request, want := range map[string]string{
		"fixed:mr-mime": "mr.mime",
		"farfetchd":     "farfetch'd",
		"nidoran-f":     "nidoran♀",
		"nidoran-m":     "nidoran♂",
	} {
		sel, err := Resolve(request, 0)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", request, err)
		}
		if string(sel.Species) != want {
			t.Fatalf("Resolve(%q) species = %q, want %q", request, sel.Species, want)
		}
	}
}

func TestResolveRandomDeterministic(t *testing.T) {
	for _, pool := range []string{"reasonable", "basic", "any"} {
		a, err := Resolve("random:"+pool, 123456)
		if err != nil {
			t.Fatal(err)
		}
		b, err := Resolve("random:"+pool, 123456)
		if err != nil {
			t.Fatal(err)
		}
		if a.Species != b.Species || a.Raw != b.Raw || a.Pool != b.Pool {
			t.Fatalf("random:%s not deterministic: %+v vs %+v", pool, a, b)
		}
	}
}

func TestResolveRandomDefaultIsReasonable(t *testing.T) {
	a, err := Resolve("random", 99)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Resolve("random:reasonable", 99)
	if err != nil {
		t.Fatal(err)
	}
	if a.Species != b.Species || a.Pool != PoolReasonable {
		t.Fatalf("random default = %+v; explicit reasonable = %+v", a, b)
	}
}

func TestResolveRejectsUnknownPoolAndSpecies(t *testing.T) {
	if _, err := Resolve("random:chaos", 1); err == nil {
		t.Fatal("random:chaos: expected error")
	}
	if _, err := Resolve("missingno", 1); err == nil {
		t.Fatal("missingno: expected error")
	}
}

func TestPatchSupportedROMChangesOnlyOakStarterBytes(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	base, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sel, err := Resolve("mewtwo", 0)
	if err != nil {
		t.Fatal(err)
	}
	patched, info, err := Patch(base, sel)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Changes) != 2 {
		t.Fatalf("changes = %+v, want exactly 2", info.Changes)
	}
	seen := map[int]bool{}
	for _, change := range info.Changes {
		if change.From != 0xb1 || change.To != 0x83 {
			t.Fatalf("change = %+v, want Squirtle byte -> Mewtwo byte", change)
		}
		seen[change.Offset] = true
	}
	for i := range base {
		if base[i] != patched[i] && !seen[i] {
			t.Fatalf("unexpected ROM difference at %#x: %#02x -> %#02x", i, base[i], patched[i])
		}
	}
	if info.BaseSHA1 == info.EffectiveSHA1 {
		t.Fatal("patched ROM hash did not change")
	}
}

func TestReplayROMLeavesVanillaCartridgeUnchanged(t *testing.T) {
	base := []byte{0x10, 0xb1, 0x20}
	got, err := ReplayROM(base, map[string]string{"starter": "squirtle"}, sha256Hex(base))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, base) {
		t.Fatalf("vanilla ReplayROM mutated the cartridge: %x", got)
	}
}

func TestReplayROMAppliesRecordedPatchBytes(t *testing.T) {
	base := []byte{0x10, 0xb1, 0x20, 0xb1}
	want := []byte{0x10, 0x83, 0x20, 0x83}
	got, err := ReplayROM(base, map[string]string{
		"starter":         "mewtwo",
		"rom_patch_bytes": "0x1:b1>83,0x3:b1>83",
	}, sha256Hex(want))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("ReplayROM = %x, want %x", got, want)
	}
}

func TestReplayROMRejectsPatchFromMismatch(t *testing.T) {
	base := []byte{0x10, 0xaa, 0x20}
	_, err := ReplayROM(base, map[string]string{"rom_patch_bytes": "0x1:b1>83"}, sha256Hex([]byte{0x10, 0x83, 0x20}))
	if err == nil {
		t.Fatal("expected error when recorded From byte does not match the base ROM")
	}
}

func TestReplayROMReconstructsMewtwoFromStarterMetadata(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	base, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sel, err := Resolve("mewtwo", 0)
	if err != nil {
		t.Fatal(err)
	}
	want, info, err := Patch(base, sel)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Changes) == 0 {
		t.Fatal("expected Mewtwo patch changes")
	}
	got, err := ReplayROM(base, map[string]string{"starter": "mewtwo", "seed": "0"}, sha256Hex(want))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("ReplayROM(starter=mewtwo) did not reconstruct the recorded cartridge")
	}
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestReplayROMRejectsSHAMismatch(t *testing.T) {
	base := []byte{0x10, 0xb1}
	_, err := ReplayROM(base, nil, sha256Hex([]byte("other")))
	if err == nil {
		t.Fatal("expected sha256 mismatch error")
	}
	if got := fmt.Sprintf("%v", err); got == "" {
		t.Fatal("error message was empty")
	}
}
