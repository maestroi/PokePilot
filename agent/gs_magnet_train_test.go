package agent

import (
	"errors"
	"testing"

	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
)

func TestGSMagnetTrainStationSpecsAreNativeAndDirectional(t *testing.T) {
	for _, tc := range []struct {
		destination game.RegionID
		from, to    string
		source      game.RegionID
	}{
		{game.RegionKanto, "GOLDENROD_MAGNET_TRAIN_STATION", "SAFFRON_MAGNET_TRAIN_STATION", game.RegionJohto},
		{game.RegionJohto, "SAFFRON_MAGNET_TRAIN_STATION", "GOLDENROD_MAGNET_TRAIN_STATION", game.RegionKanto},
	} {
		spec, ok := gsTrainStationFor(tc.destination)
		if !ok || spec.from != tc.source || spec.to != tc.destination ||
			spec.depart != tc.from || spec.arrive != tc.to {
			t.Fatalf("Magnet Train %s spec = %+v ok=%v", tc.destination, spec, ok)
		}
		if spec.standX != 9 || spec.standY != 10 || spec.officerX != 9 || spec.officerY != 9 {
			t.Fatalf("Magnet Train officer approach does not match decomp: %+v", spec)
		}
		for _, name := range []string{spec.depart, spec.arrive} {
			id, err := gsOpeningMapID(name)
			if err != nil || !gsSecondBadgeOwnedMap(id) {
				t.Fatalf("train station %s unowned/unmapped: %#04x err=%v", name, id, err)
			}
		}
	}
	if _, ok := gsTrainStationFor(""); ok {
		t.Fatal("unknown transport destination must not default to Kanto")
	}
	if err := executeGSMagnetTrainCrossing(nil, nil, game.RegionKanto); err == nil {
		t.Fatal("nil emulator must not manufacture a Kanto arrival")
	}
	if err := gsKantoRestoreRegion(nil, nil, nil); err == nil {
		t.Fatal("nil region recovery must fail closed")
	}
}

func TestGSMagnetTrainPreflightUsesTypedDirectionalPrerequisites(t *testing.T) {
	train := []game.RegionTransportOption{
		{Service: "magnet_train", FromRegion: game.RegionJohto, ToRegion: game.RegionKanto,
			Available: false, Missing: []game.CapabilityID{"gs_kanto_power_restored", "gs_magnet_train_pass_in_bag"}},
		{Service: "magnet_train", FromRegion: game.RegionKanto, ToRegion: game.RegionJohto,
			Available: true},
		{Service: "ss_aqua", FromRegion: game.RegionJohto, ToRegion: game.RegionKanto,
			Available: true},
	}
	var blocked *game.PrerequisiteMissingError
	err := gsTrainPreflight(train, game.RegionJohto, game.RegionKanto)
	if !errors.As(err, &blocked) || len(blocked.Missing) != 2 {
		t.Fatalf("blocked train must report typed power/pass: %v", err)
	}
	if err := gsTrainPreflight(train, game.RegionKanto, game.RegionJohto); err != nil {
		t.Fatalf("available reverse train was blocked: %v", err)
	}
	if err := gsTrainPreflight(train[:1], game.RegionKanto, game.RegionJohto); err == nil {
		t.Fatal("absent reverse service cannot be inferred from forward service")
	}
	if err := gsTrainPreflight(train[2:], game.RegionJohto, game.RegionKanto); err == nil {
		t.Fatal("ferry availability must not unlock railway")
	}
}

func TestGSCopycatControllersAndMapsRemainBounded(t *testing.T) {
	adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
	for _, stage := range []game.ProgressID{
		gsprofile.ProgressKantoCopycatMet,
		gsprofile.ProgressKantoLostDollReceived,
		gsprofile.ProgressKantoRailPassReceived,
	} {
		o := Objective{Kind: KindProgress, Progress: stage}
		if !gsSupportedProgress(stage) || !adapter.SupportsObjective(o) {
			t.Fatalf("missing Copycat controller for %s", stage)
		}
		if got := gsObjectiveFrameBudget(o); got != gsFirstBadgeObjectiveFrameBudget {
			t.Fatalf("Copycat %s watchdog=%d", stage, got)
		}
	}
	for _, name := range []string{
		"COPYCATS_HOUSE_1F", "COPYCATS_HOUSE_2F", "POKEMON_FAN_CLUB",
	} {
		id, err := gsOpeningMapID(name)
		if err != nil || !gsSecondBadgeOwnedMap(id) {
			t.Fatalf("Copycat sidequest map %s not owned: %#04x %v", name, id, err)
		}
	}
	for _, fn := range []func() error{
		func() error { return executeGSKantoMeetCopycat(nil, nil) },
		func() error { return executeGSKantoReceiveLostDoll(nil, nil) },
		func() error { return executeGSKantoReceiveRailPass(nil, nil) },
	} {
		if err := fn(); err == nil {
			t.Fatal("Copycat controller accepted nil emulator")
		}
	}
}
