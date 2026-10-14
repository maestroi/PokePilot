package agent

import (
	"sync"
	"unsafe"

	"github.com/maestroi/pokepilot/game"
	gsdata "github.com/maestroi/pokepilot/gs/data"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	gsrom "github.com/maestroi/pokepilot/gs/rom"
)

// The scripted leader rosters are the stable parties in
// pokegold/data/trainers/parties.asm. The type bytes, including Steel,
// are always read from this cartridge's BaseData, and effectiveness comes
// exclusively from this cartridge's TypeMatchups. Scripted rivals with
// starter-dependent teams are deliberately not given a guessed profile.
type gsChallengeMon struct {
	species uint8 // National Dex order (Gen-II native species byte)
	level   int
}

type gsChallengeSpec struct {
	progress ProgressID
	roster   []gsChallengeMon
}

func gsChallengeSpecs() []gsChallengeSpec {
	return []gsChallengeSpec{
		{gsprofile.ProgressZephyrBadgeEarned, []gsChallengeMon{{0x10, 7}, {0x11, 9}}},             // Falkner
		{gsprofile.ProgressHiveBadgeEarned, []gsChallengeMon{{0x0b, 14}, {0x0e, 14}, {0x7b, 16}}}, // Bugsy
		{gsprofile.ProgressPlainBadgeEarned, []gsChallengeMon{{0x23, 18}, {0xf1, 20}}},            // Whitney
		{gsprofile.ProgressFogBadgeEarned, []gsChallengeMon{{0x5c, 21}, {0x5d, 21}, {0x5e, 25}, {0x5d, 23}}}, // Morty
		{gsprofile.ProgressStormBadgeEarned, []gsChallengeMon{{0x39, 27}, {0x3e, 30}}},            // Chuck
		{gsprofile.ProgressMineralBadgeEarned, []gsChallengeMon{{0x51, 30}, {0x51, 30}, {0xd0, 35}}}, // Jasmine
	}
}

// Matchup profiles are immutable per loaded ROM. Observation occurs often,
// so scanning BaseData and the chart on every observation would be wasteful.
type gsChallengeROMKey struct {
	ptr    uintptr
	n      int
	header [0x1c]byte
}

var gsChallengeCache sync.Map // gsChallengeROMKey -> []CatalogChallengeProfile

func gsChallengeProfiles(romData []byte) []CatalogChallengeProfile {
	if len(romData) < 0x150 {
		return nil
	}
	key := gsChallengeROMKey{ptr: uintptr(unsafe.Pointer(unsafe.SliceData(romData))), n: len(romData)}
	copy(key.header[:], romData[0x134:0x150])
	if value, ok := gsChallengeCache.Load(key); ok {
		return value.([]CatalogChallengeProfile)
	}
	chart, err := gsrom.NewTypeChart(romData)
	if err != nil {
		return nil
	}
	base, err := gsrom.LocateBaseData(romData)
	if err != nil {
		return nil
	}
	var profiles []CatalogChallengeProfile
	for _, spec := range gsChallengeSpecs() {
		var opponents []game.ChallengeOpponent
		for _, mon := range spec.roster {
			id, ok := gsdata.Species(mon.species)
			if !ok {
				opponents = nil
				break
			}
			types, err := gsrom.SpeciesTypesAt(romData, base, mon.species)
			if err != nil {
				opponents = nil
				break
			}
			opponents = append(opponents, game.ChallengeOpponent{
				Species: id, Level: mon.level, Types: types,
			})
		}
		if len(opponents) != len(spec.roster) {
			continue // no invented matchup from a malformed/unsupported ROM
		}
		objective := Objective{Kind: KindProgress, Progress: spec.progress}
		profiles = append(profiles, CatalogChallengeProfile{
			Objective: objective.Key(),
			Readiness: matchupReadinessProfile(game.AssessMatchup(chart, opponents)),
		})
	}
	gsChallengeCache.Store(key, profiles)
	return profiles
}
