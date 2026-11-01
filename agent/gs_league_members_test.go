package agent

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

// The middle Elite Four share one controller but must never share an event
// or a predecessor; each member's retail victory is separately observable.
func TestGSLeagueMiddleMembersFormSequentialOwnedMilestones(t *testing.T) {
	want := []struct {
		progress game.ProgressID
		previous game.ProgressID
		room string
		previousRoom string
		encounter string
	}{
		{gsprofile.ProgressLeagueKogaDefeated, gsprofile.ProgressLeagueWillDefeated, "KOGAS_ROOM", "WILLS_ROOM", "league:koga"},
		{gsprofile.ProgressLeagueBrunoDefeated, gsprofile.ProgressLeagueKogaDefeated, "BRUNOS_ROOM", "KOGAS_ROOM", "league:bruno"},
		{gsprofile.ProgressLeagueKarenDefeated, gsprofile.ProgressLeagueBrunoDefeated, "KARENS_ROOM", "BRUNOS_ROOM", "league:karen"},
	}
	for _, item := range want {
		spec, ok := gsLeagueMemberForProgress(item.progress)
		if !ok || spec.progress != item.progress || spec.previous != item.previous ||
			spec.room != item.room || spec.previousRoom != item.previousRoom || spec.encounter != item.encounter {
			t.Fatalf("bad League member %s: %+v found=%v", item.progress, spec, ok)
		}
		if !gsSupportedProgress(item.progress) {
			t.Fatalf("League member %s is missing its executable controller", item.progress)
		}
		if _, err := gsOpeningMapID(item.room); err != nil {
			t.Fatalf("unregistered League room %s: %v", item.room, err)
		}
	}
	if _, ok := gsLeagueMemberForProgress("gs_league_lance_defeated"); ok {
		t.Fatal("Champion Lance is not implemented by the Elite Four room executor")
	}
	if !errors.Is(leagueMemberUnavailable(t), errGSControllerUnavailable) {
		t.Fatal("unknown League progress should fail closed")
	}
}

func leagueMemberUnavailable(t *testing.T) error {
	t.Helper()
	return executeGSLeagueMember(nil, nil, "gs_league_lance_defeated")
}
