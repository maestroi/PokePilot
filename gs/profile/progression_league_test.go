package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/gs/sym"
)

// The flypoint is not a League victory. Neither eight badges, Indigo access,
// nor unrelated events may manufacture Will's trainer-completion event.
func TestGSWillLeagueProgressRequiresCartridgeBattleEvent(t *testing.T) {
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		mem := fakeGSReader{}
		mem[sym.JohtoBadges] = 0xff
		mem[sym.VisitedSpawns+1] = 1 << 5 // SPAWN_INDIGO
		if !p.DecodeFirstBadgeProgress(mem).Has(ProgressIndigoPlateauReached) {
			t.Fatal("precondition: Indigo flypoint missing")
		}
		if p.DecodeFirstBadgeProgress(mem).Has(ProgressLeagueWillDefeated) {
			t.Fatal("Indigo and eight badges cannot count as beating Will")
		}
		mem[sym.EventFlags+eventBeatEliteFourWill/8] |= byte(1 << (eventBeatEliteFourWill % 8))
		if !p.DecodeFirstBadgeProgress(mem).Has(ProgressLeagueWillDefeated) {
			t.Fatal("retail EVENT_BEAT_ELITE_4_WILL was not projected")
		}
		mem[sym.EventFlags+eventBeatEliteFourWill/8] = 0
		if p.DecodeFirstBadgeProgress(mem).Has(ProgressLeagueWillDefeated) {
			t.Fatal("entering Indigo for another challenge must clear Will completion")
		}
	}
}
