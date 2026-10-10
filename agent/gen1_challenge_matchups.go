package agent

import (
	"sync"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/skill"
)

// gen1ChallengeSpec links one Gen-I challenge objective to the map its battle
// happens on. That link is the only hand-kept fact: the opposing team, its
// levels and its type matchup are read from the cartridge (map objects ->
// trainer class/set -> TrainerDataPointers -> base-stat types -> TypeEffects),
// so Red, Blue and Yellow each get their own teams.
//
// scriptedMaxLevel covers fights whose trainer is spawned by a map script
// rather than placed as a map object (the rival, whose party also depends on
// the starter, and Yellow's Jessie & James). Those keep a level-only floor
// until the adapter models scripted trainers; they carry no type matchup.
type gen1ChallengeSpec struct {
	objective        Objective
	mapID            uint8
	scriptedMaxLevel int
}

// Map ids are pokered/pokeyellow constants/map_constants.asm.
func gen1ChallengeSpecs() []gen1ChallengeSpec {
	progress := func(id ProgressID, mapID uint8, scripted int) gen1ChallengeSpec {
		return gen1ChallengeSpec{objective: Objective{Kind: KindProgress, Progress: id}, mapID: mapID, scriptedMaxLevel: scripted}
	}
	specs := []gen1ChallengeSpec{
		progress(gen1.ProgressBoulderBadge, 0x36, 0),               // PEWTER_GYM
		progress(gen1.ProgressThunderBadge, 0x5C, 0),               // VERMILION_GYM
		progress(gen1.ProgressRainbowBadge, 0x86, 0),               // CELADON_GYM
		progress(gen1.ProgressFuchsiaProgressionComplete, 0x9D, 0), // FUCHSIA_GYM (Koga)
		progress(gen1.ProgressVolcanoBadge, 0xA6, 0),               // CINNABAR_GYM
		progress(gen1.ProgressEarthBadge, 0x2D, 0),                 // VIRIDIAN_GYM
		progress(gen1.ProgressSilphScopeAcquired, 0xCA, 0),         // ROCKET_HIDEOUT_B4F (Giovanni)
		progress(gen1.ProgressSilphRescueComplete, 0xEB, 0),        // SILPH_CO_11F (Giovanni)
		progress(gen1.ProgressPokeFluteAcquired, 0x94, 30),         // POKEMON_TOWER_7F; Yellow's are scripted
		progress(gen1.ProgressLeagueLoreleiDefeated, 0xF5, 0),      // LORELEIS_ROOM
		progress(gen1.ProgressLeagueBrunoDefeated, 0xF6, 0),        // BRUNOS_ROOM
		progress(gen1.ProgressLeagueAgathaDefeated, 0xF7, 0),       // AGATHAS_ROOM
		progress(gen1.ProgressLeagueLanceDefeated, 0x71, 0),        // LANCES_ROOM
		// Scripted rival fights: no map trainer object to read.
		progress(gen1.ProgressHM01Acquired, 0, 20),
		progress(gen1.ProgressRoute22RivalResolved, 0, 53),
		progress(gen1.ProgressLeagueChampionDefeated, 0, 65),
	}
	for _, gym := range skill.Gyms() {
		specs = append(specs, gen1ChallengeSpec{objective: Objective{Kind: KindGym, Place: PlaceID(gym.Place)}, mapID: gym.Map})
	}
	return specs
}

// readinessFloorForLevel maps a team's top level onto the weighted
// party-readiness scale (a lead at that level counts four times).
func readinessFloorForLevel(maxEnemyLevel int) int {
	if maxEnemyLevel <= 0 {
		return 0
	}
	return maxEnemyLevel * 4
}

type gen1ChallengeKey struct {
	header [0x1c]byte
	n      int
}

var gen1ChallengeCache sync.Map // gen1ChallengeKey -> []CatalogChallengeProfile

// gen1ChallengeProfiles derives every challenge profile from romData. A map
// whose objects cannot be read yields no profile rather than a guess.
func gen1ChallengeProfiles(romData []byte) []CatalogChallengeProfile {
	if len(romData) < 0x150 {
		return nil
	}
	key := gen1ChallengeKey{n: len(romData)}
	copy(key.header[:], romData[0x134:0x150])
	if v, ok := gen1ChallengeCache.Load(key); ok {
		return v.([]CatalogChallengeProfile)
	}
	chart, err := rom.NewTypeChart(romData)
	if err != nil {
		return nil
	}
	var out []CatalogChallengeProfile
	for _, spec := range gen1ChallengeSpecs() {
		profile := ChallengeReadinessProfile{}
		if spec.mapID != 0 {
			if opponents, ok, err := rom.MapChallengeOpponents(romData, spec.mapID); err == nil && ok {
				profile = matchupReadinessProfile(game.AssessMatchup(chart, opponents))
			}
		}
		if !profile.Matchup.Known() && spec.scriptedMaxLevel > 0 {
			profile.MinimumReadiness = readinessFloorForLevel(spec.scriptedMaxLevel)
		}
		if challengeProfileKnown(profile) {
			out = append(out, CatalogChallengeProfile{Objective: spec.objective.Key(), Readiness: profile})
		}
	}
	gen1ChallengeCache.Store(key, out)
	return out
}

// matchupReadinessProfile is the generic projection of a ROM matchup onto the
// readiness profile every game shares.
func matchupReadinessProfile(m game.Matchup) ChallengeReadinessProfile {
	profile := ChallengeReadinessProfile{MinimumReadiness: readinessFloorForLevel(m.MaxLevel), Matchup: m}
	for _, t := range m.Preferred {
		profile.PreferredMoveTypes = append(profile.PreferredMoveTypes, string(t))
	}
	return profile
}

// challengeProfileIn finds the derived profile for a challenge objective.
func challengeProfileIn(profiles []CatalogChallengeProfile, o Objective) ChallengeReadinessProfile {
	want := combatRecoveryObjective(o).Key()
	for _, p := range profiles {
		if p.Objective == want {
			return p.Readiness
		}
	}
	return ChallengeReadinessProfile{}
}
