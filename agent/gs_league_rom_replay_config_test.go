package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSLeagueReplaySelectionsHaveExactAuthenticPredecessors(t *testing.T) {
	want := map[string]struct {
		source string
		goals  []game.ProgressID
	}{
		"indigo": {"post-rising", []game.ProgressID{gsprofile.ProgressIndigoPlateauReached}},
		"will":   {"post-indigo", []game.ProgressID{gsprofile.ProgressLeagueWillDefeated}},
		"koga":   {"post-will", []game.ProgressID{gsprofile.ProgressLeagueKogaDefeated}},
		"bruno":  {"post-koga", []game.ProgressID{gsprofile.ProgressLeagueBrunoDefeated}},
		"karen":  {"post-bruno", []game.ProgressID{gsprofile.ProgressLeagueKarenDefeated}},
		"hall-of-fame": {"post-karen", []game.ProgressID{
			gsprofile.ProgressLeagueLanceDefeated, gsprofile.ProgressLeagueHallOfFameEntered,
		}},
	}
	if len(want) != len(gsLeagueReplaySpecs) {
		t.Fatalf("replay stages=%d want %d", len(gsLeagueReplaySpecs), len(want))
	}
	for name, expected := range want {
		source, goals, ok := gsLeagueReplaySelection(name)
		if !ok || source.directory != expected.source || len(goals) != len(expected.goals) {
			t.Fatalf("%s source=%+v goals=%v ok=%v", name, source, goals, ok)
		}
		for i, goal := range goals {
			if goal != expected.goals[i] || !gsSupportedProgress(goal) {
				t.Fatalf("%s replay goal %d=%s want %s, executable", name, i, goal, expected.goals[i])
			}
		}
		if goals[0] == source.progress {
			t.Fatalf("%s replay target equals already completed checkpoint event", name)
		}
	}
	for _, name := range []string{"", "will; rm -rf .", "lance", "post-koga"} {
		if _, _, ok := gsLeagueReplaySelection(name); ok {
			t.Fatalf("invalid replay selector accepted: %q", name)
		}
	}
}

func TestGSLeagueEndingEventsVerifyWithoutOverworldControl(t *testing.T) {
	for _, id := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		adapter := newGSObjectiveAdapter(nil, nil, id)
		for _, tc := range []struct {
			name  string
			prior game.ProgressID
			goal  game.ProgressID
		}{
			{"lance", gsprofile.ProgressLeagueKarenDefeated, gsprofile.ProgressLeagueLanceDefeated},
			{"hall-of-fame", gsprofile.ProgressLeagueLanceDefeated, gsprofile.ProgressLeagueHallOfFameEntered},
		} {
			t.Run(string(id)+"/"+tc.name, func(t *testing.T) {
				before := gsLeagueCaptureObs(id, tc.prior)
				after := gsLeagueCaptureObs(id, tc.prior, tc.goal)
				before.Controllable = true
				after.Controllable = false // legitimate Oak/Mary/Hall script
				o := Objective{Kind: KindProgress, Progress: tc.goal}
				if err := adapter.VerifyPostcondition(o, before, after, ObjectiveResult{}); err != nil {
					t.Fatalf("real ending event rejected while scripted: %v", err)
				}
				scenarios := []struct {
					name   string
					before Observation
					after  Observation
				}{
					{"missing prior event", gsLeagueCaptureObs(id), after},
					{"no victory flag", before, gsLeagueCaptureObs(id, tc.prior)},
					{"already completed before objective", after, after},
					{"still in battle", before, func() Observation { v := after; v.InBattle = true; return v }()},
					{"wrong cartridge", before, func() Observation {
						v := after
						if id == gsprofile.GoldGameID {
							v.GameID = gsprofile.SilverGameID
						} else {
							v.GameID = gsprofile.GoldGameID
						}
						return v
					}()},
				}
				if tc.goal == gsprofile.ProgressLeagueLanceDefeated {
					scenarios = append(scenarios, struct {
						name   string
						before Observation
						after  Observation
					}{"Karen flag reset", before, gsLeagueCaptureObs(id, tc.goal)})
				}
				for _, scenario := range scenarios {
					t.Run(scenario.name, func(t *testing.T) {
						if err := adapter.VerifyPostcondition(o, scenario.before, scenario.after, ObjectiveResult{}); err == nil {
							t.Fatal("unproven Champion/Hall transition accepted")
						}
					})
				}
			})
		}
	}
}
