package agent

import (
	"errors"
	"fmt"
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

// The middle Elite Four share one controller but must never share an event
// or a predecessor; each member's retail victory is separately observable.
func TestGSLeagueMiddleMembersFormSequentialOwnedMilestones(t *testing.T) {
	want := []struct {
		progress     game.ProgressID
		previous     game.ProgressID
		room         string
		previousRoom string
		encounter    string
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

// A League whiteout is a lost attempt, not an unowned state: the reset must
// stay recoverable so the planner restarts from Will (run-zzrclw1r60a8g).
func TestGSLeagueAttemptResetIsRecoverableBlock(t *testing.T) {
	adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
	err := fmt.Errorf("agent: progress: %w: league:karen", errGSLeagueAttemptReset)
	failure := adapter.NormalizeFailure(game.FailurePhaseExecution, err, Observation{})
	if failure.Class != game.FailureClassBlocked || !failure.Recoverable || failure.Cause != "gen2_league_attempt_reset" {
		t.Fatalf("failure = %+v, want recoverable League reset block", failure)
	}
	route23, err := gsOpeningMapID("ROUTE_23")
	if err != nil || !gsLeagueApproachMap(route23) {
		t.Fatalf("Route 23 whiteout spawn must be an owned League approach map: %v", err)
	}
}
