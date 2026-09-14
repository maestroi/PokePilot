package starter

import (
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
