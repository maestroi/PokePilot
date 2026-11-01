package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/gs/sym"
)

// Hall of Fame is not a synonym for a Lance battle victory: only its own
// ceremony sets EVENT_BEAT_ELITE_FOUR (68). Indigo resets Lance's event
// (1468) on another challenge, but not the historical Hall of Fame flag.
func TestGSChampionAndHallOfFameRequireIndependentRetailEvents(t *testing.T) {
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		t.Run(string(p.ID()), func(t *testing.T) {
			mem := fakeGSReader{}
			mem[sym.JohtoBadges] = 0xff
			mem[sym.VisitedSpawns+1] = 1 << 5
			for _, event := range []uint16{
				eventBeatEliteFourWill,
				eventBeatEliteFourKoga,
				eventBeatEliteFourBruno,
				eventBeatEliteFourKaren,
			} {
				mem[sym.EventFlags+event/8] |= byte(1 << (event % 8))
			}
			if p.DecodeFirstBadgeProgress(mem).Has(ProgressLeagueLanceDefeated) ||
				p.DecodeFirstBadgeProgress(mem).Has(ProgressLeagueHallOfFameEntered) {
				t.Fatal("four Elite Four wins cannot manufacture Champion or Hall of Fame")
			}
			mem[sym.EventFlags+eventBeatChampionLance/8] |= byte(1 << (eventBeatChampionLance % 8))
			if !p.DecodeFirstBadgeProgress(mem).Has(ProgressLeagueLanceDefeated) {
				t.Fatal("Lance's retail victory flag did not project")
			}
			if p.DecodeFirstBadgeProgress(mem).Has(ProgressLeagueHallOfFameEntered) {
				t.Fatal("Lance's trainer flag cannot count as Hall of Fame")
			}
			mem[sym.EventFlags+eventBeatEliteFour/8] |= byte(1 << (eventBeatEliteFour % 8))
			if !p.DecodeFirstBadgeProgress(mem).Has(ProgressLeagueHallOfFameEntered) {
				t.Fatal("Hall ceremony event did not project")
			}
			mem[sym.EventFlags+eventBeatChampionLance/8] &^= byte(1 << (eventBeatChampionLance % 8))
			if p.DecodeFirstBadgeProgress(mem).Has(ProgressLeagueLanceDefeated) {
				t.Fatal("fresh Indigo League attempt must clear Lance's battle victory")
			}
			if !p.DecodeFirstBadgeProgress(mem).Has(ProgressLeagueHallOfFameEntered) {
				t.Fatal("historical Hall of Fame flag must persist after Lance flag resets")
			}
		})
	}
}
