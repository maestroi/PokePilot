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
// reachable across Route 40/41 water; the Dragon Fang sits past Dragon's Den
// B1F water and whirlpools. Surf is listed first: a Whirlpool carrier hunt
// may itself need to surf.
func gsProgressFieldCapabilities(id ProgressID) []CapabilityID {
	switch id {
	case gsprofile.ProgressSecretPotionAcquired,
		gsprofile.ProgressJasmineReturnedToGym,
		gsprofile.ProgressStormBadgeEarned,
		gsprofile.ProgressHM02FlyAcquired,
		gsprofile.ProgressRedGyaradosDefeated,
		gsprofile.ProgressLanceHelpAccepted:
		return []CapabilityID{CapabilityID(game.FieldMoveSurf)}
	case gsprofile.ProgressRisingBadgeEarned:
		return []CapabilityID{CapabilityID(game.FieldMoveSurf), CapabilityID(game.FieldMoveWhirlpool)}
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
//
// habitat selects which encounter table the hunt tile rolls: a grass tile
// or, for gsrom.HabitatWater, a water tile reached and walked while surfing.
type gsFieldCarrierSite struct {
	after              ProgressID
	area               string
	habitat            string
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
		area:  "ROUTE_38", habitat: gsrom.HabitatGrass, x: 8, y: 10,
		mart: "OLIVINE_MART", standX: 3, standY: 3, counterX: 2, counterY: 3,
		ball: 0x04,
	},
	// Reachable grass carriers of Whirlpool are time-of-day bound (Route 32
	// Wooper) or a 1% slot (Mt. Mortar Marill), and the Whirl Islands are
	// themselves ringed by whirlpools. Route 44's
	// pond rolls 2% of surfed steps at every time of day and its whole water
	// table (Poliwag/Poliwhirl) can learn Whirlpool. (38,7) is COLL_WATER with
	// two more water tiles above and below it. Clair's defeat proves both
	// Route 44 and Blackthorn were crossed; Blackthorn Mart (MART_BLACKTHORN)
	// stocks GREAT_BALL with Olivine's counter layout (clerk at (1,3)).
	{
		after: gsprofile.ProgressClairDefeated,
		area:  "ROUTE_44", habitat: gsrom.HabitatWater, x: 38, y: 7,
		mart: "BLACKTHORN_MART", standX: 3, standY: 3, counterX: 2, counterY: 3,
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

// gsCarrierSiteFor picks the first qualified site whose hunted encounter table
// holds a species that can learn id. A water site qualifies only while the
// party can already surf.
func gsCarrierSiteFor(m *emu.Emu, romData []byte, profile *gsprofile.Profile, id game.FieldMoveID) (gsFieldCarrierSite, []uint8, error) {
	encounters, err := gsrom.WildEncounters(romData)
	if err != nil {
		return gsFieldCarrierSite{}, nil, err
	}
	surf, _, err := profile.DecodeFieldMoveCapability(m, romData, game.FieldMoveSurf)
	if err != nil {
		return gsFieldCarrierSite{}, nil, err
	}
	for _, site := range gsFieldCarrierSites {
		if !gsFirstBadgeProgressComplete(profile, m, site.after) {
			continue
		}
		if site.habitat == gsrom.HabitatWater && !surf.Usable {
			continue
		}
		area, err := gsOpeningMapID(site.area)
		if err != nil {
			return gsFieldCarrierSite{}, nil, err
		}
		var want []uint8
		for _, e := range encounters {
			if e.MapID != area || e.Habitat != site.habitat || speciesIn(e.Species, want) {
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

// gsAcquireFieldCarrier stocks balls, then hunts the site's tile until one
// wanted species is caught into the party. Unwanted and trainer battles are
// fought by the owned interruption driver.
func gsAcquireFieldCarrier(m *emu.Emu, romData []byte, profile *gsprofile.Profile, id game.FieldMoveID) error {
	// Bill's PC first: a boxed species that can learn the move is cheaper and
	// surer than a hunt, and a full party must make room for either.
	withdrew, err := gsPrepareCarrierRoster(m, romData, profile, id)
	if err != nil {
		return fmt.Errorf("prepare roster: %w", err)
	}
	if withdrew {
		return nil
	}
	site, want, err := gsCarrierSiteFor(m, romData, profile, id)
	if err != nil {
		return err
	}
	if err := gsStockCarrierBalls(m, romData, profile, site); err != nil {
		return err
	}
	area, err := gsOpeningMapID(site.area)
	if err != nil {
		return err
	}
	tile := skill.ExactNativeDestination(area, site.x, site.y)
	if err := gsEnsurePartyRecovered(m, romData, profile); err != nil {
		return fmt.Errorf("recover before carrier hunt: %w", err)
	}
	if err := gsSecondBadgeGoTo(m, romData, profile, tile); err != nil {
		return fmt.Errorf("reach carrier %s tile: %w", site.habitat, err)
	}

	policy := skill.StatAwareMove(romData)
	partyBefore := len(profile.DecodeCapture(m).PartySpecies)
	// A short Gen-II tap toward a new direction only turns the player, so each
	// direction is pressed twice: turn, then step. From the hunt tile this
	// stays inside the measured grass strip or water body.
	dirs := []emu.Button{emu.Up, emu.Up, emu.Down, emu.Down}
	for steps := 0; ; steps++ {
		if steps >= gsCarrierHuntMaxSteps {
			return fmt.Errorf("%w: %d hunt steps on %s without a catch", errGSFieldCarrierUnavailable, steps, site.area)
		}
		balls := gsCarrierBallCount(profile, m)
		if balls == 0 {
			// The initial stock was exhausted without a catch. Re-stock as long
			// as the player can afford it; the loop's return-to-site and
			// recovery checks below handle the mart trip's side effects. Only a
			// player with no balls AND no money is truly out of options.
			if err := gsStockCarrierBalls(m, romData, profile, site); err != nil {
				return fmt.Errorf("%w: out of balls on %s", errGSFieldCarrierUnavailable, site.area)
			}
			continue
		}
		if world := profile.DecodeOverworld(m); world.NativeMapID != area {
			if err := gsSecondBadgeGoTo(m, romData, profile, tile); err != nil {
				return fmt.Errorf("return to carrier %s tile: %w", site.habitat, err)
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
