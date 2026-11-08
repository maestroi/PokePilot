package agent

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSKantoFirstVoyageMilestonesAreExecutable(t *testing.T) {
	for _, id := range []game.GameID{gsprofile.GoldGameID, gsprofile.SilverGameID} {
		adapter := newGSObjectiveAdapter(nil, nil, id)
		for _, progress := range []ProgressID{
			gsprofile.ProgressSSTicketReceived,
			gsprofile.ProgressKantoFirstLanding,
		} {
			objective := Objective{Kind: KindProgress, Progress: progress}
			if !gsSupportedProgress(progress) || !adapter.SupportsObjective(objective) {
				t.Fatalf("%s does not support %s", id, progress)
			}
			if got := gsObjectiveFrameBudget(objective); got != gsFirstBadgeObjectiveFrameBudget {
				t.Fatalf("%s frame budget = %d, want scripted-event budget", progress, got)
			}
		}
	}
	if err := executeGSSSTicket(nil, nil); err == nil {
		t.Fatal("nil emulator must not produce a ticket")
	}
	if err := executeGSFirstKantoLanding(nil, nil); err == nil {
		t.Fatal("nil emulator must not manufacture a Kanto landing")
	}
}

func TestGSAquaPreflightIsTypedAndDirectional(t *testing.T) {
	blocked := []game.RegionTransportOption{
		{Service: "ss_aqua", FromRegion: game.RegionKanto, ToRegion: game.RegionJohto, Available: true},
		{
			Service: "ss_aqua", FromRegion: game.RegionJohto, ToRegion: game.RegionKanto,
			Missing: []game.CapabilityID{"ticket", "olivine_departure_day"},
		},
	}
	err := gsAquaMissingPrerequisites(blocked)
	var missing *game.PrerequisiteMissingError
	if !errors.As(err, &missing) || len(missing.Missing) != 2 ||
		missing.Missing[0].Capability != "ticket" ||
		missing.Missing[1].Capability != "olivine_departure_day" {
		t.Fatalf("wrong directional prerequisite evidence: %v", err)
	}
	blocked[1].Available = true
	blocked[1].Missing = nil
	if err := gsAquaMissingPrerequisites(blocked); err != nil {
		t.Fatalf("boardable route rejected: %v", err)
	}
	if err := gsAquaMissingPrerequisites(blocked[:1]); !errors.Is(err, errGSControllerUnavailable) {
		t.Fatalf("missing eastbound route should fail closed, got %v", err)
	}
}

func TestGSAquaOwnsOnlyTheKnownScriptCorridor(t *testing.T) {
	for _, name := range append([]string{"OLIVINE_PORT", "OLIVINE_PORT_PASSAGE", "VERMILION_PORT"}, gsAquaShipMaps...) {
		id, err := gsOpeningMapID(name)
		if err != nil || !gsSecondBadgeOwnedMap(id) {
			t.Fatalf("ship script map %q not owned: id=%#04x err=%v", name, id, err)
		}
	}
}
