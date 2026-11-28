package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

// Defeating one League member is not evidence that a later member was won.
// The Indigo callback resets all per-battle events when starting a new run.
func TestGSLeagueMiddleMemberEventsAreIndependent(t *testing.T) {
	members := []struct {
		progress game.ProgressID
		event    uint16
	}{
		{ProgressLeagueWillDefeated, eventBeatEliteFourWill},
		{ProgressLeagueKogaDefeated, eventBeatEliteFourKoga},
		{ProgressLeagueBrunoDefeated, eventBeatEliteFourBruno},
		{ProgressLeagueKarenDefeated, eventBeatEliteFourKaren},
	}
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		t.Run(string(p.ID()), func(t *testing.T) {
			mem := fakeGSReader{}
			mem[sym.JohtoBadges] = 0xff
			mem[sym.VisitedSpawns+1] = 1 << 5
			// EVENT_BEAT_ELITE_FOUR is the Hall of Fame ceremony, not a
			// substitute for any individual Elite Four member event.
			mem[sym.EventFlags+68/8] |= 1 << (68 % 8)
			for _, member := range members {
				if p.DecodeFirstBadgeProgress(mem).Has(member.progress) {
					t.Fatalf("Hall of Fame marker fabricated %s", member.progress)
				}
			}
			for i, member := range members {
				mem[sym.EventFlags+member.event/8] |= byte(1 << (member.event % 8))
				story := p.DecodeFirstBadgeProgress(mem)
				if !story.Has(member.progress) {
					t.Fatalf("%s event bit missing from projection", member.progress)
				}
				for _, later := range members[i+1:] {
					if story.Has(later.progress) {
						t.Fatalf("%s manufactured later victory %s", member.progress, later.progress)
					}
				}
			}
			for _, member := range members {
				mem[sym.EventFlags+member.event/8] &^= byte(1 << (member.event % 8))
				if p.DecodeFirstBadgeProgress(mem).Has(member.progress) {
					t.Fatalf("League reset did not revoke %s", member.progress)
				}
			}
		})
	}
}
