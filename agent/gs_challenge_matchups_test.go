package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func gsSyntheticChallengeROM() []byte {
	const base = 0x800
	rom := make([]byte, base+251*32+0x100)
	for species := 1; species <= 251; species++ {
		off := base + (species-1)*32
		rom[off] = uint8(species)
		rom[off+7] = 0 // Normal for members this small fixture does not specialize
		rom[off+8] = 0
	}
	// The native base-data entry, not a hardcoded challenge chart, gives
	// Jasmine's Steel/Ground Steelix and Electric/Steel Magnemite.
	rom[base+(0xd0-1)*32+7] = 9
	rom[base+(0xd0-1)*32+8] = 4
	rom[base+(0x51-1)*32+7] = 23
	rom[base+(0x51-1)*32+8] = 9
	chart := []byte{0, 5, 5, 0, 9, 5}
	for i := 0; i < 70; i++ {
		chart = append(chart, 1, 5, 5)
	}
	chart = append(chart, 20, 9, 20, 27, 24, 20, 24, 27, 0, 1, 27, 20, 0xff)
	copy(rom[0x100:], chart)
	return rom
}

func TestGSChallengeProfilesReachGenericReadiness(t *testing.T) {
	profiles := gsChallengeProfiles(gsSyntheticChallengeROM())
	if len(profiles) != 8 {
		t.Fatalf("Gold/Silver gym profiles = %d, want eight Johto leaders", len(profiles))
	}
	obs := Observation{
		GameID: gsprofile.GoldGameID, PartyCount: 1,
		ChallengeProfiles: profiles,
	}
	jasmine := Objective{Kind: KindProgress, Progress: gsprofile.ProgressMineralBadgeEarned}
	matchup := challengeProfileFor(obs, jasmine)
	if matchup.MinimumReadiness != readinessFloorForLevel(35) {
		t.Fatalf("Jasmine readiness floor = %d, want level 35", matchup.MinimumReadiness)
	}
	if !matchup.Matchup.Prefers("fire") || matchup.Matchup.Prefers("normal") {
		t.Fatalf("Jasmine preferred types = %v, want fire but not normal", matchup.Matchup.Preferred)
	}
	if len(matchup.Matchup.Opponents) != 3 {
		t.Fatalf("Jasmine party = %+v", matchup.Matchup.Opponents)
	}
	steelix := matchup.Matchup.Opponents[2]
	if steelix.Species != "steelix" || steelix.Level != 35 ||
		len(steelix.Types) != 2 || steelix.Types[0] != game.TypeID("steel") ||
		steelix.Types[1] != game.TypeID("ground") {
		t.Fatalf("Steelix uses wrong ROM types: %+v", steelix)
	}
}

func TestGSFutureGymReadinessUsesPinnedRetailRosters(t *testing.T) {
	obs := Observation{
		GameID: gsprofile.GoldGameID, PartyCount: 1,
		ChallengeProfiles: gsChallengeProfiles(gsSyntheticChallengeROM()),
	}
	for _, tc := range []struct {
		progress    game.ProgressID
		level       int
		party       int
		lastSpecies string
	}{
		{gsprofile.ProgressGlacierBadgeEarned, 31, 3, "piloswine"},
		{gsprofile.ProgressRisingBadgeEarned, 40, 4, "kingdra"},
	} {
		matchup := challengeProfileFor(obs, Objective{Kind: KindProgress, Progress: tc.progress})
		if matchup.MinimumReadiness != readinessFloorForLevel(tc.level) {
			t.Fatalf("%s readiness floor = %d, want level %d", tc.progress, matchup.MinimumReadiness, tc.level)
		}
		if len(matchup.Matchup.Opponents) != tc.party || matchup.Matchup.Opponents[tc.party-1].Species != game.SpeciesID(tc.lastSpecies) {
			t.Fatalf("%s opponents = %+v, want %d ending in %s", tc.progress, matchup.Matchup.Opponents, tc.party, tc.lastSpecies)
		}
	}
}

func TestGSChallengeProfilesFailClosedForUnknownChart(t *testing.T) {
	rom := gsSyntheticChallengeROM()
	// Remove the unique chart's required first row. No Gen-I fallback.
	rom[0x100] = 0x77
	if profiles := gsChallengeProfiles(rom); len(profiles) != 0 {
		t.Fatalf("malformed ROM unexpectedly produced profiles: %+v", profiles)
	}
}

func TestSilverPlannerUsesSameNativeMatchupContract(t *testing.T) {
	obs := Observation{
		GameID: gsprofile.SilverGameID, PartyCount: 1,
		ChallengeProfiles: gsChallengeProfiles(gsSyntheticChallengeROM()),
	}
	falkner := Objective{Kind: KindProgress, Progress: gsprofile.ProgressZephyrBadgeEarned}
	profile := challengeProfileFor(obs, falkner)
	if !profile.Matchup.Known() || profile.MinimumReadiness != readinessFloorForLevel(9) {
		t.Fatalf("Silver Falkner readiness = %+v", profile)
	}
}
