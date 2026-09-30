package farm

import "testing"

// Deterministic replay resolves the cartridge from the failure contract, not
// from one hardcoded Red variable. Recording run-nk4u8m5acmn5 attempt 57 is a
// pokemon-gold run; make debug used to report its replay as "skipped" because
// verification only ever looked at POKEMON_RED_ROM.
func TestReproROMEnvFollowsFailureAdapter(t *testing.T) {
	tests := []struct {
		adapter string
		game    string
		want    string
	}{
		{adapter: "pokemon-gold", game: "pokemon", want: "POKEMON_GOLD_ROM"},
		{adapter: "pokemon-silver", game: "pokemon", want: "POKEMON_SILVER_ROM"},
		{adapter: "pokemon-yellow", game: "pokemon", want: "POKEMON_YELLOW_ROM"},
		{adapter: "pokemon-blue", game: "pokemon", want: "POKEMON_BLUE_ROM"},
		{adapter: "pokemon-red", game: "pokemon", want: "POKEMON_RED_ROM"},
		// Gen-I runs predate the adapter field and only carry the coarse game id.
		{adapter: "", game: "pokemon-red", want: "POKEMON_RED_ROM"},
		{adapter: "", game: "pokemon", want: "POKEMON_RED_ROM"},
		{adapter: "", game: "", want: "POKEMON_RED_ROM"},
	}
	for _, test := range tests {
		if got := ReproROMEnv(test.adapter, test.game); got != test.want {
			t.Errorf("ReproROMEnv(%q, %q) = %q, want %q", test.adapter, test.game, got, test.want)
		}
	}
}

func TestReproROMPathResolvesGameSpecificVariable(t *testing.T) {
	t.Setenv("POKEMON_GOLD_ROM", "/roms/pokemon_gold.gbc")
	t.Setenv("POKEMON_RED_ROM", "/roms/pokemon_red.gb")

	path, env := ReproROMPath("pokemon-gold", "pokemon")
	if env != "POKEMON_GOLD_ROM" {
		t.Fatalf("env = %q, want POKEMON_GOLD_ROM", env)
	}
	if path != "/roms/pokemon_gold.gbc" {
		t.Fatalf("path = %q, want the Gold cartridge, never the Red one", path)
	}

	// An unset variable must be reported by name rather than silently falling
	// back to another cartridge's ROM.
	t.Setenv("POKEMON_GOLD_ROM", "")
	path, env = ReproROMPath("pokemon-gold", "pokemon")
	if path != "" || env != "POKEMON_GOLD_ROM" {
		t.Fatalf("ReproROMPath = (%q, %q), want empty path with POKEMON_GOLD_ROM named", path, env)
	}
}
