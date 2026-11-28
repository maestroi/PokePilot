package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func gsLeagueCaptureObs(id game.GameID, steps ...ProgressID) Observation {
	facts := make(ProgressState, 0, len(steps))
	for _, id := range steps {
		facts = append(facts, ProgressFact{ID: id, Complete: true})
	}
	return Observation{GameID: id, Controllable: true, Story: facts}
}

func TestGSLeagueQualificationCapturesVerifiedFirstTransitions(t *testing.T) {
	for _, id := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		for _, stage := range gsLeagueCheckpointBoundaries {
			t.Run(string(id)+"/"+stage.directory, func(t *testing.T) {
				before := gsLeagueCaptureObs(id, stage.previous)
				after := gsLeagueCaptureObs(id, stage.previous, stage.progress)
				goal := Objective{Kind: KindProgress, Progress: stage.progress}
				got, ok := gsLeagueCaptureEligible(goal, before, after)
				if !ok || got != stage {
					t.Fatalf("valid first milestone refused: got=%+v ok=%v want=%+v", got, ok, stage)
				}

				cases := []struct {
					name   string
					goal   Objective
					before Observation
					after  Observation
				}{
					{"already had victory", goal, after, after},
					{"no new victory event", goal, before, before},
					{"missing preceding flag", goal, gsLeagueCaptureObs(id), after},
					{"preceding flag vanished", goal, before, gsLeagueCaptureObs(id, stage.progress)},
					{"not a progress objective", Objective{Kind: KindHeal, Progress: stage.progress}, before, after},
					{"wrong progress objective", Objective{Kind: KindProgress, Progress: stage.previous}, before, after},
					{"pre-cutscene not controllable", goal, func() Observation { v := before; v.Controllable = false; return v }(), after},
					{"post-cutscene not controllable", goal, before, func() Observation { v := after; v.Controllable = false; return v }()},
					{"battle underway", goal, before, func() Observation { v := after; v.InBattle = true; return v }()},
					{"Hall already completed", goal, gsLeagueCaptureObs(id, stage.previous, gsprofile.ProgressLeagueHallOfFameEntered), after},
					{"rematch after Hall", goal, before, gsLeagueCaptureObs(id, stage.previous, stage.progress, gsprofile.ProgressLeagueHallOfFameEntered)},
					{"cross-version replay", goal, before, func() Observation {
						v := after
						if id == gsprofile.GoldGameID {
							v.GameID = gsprofile.SilverGameID
						} else {
							v.GameID = gsprofile.GoldGameID
						}
						return v
					}()},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						if selected, ok := gsLeagueCaptureEligible(tc.goal, tc.before, tc.after); ok {
							t.Fatalf("incorrect qualification acceptance %+v", selected)
						}
					})
				}
			})
		}
	}
}

func TestGSLeagueQualificationBoundariesMatchRetailExitRooms(t *testing.T) {
	// Indigo is the Route 23 flypoint set by its MAPCALLBACK_NEWMAP. The
	// Indigo Pokecenter is reached *later* in the Will objective.
	want := map[string]string{
		"post-rising": "DRAGONS_DEN_B1F",
		"post-indigo": "ROUTE_23",
		"post-will":   "WILLS_ROOM",
		"post-koga":   "KOGAS_ROOM",
		"post-bruno":  "BRUNOS_ROOM",
		"post-karen":  "KARENS_ROOM",
	}
	if len(gsLeagueCheckpointBoundaries) != len(want) {
		t.Fatalf("checkpoint entries=%d want %d", len(gsLeagueCheckpointBoundaries), len(want))
	}
	seen := map[string]bool{}
	for _, stage := range gsLeagueCheckpointBoundaries {
		mapName, ok := want[stage.directory]
		if !ok || mapName != stage.mapName {
			t.Fatalf("wrong native qualification boundary %+v", stage)
		}
		if seen[stage.directory] {
			t.Fatalf("duplicated directory: %s", stage.directory)
		}
		seen[stage.directory] = true
		if _, err := gsOpeningMapID(stage.mapName); err != nil {
			t.Fatalf("unresolved retail map %s: %v", stage.mapName, err)
		}
	}
}
