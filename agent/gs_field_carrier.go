package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	gsrom "github.com/maestroi/pokepilot/gs/rom"
	"github.com/maestroi/pokepilot/skill"
)

// errGSFieldCarrierUnavailable is the ordinary blocked outcome of a Gen-II
// field-capability repair: no compatible carrier could be acquired from the
// current party, money and qualified catch sites.
var errGSFieldCarrierUnavailable = errors.New("Gold/Silver field-move carrier is unavailable")

// gsProgressFieldCapabilities names the field capabilities a Gold/Silver story
// objective's executor needs to reach its destination. Cianwood is only
// reachable across Route 40/41 water.
func gsProgressFieldCapabilities(id ProgressID) []CapabilityID {
	switch id {
	case gsprofile.ProgressSecretPotionAcquired,
		gsprofile.ProgressJasmineReturnedToGym,
		gsprofile.ProgressStormBadgeEarned,
		gsprofile.ProgressHM02FlyAcquired:
		return []CapabilityID{CapabilityID(game.FieldMoveSurf)}
	default:
		return nil
	}
}

func gsMissingFieldCapabilities(o Objective, obs Observation) []CapabilityID {
	if o.Kind != KindProgress {
		return nil
	}
	var missing []CapabilityID
	for _, id := range gsProgressFieldCapabilities(o.Progress) {
		if !fieldCapabilityUsable(obs, id) {
			missing = append(missing, id)
		}
	}
	return missing
}

// gsFieldCarrierSite is a qualified place to acquire a new field-move carrier:
// a measured encounter tile plus the nearest mart that stocks an ordinary
// ball. The wanted species are not listed here; they come from the ROM's
// encounter and TM/HM compatibility tables for the requested move.
type gsFieldCarrierSite struct {
	after              ProgressID
	grass              string
	x, y               uint8
	mart               string
	standX, standY     uint8
	counterX, counterY uint8
	ball               uint8
}

var gsFieldCarrierSites = []gsFieldCarrierSite{
	// Route 38's grass strip (live collision COLL_LONG_GRASS at x=8..9,
	// y=6..13) is on the Olivine frontier, rolls encounters 10% of steps at
	// every time of day (Route 39 is only 2% outside the night), and holds
	// Tauros/Miltank, which can learn Surf. Olivine Mart (MART_OLIVINE) stocks
	// GREAT_BALL; its clerk is talked to across the counter at (2,3).
	{
		after: gsprofile.ProgressHM03SurfAcquired,
		grass: "ROUTE_38", x: 8, y: 10,
		mart: "OLIVINE_MART", standX: 3, standY: 3, counterX: 2, counterY: 3,
		ball: 0x04,
	},
}

const (
	// gsCarrierBallTarget is the needs-based ball stock for one carrier hunt:
	// a full-HP catch-rate-45 target takes a Great Ball ~9% of the time, so
	// twenty balls land one about 85% of the time.
	gsCarrierBallTarget   = 20
	gsCarrierHuntMaxSteps = 3000
)

func gsFieldMoveForCapability(id CapabilityID) (skill.FieldMove, bool) {
	for _, move := range skill.SemanticFieldMoves() {
		if strings.ToLower(move.String()) == string(id) {
			return move, true
		}
	}
	return 0, false
}

// executeGSRepairFieldCapability makes one field capability usable: teach a
// compatible party member, or first acquire a ROM-compatible wild carrier.
// The generic verifier proves the result from the observed capability.
func executeGSRepairFieldCapability(m *emu.Emu, romData []byte, capability CapabilityID) error {
	if m == nil {
		return fmt.Errorf("gen2 field repair: nil emulator")
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	move, ok := gsFieldMoveForCapability(capability)
	if !ok {
		return fmt.Errorf("%w: unknown field capability %q", errGSControllerUnavailable, capability)
	}
	id := game.FieldMoveID(capability)
	live, supported, err := profile.DecodeFieldMoveCapability(m, romData, id)
	if err != nil {
		return err
	}
	if !supported {
		return fmt.Errorf("%w: %s is not a Gold/Silver field move", errGSControllerUnavailable, id)
	}
	if !live.BadgeOwned || !live.MachineOwned {
		return fmt.Errorf("%w: %s needs its badge and machine first", errGSFieldCarrierUnavailable, id)
	}
	if live.Usable {
		return nil
	}
	if !live.Preparable {
		if err := gsAcquireFieldCarrier(m, romData, profile, id); err != nil {
			return fmt.Errorf("gen2 field repair %s: %w", id, err)
		}
	}
	if _, err := skill.EnsureFieldMove(m, move); err != nil {
		return fmt.Errorf("gen2 field repair %s: teach: %w", id, err)
	}
	// Teaching returns as the menus close; hand back a controllable boundary.
	return driveGSSecondBadgeInterruption(m, profile, "gen2:field-repair")
}

// gsCarrierSiteFor picks the first qualified site whose grass table holds a
// species that can learn id.
func gsCarrierSiteFor(m *emu.Emu, romData []byte, profile *gsprofile.Profile, id game.FieldMoveID) (gsFieldCarrierSite, []uint8, error) {
	encounters, err := gsrom.WildEncounters(romData)
	if err != nil {
		return gsFieldCarrierSite{}, nil, err
	}
	for _, site := range gsFieldCarrierSites {
		if !gsFirstBadgeProgressComplete(profile, m, site.after) {
			continue
		}
		grass, err := gsOpeningMapID(site.grass)
		if err != nil {
			return gsFieldCarrierSite{}, nil, err
		}
		var want []uint8
		for _, e := range encounters {
			if e.MapID != grass || e.Habitat != gsrom.HabitatGrass || speciesIn(e.Species, want) {
				continue
			}
			ok, err := profile.SpeciesCanLearnFieldMove(romData, e.Species, id)
			if err != nil {
				return gsFieldCarrierSite{}, nil, err
			}
			if ok {
				want = append(want, e.Species)
			}
		}
		if len(want) > 0 {
			return site, want, nil
		}
	}
	return gsFieldCarrierSite{}, nil, fmt.Errorf("%w: no qualified catch site holds a %s carrier", errGSFieldCarrierUnavailable, id)
}

func speciesIn(species uint8, list []uint8) bool {
	for _, s := range list {
		if s == species {
			return true
		}
	}
	return false
}

func gsCarrierBallCount(profile *gsprofile.Profile, m *emu.Emu) int {
	inv := profile.DecodeInventory(m)
	total := 0
	for _, ball := range profile.OrdinaryCaptureBallOrder() {
		for _, item := range inv.Items {
			if item.NativeItemID == ball {
				total += item.Quantity
			}
		}
	}
	return total
}

// gsStockCarrierBalls buys only the shortfall to gsCarrierBallTarget, capped
// by what the player can afford.
func gsStockCarrierBalls(m *emu.Emu, romData []byte, profile *gsprofile.Profile, site gsFieldCarrierSite) error {
	have := gsCarrierBallCount(profile, m)
	if have >= gsCarrierBallTarget {
		return nil
	}
	attr, err := gsrom.LookupItem(romData, site.ball)
	if err != nil || attr.Price == 0 {
		return fmt.Errorf("gen2 carrier balls: price of item %#02x: %v", site.ball, err)
	}
	qty := min(gsCarrierBallTarget-have, int(profile.DecodeInventory(m).Money)/int(attr.Price))
	if qty <= 0 {
		if have > 0 {
			return nil
		}
		return fmt.Errorf("%w: no balls and no money for one", errGSFieldCarrierUnavailable)
	}
	mart, err := gsOpeningMapID(site.mart)
	if err != nil {
		return err
	}
	if err := gsSecondBadgeFaceFrom(m, romData, profile,
		skill.ExactNativeDestination(mart, site.standX, site.standY),
		site.counterX, site.counterY, "gen2:carrier-mart"); err != nil {
		return fmt.Errorf("reach mart clerk: %w", err)
	}
	if err := skill.Buy(m, site.ball, qty); err != nil {
		return fmt.Errorf("buy %d of item %#02x: %w", qty, site.ball, err)
	}
	return nil
}

// gsAcquireFieldCarrier stocks balls, then hunts the site's grass until one
// wanted species is caught into the party. Unwanted and trainer battles are
// fought by the owned interruption driver.
func gsAcquireFieldCarrier(m *emu.Emu, romData []byte, profile *gsprofile.Profile, id game.FieldMoveID) error {
	// ponytail: a full party would send the catch to the SRAM box, which this
	// controller cannot withdraw yet; add box withdraw when that blocks a run.
	if count := len(profile.DecodeCapture(m).PartySpecies); count >= 6 {
		return fmt.Errorf("%w: party is full", errGSFieldCarrierUnavailable)
	}
	site, want, err := gsCarrierSiteFor(m, romData, profile, id)
	if err != nil {
		return err
	}
	if err := gsStockCarrierBalls(m, romData, profile, site); err != nil {
		return err
	}
	grass, err := gsOpeningMapID(site.grass)
	if err != nil {
		return err
	}
	tile := skill.ExactNativeDestination(grass, site.x, site.y)
	if err := gsEnsurePartyRecovered(m, romData, profile); err != nil {
		return fmt.Errorf("recover before carrier hunt: %w", err)
	}
	if err := gsSecondBadgeGoTo(m, romData, profile, tile); err != nil {
		return fmt.Errorf("reach carrier grass: %w", err)
	}

	policy := skill.StatAwareMove(romData)
	partyBefore := len(profile.DecodeCapture(m).PartySpecies)
	// A short Gen-II tap toward a new direction only turns the player, so each
	// direction is pressed twice: turn, then step. From the hunt tile this
	// stays inside the grass strip.
	dirs := []emu.Button{emu.Up, emu.Up, emu.Down, emu.Down}
	for steps := 0; ; steps++ {
		if steps >= gsCarrierHuntMaxSteps {
			return fmt.Errorf("%w: %d hunt steps on %s without a catch", errGSFieldCarrierUnavailable, steps, site.grass)
		}
		balls := gsCarrierBallCount(profile, m)
		if balls == 0 {
			return fmt.Errorf("%w: out of balls on %s", errGSFieldCarrierUnavailable, site.grass)
		}
		if world := profile.DecodeOverworld(m); world.NativeMapID != grass {
			if err := gsSecondBadgeGoTo(m, romData, profile, tile); err != nil {
				return fmt.Errorf("return to carrier grass: %w", err)
			}
		}
		if frac, err := gsLeadHPFraction(profile, m, romData); err == nil && frac < 0.4 {
			if err := gsEnsurePartyRecovered(m, romData, profile); err != nil {
				return fmt.Errorf("recover during carrier hunt: %w", err)
			}
			continue
		}

		m.Tap(dirs[steps%len(dirs)], 3, 7)
		if battle, ok := gsWaitWildBattle(m, profile); ok && battle.Kind == game.BattleWild && speciesIn(battle.EnemySpecies, want) {
			res, err := skill.CatchEngaged(m, romData, want, policy, balls)
			if err != nil {
				return fmt.Errorf("throw at species %#02x: %w", battle.EnemySpecies, err)
			}
			if res.Outcome == skill.OutcomeCaught && len(profile.DecodeCapture(m).PartySpecies) == partyBefore+1 {
				return nil
			}
			continue
		}
		if err := driveGSSecondBadgeInterruption(m, profile, "gen2:carrier-hunt"); err != nil {
			resume, afterBlackout := gsResumeGrindAfterInterruption(err)
			if !resume {
				return fmt.Errorf("carrier hunt step: %w", err)
			}
			if afterBlackout {
				if err := gsEnsurePartyRecovered(m, romData, profile); err != nil {
					return fmt.Errorf("recover after carrier hunt blackout: %w", err)
				}
			}
		}
	}
}

const (
	// A grass step's battle transition sets the battle mode ~170 frames after
	// the press; the species is loaded by the "Wild X appeared!" box, which
	// waits for A. Throwing (UseItem) advances that box itself.
	gsWildBattleStartFrames = 240
	gsWildBattleStateFrames = 120
)

// gsWaitWildBattle reports a battle that started after a grass step once its
// enemy species is decoded.
func gsWaitWildBattle(m *emu.Emu, profile *gsprofile.Profile) (game.BattleState, bool) {
	for i := 0; i < gsWildBattleStartFrames && !profile.DecodeOverworld(m).InBattle; i++ {
		m.StepFrame()
	}
	for i := 0; i < gsWildBattleStateFrames; i++ {
		if battle, ok := profile.DecodeBattleState(m); ok && battle.EnemySpecies != 0 {
			return battle, true
		}
		m.StepFrame()
	}
	return game.BattleState{}, false
}
