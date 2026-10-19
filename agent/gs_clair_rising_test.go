package agent

import (
	"errors"
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	gsrom "github.com/maestroi/pokepilot/gs/rom"
	"github.com/maestroi/pokepilot/skill"
)

func TestGSBadgeEightCoordinatesAndOwnedMaps(t *testing.T) {
	if gsIcePathWaterfallX != 31 || gsIcePathWaterfallY != 7 ||
		gsIcePathWaterfallStandX != 30 || gsIcePathWaterfallStandY != 7 ||
		gsClairX != 5 || gsClairY != 3 ||
		gsClairStandX != 5 || gsClairStandY != 4 ||
		gsDragonFangX != 35 || gsDragonFangY != 16 ||
		gsDragonFangStandX != 35 || gsDragonFangStandY != 17 {
		t.Fatal("Clair or Dragon Fang scene coordinates drifted from pokegold maps")
	}
	for _, name := range []string{
		"ROUTE_44", "ICE_PATH_1F", "ICE_PATH_B1F",
		"ICE_PATH_B2F_MAHOGANY_SIDE", "ICE_PATH_B3F",
		"ICE_PATH_B2F_BLACKTHORN_SIDE", "BLACKTHORN_CITY",
		"BLACKTHORN_POKECENTER_1F", "BLACKTHORN_GYM_1F",
		"BLACKTHORN_GYM_2F", "DRAGONS_DEN_1F", "DRAGONS_DEN_B1F",
	} {
		id, err := gsOpeningMapID(name)
		if err != nil || !gsSecondBadgeOwnedMap(id) {
			t.Fatalf("late Johto map %q not owned: %v", name, err)
		}
	}
	for _, milestone := range []ProgressID{
		gsprofile.ProgressIcePathWaterfallAcquired,
		gsprofile.ProgressClairDefeated,
		gsprofile.ProgressRisingBadgeEarned,
	} {
		if !gsSupportedProgress(milestone) {
			t.Fatalf("eighth badge milestone %s not executable", milestone)
		}
	}
}

// run-a018a6cmhnkr / triage:223fbce86180e4f4: a Typhlosion+Togepi+Miltank
// party beat Clair holding HM06 and the Glacier Badge, but nobody could learn
// Whirlpool, so Dragon's Den failed terminally inside its executor. Rising
// Badge must instead report the missing capability so recovery runs the
// Gen-II carrier repair.
func TestGSRisingBadgeRequiresUsableWhirlpoolAndRecoversByRepair(t *testing.T) {
	adapter := newGSObjectiveAdapter(nil, nil, gsprofile.GoldGameID)
	rising := Objective{Kind: KindProgress, Progress: gsprofile.ProgressRisingBadgeEarned}
	noCarrier := Observation{
		Controllable: true,
		FieldCapabilities: []FieldCapability{
			{Name: "surf", Badge: "Fog", BadgeOwned: true, HMOwned: true, Learned: true, PartySlot: 2, Usable: true, Preparable: true},
			{Name: "whirlpool", Badge: "Glacier", BadgeOwned: true, HMOwned: true, PartySlot: -1},
		},
	}

	err := adapter.Validate(rising, noCarrier)
	var missing *game.PrerequisiteMissingError
	if !errors.As(err, &missing) || len(missing.Missing) != 1 || missing.Missing[0].FieldCapability != "whirlpool" {
		t.Fatalf("Validate without a Whirlpool carrier = %v, want missing field capability whirlpool", err)
	}
	failure := adapter.NormalizeFailure(game.FailurePhaseValidation, err, noCarrier)
	if failure.Class != game.FailureClassBlocked || !failure.Recoverable {
		t.Fatalf("normalized failure = %+v, want recoverable blocked", failure)
	}
	policy := newRunFailurePolicy(3)
	policy.pendingPrerequisites = failure.Prerequisites
	repair, _, ok := policy.adapterPrerequisiteRecovery(adapter, noCarrier, nil)
	if !ok || repair.Kind != KindRepairFieldCapability || repair.FieldCapability != "whirlpool" {
		t.Fatalf("recovery = %v ok=%v, want the Whirlpool repair objective", repair, ok)
	}
	if !adapter.SupportsObjective(repair) {
		t.Fatal("Gen2 adapter must execute the Whirlpool repair it asks recovery for")
	}

	withCarrier := noCarrier
	withCarrier.FieldCapabilities = append([]FieldCapability(nil), noCarrier.FieldCapabilities...)
	withCarrier.FieldCapabilities[1] = FieldCapability{Name: "whirlpool", BadgeOwned: true, HMOwned: true, Learned: true, PartySlot: 3, Usable: true, Preparable: true}
	if err := adapter.Validate(rising, withCarrier); err != nil {
		t.Fatalf("Validate with usable Surf and Whirlpool = %v", err)
	}
}

// The ROM must keep holding a Whirlpool carrier in the Route 44 site's water
// table and none in Route 38's grass, or the repair would silently pick the
// wrong site.
func TestGSWhirlpoolCarrierSiteIsRoute44Water(t *testing.T) {
	romPath := os.Getenv("POKEMON_GOLD_ROM")
	if romPath == "" {
		t.Skip("set POKEMON_GOLD_ROM to check the ROM-derived carrier site")
	}
	romData, err := os.ReadFile(romPath)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		t.Fatal(err)
	}
	encounters, err := gsrom.WildEncounters(romData)
	if err != nil {
		t.Fatal(err)
	}
	carriers := func(site gsFieldCarrierSite) int {
		area, err := gsOpeningMapID(site.area)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, e := range encounters {
			if e.MapID != area || e.Habitat != site.habitat {
				continue
			}
			if ok, err := profile.SpeciesCanLearnFieldMove(romData, e.Species, game.FieldMoveWhirlpool); err != nil {
				t.Fatal(err)
			} else if ok {
				n++
			}
		}
		return n
	}
	var route38, route44 *gsFieldCarrierSite
	for i := range gsFieldCarrierSites {
		switch gsFieldCarrierSites[i].area {
		case "ROUTE_38":
			route38 = &gsFieldCarrierSites[i]
		case "ROUTE_44":
			route44 = &gsFieldCarrierSites[i]
		}
	}
	if route38 == nil || route44 == nil || route44.habitat != gsrom.HabitatWater {
		t.Fatalf("carrier sites = %+v, want Route 38 grass and Route 44 water", gsFieldCarrierSites)
	}
	if n := carriers(*route38); n != 0 {
		t.Fatalf("Route 38 grass has %d Whirlpool carrier slots; site order no longer selects Route 44", n)
	}
	if n := carriers(*route44); n == 0 {
		t.Fatal("Route 44 water table holds no Whirlpool carrier")
	}
}

// TestGSRisingWhirlpoolRepairReplay replays run-a018a6cmhnkr's Rising Badge
// checkpoint (issue #2803) through the planner's chain: Rising Badge blocks on
// the missing Whirlpool, the repair surfs Route 44 for a carrier and teaches
// HM06, and Rising Badge then completes. The .state capture is never
// committed; set GS_RISING_WHIRLPOOL_CHECKPOINT to its materialized path.
func TestGSRisingWhirlpoolRepairReplay(t *testing.T) {
	rom := os.Getenv("POKEMON_GOLD_ROM")
	checkpoint := os.Getenv("GS_RISING_WHIRLPOOL_CHECKPOINT")
	if rom == "" || checkpoint == "" {
		t.Skip("set POKEMON_GOLD_ROM and GS_RISING_WHIRLPOOL_CHECKPOINT to replay the captured failure")
	}
	stateBytes, err := os.ReadFile(checkpoint)
	if err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}
	m, err := emu.Open(rom)
	if err != nil {
		t.Fatalf("open ROM: %v", err)
	}
	defer m.Close()
	if err := m.LoadState(stateBytes); err != nil {
		t.Fatalf("load state: %v", err)
	}
	rising := Objective{Kind: KindProgress, Progress: gsprofile.ProgressRisingBadgeEarned, Starter: skill.StarterCharmander}

	blocked, err := ExecuteWithRoutePriority(m, m.ROM(), rising, RoutePriorityConservative)
	if err == nil || blocked.Outcome != OutcomeBlocked || blocked.Failure == nil ||
		len(blocked.Failure.Prerequisites) != 1 || blocked.Failure.Prerequisites[0].FieldCapability != "whirlpool" {
		t.Fatalf("first Rising Badge = %s cause=%s failure=%+v err=%v, want blocked on whirlpool",
			blocked.Outcome, blocked.Cause, blocked.Failure, err)
	}

	repair := Objective{Kind: KindRepairFieldCapability, FieldCapability: "whirlpool"}
	repaired, err := ExecuteWithRoutePriority(m, m.ROM(), repair, RoutePriorityConservative)
	if err != nil || repaired.Outcome != OutcomeCompleted {
		t.Fatalf("Whirlpool repair = %s cause=%s summary=%s err=%v, want completed",
			repaired.Outcome, repaired.Cause, repaired.Summary, err)
	}

	earned, err := ExecuteWithRoutePriority(m, m.ROM(), rising, RoutePriorityConservative)
	if err != nil || earned.Outcome != OutcomeCompleted {
		t.Fatalf("Rising Badge after repair = %s cause=%s summary=%s err=%v, want completed",
			earned.Outcome, earned.Cause, earned.Summary, err)
	}
}
