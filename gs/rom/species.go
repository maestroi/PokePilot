package rom

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
)

// SpeciesName decodes one National Dex name from PokemonNames.
func SpeciesName(rom []byte, species uint8) (string, error) {
	if species == 0 || int(species) > gen2PokemonCount {
		return "", fmt.Errorf("gs/rom: species %#02x is outside 1..%d", species, gen2PokemonCount)
	}
	base, err := locatePokemonNames(rom)
	if err != nil {
		return "", err
	}
	off := base + int(species-1)*pokemonNameLen
	if err := mustInROM(rom, off, pokemonNameLen, "species name"); err != nil {
		return "", err
	}
	return DecodeText(rom[off : off+pokemonNameLen]), nil
}

// Species maps a Gen 2 internal index (National Dex number) onto the
// semantic SpeciesID. The ordering is not Gen 1's and is not reused.
func Species(rom []byte, raw uint8) (game.SpeciesID, bool) {
	id, ok := gsdata.Species(raw)
	if !ok {
		return "", false
	}
	if raw == 0 || int(raw) > gen2PokemonCount {
		return id, true
	}
	name, err := SpeciesName(rom, raw)
	if err != nil {
		return id, true
	}
	if !speciesNameMatches(id, name) {
		return "", false
	}
	return id, true
}

func speciesNameMatches(id game.SpeciesID, decoded string) bool {
	want := strings.ToUpper(strings.ReplaceAll(string(id), "-", ""))
	got := strings.ToUpper(strings.ReplaceAll(decoded, " ", ""))
	got = strings.ReplaceAll(got, "é", "E")
	got = strings.ReplaceAll(got, "É", "E")
	if want == "NIDORANF" {
		return strings.HasPrefix(got, "NIDORAN")
	}
	if want == "NIDORANM" {
		return strings.HasPrefix(got, "NIDORAN")
	}
	if want == "FARFETCHD" {
		return got == "FARFETCH'D" || got == "FARFETCHD"
	}
	if want == "MRMIME" {
		return strings.Contains(got, "MIME")
	}
	if want == "HOOH" {
		return got == "HOOH" || got == "HO-OH"
	}
	return strings.ReplaceAll(got, "'", "") == want || strings.HasPrefix(got, want)
}
