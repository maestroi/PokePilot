package agent

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func gsKantoCaptureObs(id game.GameID, johto bool, steps ...ProgressID) Observation {
	obs := gsLeagueCaptureObs(id, steps...)
	obs.Region = string(game.RegionKanto)
	if johto {
		obs.Region = string(game.RegionJohto)
	}
	return obs
}

func TestGSKantoQualificationCapturesFirstVerifiedMilestones(t *testing.T) {
	for _, id := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		for _, stage := range gsKantoCheckpointBoundaries {
			t.Run(string(id)+"/"+stage.directory, func(t *testing.T) {
				// Both the Elm gift and the first ship's boarding start in
				// Johto. Every completed Kanto milestone must be in Kanto.
				beforeJohto := stage.progress == gsprofile.ProgressSSTicketReceived ||
					stage.progress == gsprofile.ProgressKantoFirstLanding
				afterJohto := stage.progress == gsprofile.ProgressSSTicketReceived
				hall := gsprofile.ProgressLeagueHallOfFameEntered
				before := gsKantoCaptureObs(id, beforeJohto, hall, stage.previous)
				after := gsKantoCaptureObs(id, afterJohto, hall, stage.previous, stage.progress)
				goal := Objective{Kind: KindProgress, Progress: stage.progress}
				got, ok := gsKantoCaptureEligible(goal, before, after)
				if !ok || got != stage {
					t.Fatalf("valid retail Kanto transition refused: stage=%+v ok=%t", got, ok)
				}

				type captureCase struct {
					name   string
					goal   Objective
					before Observation
					after  Observation
				}
				cases := []captureCase{
					{"already complete", goal, after, after},
					{"no new event", goal, before, before},
					{"wrong objective kind", Objective{Kind: KindHeal, Progress: stage.progress}, before, after},
					{"wrong objective progress", Objective{Kind: KindProgress, Progress: stage.previous}, before, after},
					{"initial script still running", goal, func() Observation { v := before; v.Controllable = false; return v }(), after},
					{"ending still scripted", goal, before, func() Observation { v := after; v.Controllable = false; return v }()},
					{"still in battle", goal, before, func() Observation { v := after; v.InBattle = true; return v }()},
					{"wrong destination region", goal, before, func() Observation {
						v := after
						if afterJohto {
							v.Region = string(game.RegionKanto)
						} else {
							v.Region = string(game.RegionJohto)
						}
						return v
					}()},
					{"wrong cartridge", goal, before, func() Observation {
						v := after
						if id == gsprofile.GoldGameID {
							v.GameID = gsprofile.SilverGameID
						} else {
							v.GameID = gsprofile.GoldGameID
						}
						return v
					}()},
				}
				// The ticket's immediate predecessor IS Hall of Fame. It is
				// impossible to remove one flag while keeping the other.
				if stage.previous != hall {
					cases = append(cases,
						captureCase{"missing previous event", goal, gsKantoCaptureObs(id, beforeJohto, hall), after},
						captureCase{"previous flag vanished", goal, before, gsKantoCaptureObs(id, afterJohto, hall, stage.progress)},
						captureCase{"Hall of Fame absent at start", goal, gsKantoCaptureObs(id, beforeJohto, stage.previous), after},
						captureCase{"Hall event vanished", goal, before, gsKantoCaptureObs(id, afterJohto, stage.previous, stage.progress)},
					)
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						if selected, ok := gsKantoCaptureEligible(tc.goal, tc.before, tc.after); ok {
							t.Fatalf("invalid capture accepted: %+v", selected)
						}
					})
				}
			})
		}
	}
}

func TestGSKantoQualificationBoundariesAreOwnedRetailMaps(t *testing.T) {
	want := map[string]string{
		"post-ticket":  "ELMS_LAB",
		"post-landing": "VERMILION_PORT",
		"post-thunder": "VERMILION_GYM",
		"post-marsh":   "SAFFRON_GYM",
		"post-rainbow": "CELADON_GYM",
		"post-soul":         "FUCHSIA_GYM",
		"post-manager":      "POWER_PLANT",
		"post-rocket":       "CERULEAN_GYM",
		"post-machine-part": "CERULEAN_GYM",
		"post-power":        "POWER_PLANT",
		"post-copycat":      "COPYCATS_HOUSE_2F",
		"post-lost-doll":    "POKEMON_FAN_CLUB",
		"post-rail-pass":    "COPYCATS_HOUSE_2F",
		"post-misty-date":   "ROUTE_25",
		"post-cascade":      "CERULEAN_GYM",
	}
	if len(gsKantoCheckpointBoundaries) != len(want) {
		t.Fatalf("Kanto checkpoint count=%d, want %d", len(gsKantoCheckpointBoundaries), len(want))
	}
	seen := map[string]bool{}
	for _, stage := range gsKantoCheckpointBoundaries {
		if want[stage.directory] != stage.mapName || seen[stage.directory] {
			t.Fatalf("wrong or duplicated checkpoint boundary: %+v", stage)
		}
		seen[stage.directory] = true
		id, err := gsOpeningMapID(stage.mapName)
		if err != nil || (!gsOpeningScriptMap(id) && !gsSecondBadgeOwnedMap(id)) {
			t.Fatalf("unowned checkpoint map %s: id=%#04x err=%v", stage.mapName, id, err)
		}
	}
}
