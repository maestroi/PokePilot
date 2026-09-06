package skill

import "testing"

func TestKnownPokemonCenterMapIncludesRegisteredCenters(t *testing.T) {
	for _, name := range []string{"viridian pokemon center", "pewter pokemon center", "cerulean pokemon center", "fuchsia pokemon center"} {
		d, ok := Place(name)
		if !ok {
			t.Fatalf("Place(%q) missing", name)
		}
		if !knownPokemonCenterMap(d.Map) {
			t.Fatalf("knownPokemonCenterMap(%#04x) = false for %s", d.Map, name)
		}
	}
}
