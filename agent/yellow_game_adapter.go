package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
	yellowcontroller "github.com/maestroi/pokepilot/yellow/controller"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	yellowrom "github.com/maestroi/pokepilot/yellow/rom"
)

var errYellowControllerUnavailable = errors.New("Pokémon Yellow objective controller is not implemented yet")

type yellowObjectiveAdapter struct {
	m       *emu.Emu
	romData []byte
}

func init() {
	registerObjectiveAdapter(yellowprofile.GameID, func(m *emu.Emu, romData []byte) ObjectiveGameAdapter {
		return &yellowObjectiveAdapter{m: m, romData: romData}
	})
	registerObjectiveCatalogProvider(yellowprofile.GameID, &yellowObjectiveAdapter{})
}

func (a *yellowObjectiveAdapter) Observe() (Observation, error) {
	return ObserveChecked(a.m, a.romData)
}

func (a *yellowObjectiveAdapter) Validate(o Objective, _ Observation) error {
	if err := o.Validate(); err != nil {
		return err
	}
	if o.Kind == KindStarter && o.Species != "" && o.Species != "pikachu" {
		return fmt.Errorf("agent: %s: Yellow starter must be pikachu, got %q", o, o.Species)
	}
	if o.Kind == KindProgress && !yellowProgressionKnown(o.Progress) {
		return fmt.Errorf("agent: %s: unknown Yellow progression goal %q", o, o.Progress)
	}
	return nil
}

func (a *yellowObjectiveAdapter) NormalizeBoundary() error {
	obs, err := a.Observe()
	if err != nil {
		return err
	}
	if obs.Controllable && !obs.InBattle {
		return nil
	}
	return fmt.Errorf("%w: Yellow player is not at a stable controllable boundary", ErrObjectiveBoundaryDirty)
}

func (a *yellowObjectiveAdapter) ExecuteOwned(o Objective) (ObjectiveResult, error) {
	result := ObjectiveResult{Objective: o}
	switch o.Kind {
	case KindProgress:
		var err error
		switch o.Progress {
		case yellowprofile.ProgressYellowLabRivalResolved:
			err = yellowcontroller.GetPikachuStarter(a.m, a.romData)
		case gen1.ProgressPokedexAcquired:
			err = yellowcontroller.AcquirePokedex(a.m, a.romData)
		case gen1.ProgressBoulderBadge:
			err = yellowcontroller.DefeatBrock(a.m, a.romData)
		case gen1.ProgressMtMoonFossilAcquired:
			err = yellowcontroller.AcquireMtMoonFossil(a.m, a.romData)
		case yellowprofile.ProgressYellowMtMoonExitResolved:
			err = yellowcontroller.ResolveMtMoonExit(a.m, a.romData)
		case gen1.ProgressSSTicketAcquired:
			err = yellowcontroller.AcquireSSTicket(a.m, a.romData)
		case gen1.ProgressHM01Acquired:
			err = yellowcontroller.AcquireHM01(a.m, a.romData)
		default:
			err = fmt.Errorf("%w: Yellow progression %q", errYellowControllerUnavailable, o.Progress)
		}
		if err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil
	case KindTrainer:
		if err := yellowcontroller.ChallengeTrainer(a.m, a.romData, o.X, o.Y); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		result.Battle = &BattleEvidence{Result: "won", Won: true}
		return result, nil
	case KindTalk:
		presses, err := yellowcontroller.TalkAt(a.m, a.romData, o.X, o.Y)
		if err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		result.InteractionPresses = presses
		return result, nil
	case KindHeal:
		if o.Place != "" {
			obs, err := a.Observe()
			if err != nil {
				return result, fmt.Errorf("agent: %s: resolve Yellow Center: %w", o, err)
			}
			destination, ok := obs.Catalog.destination(o.Place)
			if !ok || !destination.Center {
				return result, fmt.Errorf("agent: %s: Yellow Center %q is not in the active catalog", o, o.Place)
			}
			mapID, ok := yellowNativeMapForLocation(destination.Location)
			if !ok {
				return result, fmt.Errorf("agent: %s: Yellow Center location %q has no native map", o, destination.Location)
			}
			if err := yellowcontroller.GoTo(a.m, a.romData, mapID, destination.X, destination.Y); err != nil {
				return result, fmt.Errorf("agent: %s: travel to Center: %w", o, err)
			}
		}
		if err := yellowcontroller.Heal(a.m, a.romData); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil
	case KindCatch:
		rawSpecies, ok := yellowSpeciesID(a.romData, o.Species)
		if !ok {
			return result, fmt.Errorf("agent: %s: unknown Yellow species %q", o, o.Species)
		}
		switch o.Intent {
		case dexGiftIntent:
			if err := yellowcontroller.ReceiveGift(a.m, a.romData, rawSpecies); err != nil {
				return result, fmt.Errorf("agent: %s: %w", o, err)
			}
			return result, nil
		case dexStaticIntent:
			caught, err := yellowcontroller.CaptureStatic(a.m, a.romData, rawSpecies)
			if err != nil {
				return result, fmt.Errorf("agent: %s: %w", o, err)
			}
			if !caught.Caught {
				return result, fmt.Errorf("agent: %s: Yellow static capture ended without ownership", o)
			}
			return result, nil
		case dexWaterIntent:
			if o.Place != "" {
				obs, err := a.Observe()
				if err != nil {
					return result, fmt.Errorf("agent: %s: resolve Yellow Surf habitat: %w", o, err)
				}
				destination, ok := obs.Catalog.destination(o.Place)
				if !ok {
					return result, fmt.Errorf("agent: %s: Yellow Surf habitat %q is not in the active catalog", o, o.Place)
				}
				mapID, ok := yellowNativeMapForLocation(destination.Location)
				if !ok {
					return result, fmt.Errorf("agent: %s: Yellow Surf habitat location %q has no native map", o, destination.Location)
				}
				if err := yellowcontroller.GoTo(a.m, a.romData, mapID, destination.X, destination.Y); err != nil {
					return result, fmt.Errorf("agent: %s: travel to Yellow Surf habitat: %w", o, err)
				}
			}
			caught, err := yellowcontroller.CaptureWildWater(a.m, a.romData, rawSpecies)
			if err != nil {
				return result, fmt.Errorf("agent: %s: %w", o, err)
			}
			if !caught.Caught {
				return result, fmt.Errorf("agent: %s: Yellow Surf capture ended without ownership", o)
			}
			return result, nil
		case dexFishingIntent:
			if o.Place != "" {
				obs, err := a.Observe()
				if err != nil {
					return result, fmt.Errorf("agent: %s: resolve Yellow fishing habitat: %w", o, err)
				}
				destination, ok := obs.Catalog.destination(o.Place)
				if !ok {
					return result, fmt.Errorf("agent: %s: Yellow fishing habitat %q is not in the active catalog", o, o.Place)
				}
				mapID, ok := yellowNativeMapForLocation(destination.Location)
				if !ok {
					return result, fmt.Errorf("agent: %s: Yellow fishing habitat location %q has no native map", o, destination.Location)
				}
				if err := yellowcontroller.GoTo(a.m, a.romData, mapID, destination.X, destination.Y); err != nil {
					return result, fmt.Errorf("agent: %s: travel to Yellow fishing habitat: %w", o, err)
				}
			}
			rawRod, ok := yellowFishingRodID(o.Item)
			if !ok {
				return result, fmt.Errorf("agent: %s: unknown Yellow fishing rod %q", o, o.Item)
			}
			caught, err := yellowcontroller.Fish(a.m, a.romData, rawRod, rawSpecies)
			if err != nil {
				return result, fmt.Errorf("agent: %s: %w", o, err)
			}
			if !caught.Caught {
				return result, fmt.Errorf("agent: %s: Yellow fishing ended without ownership", o)
			}
			return result, nil
		case dexSafariIntent:
			obs, err := a.Observe()
			if err != nil {
				return result, fmt.Errorf("agent: %s: resolve Yellow Safari habitat: %w", o, err)
			}
			destination, ok := obs.Catalog.destination(o.Place)
			if !ok {
				return result, fmt.Errorf("agent: %s: Yellow Safari habitat %q is not in the active catalog", o, o.Place)
			}
			mapID, ok := yellowNativeMapForLocation(destination.Location)
			if !ok {
				return result, fmt.Errorf("agent: %s: Yellow Safari habitat location %q has no native map", o, destination.Location)
			}
			caught, err := yellowcontroller.CaptureSafari(a.m, a.romData, mapID, rawSpecies)
			if err != nil {
				return result, fmt.Errorf("agent: %s: %w", o, err)
			}
			if !caught.Caught {
				return result, fmt.Errorf("agent: %s: Yellow Safari capture ended without ownership", o)
			}
			return result, nil
		default:
			if o.Place != "" {
				obs, err := a.Observe()
				if err != nil {
					return result, fmt.Errorf("agent: %s: resolve Yellow catch habitat: %w", o, err)
				}
				destination, ok := obs.Catalog.destination(o.Place)
				if !ok {
					return result, fmt.Errorf("agent: %s: Yellow catch habitat %q is not in the active catalog", o, o.Place)
				}
				mapID, ok := yellowNativeMapForLocation(destination.Location)
				if !ok {
					return result, fmt.Errorf("agent: %s: Yellow catch habitat location %q has no native map", o, destination.Location)
				}
				if err := yellowcontroller.GoTo(a.m, a.romData, mapID, destination.X, destination.Y); err != nil {
					return result, fmt.Errorf("agent: %s: travel to Yellow catch habitat: %w", o, err)
				}
			}
			caught, err := yellowcontroller.CaptureWildGrass(a.m, a.romData, rawSpecies)
			if err != nil {
				return result, fmt.Errorf("agent: %s: %w", o, err)
			}
			if !caught.Caught {
				return result, fmt.Errorf("agent: %s: Yellow grass capture ended without ownership", o)
			}
			return result, nil
		}
	case KindUseItem:
		obs, err := a.Observe()
		if err != nil {
			return result, fmt.Errorf("agent: %s: observe Yellow bag: %w", o, err)
		}
		rawItem, ok := yellowBagItemID(a.romData, obs, o.Item)
		if !ok {
			return result, fmt.Errorf("agent: %s: Yellow bag does not contain semantic item %q", o, o.Item)
		}
		switch rawItem {
		case 0x06: // BICYCLE
			if err := yellowcontroller.UseBicycle(a.m, a.romData); err != nil {
				return result, fmt.Errorf("agent: %s: %w", o, err)
			}
			result.ItemEffectVerified = true
			return result, nil
		case 0x49: // POKE_FLUTE
			if err := yellowcontroller.UsePokeFluteAtSnorlax(a.m, a.romData); err != nil {
				return result, fmt.Errorf("agent: %s: %w", o, err)
			}
			result.ItemEffectVerified = true
			return result, nil
		}
		if _, err := yellowrom.LookupTMHM(a.romData, rawItem); err == nil {
			if err := yellowcontroller.TeachTMHM(a.m, a.romData, rawItem, o.Slot); err != nil {
				return result, fmt.Errorf("agent: %s: %w", o, err)
			}
			result.ItemEffectVerified = true
			return result, nil
		}
		if err := yellowcontroller.UseFieldItem(a.m, a.romData, rawItem, o.Slot); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		result.ItemEffectVerified = true
		return result, nil
	case KindBuy:
		obs, err := a.Observe()
		if err != nil {
			return result, fmt.Errorf("agent: %s: observe Yellow mart: %w", o, err)
		}
		rawItem, ok := yellowMartItemID(a.romData, obs.Map, o.Item)
		if !ok {
			return result, fmt.Errorf("agent: %s: Yellow mart does not stock semantic item %q", o, o.Item)
		}
		if err := yellowcontroller.Buy(a.m, a.romData, rawItem, o.Qty); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil
	case KindStarter:
		if err := yellowcontroller.GetPikachuStarter(a.m, a.romData); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil
	case KindGoTo:
		obs, err := a.Observe()
		if err != nil {
			return result, fmt.Errorf("agent: %s: resolve Yellow destination: %w", o, err)
		}
		destination, ok := obs.Catalog.destination(o.Place)
		if !ok {
			return result, fmt.Errorf("agent: %s: Yellow destination %q is not in the active catalog", o, o.Place)
		}
		mapID, ok := yellowNativeMapForLocation(destination.Location)
		if !ok {
			return result, fmt.Errorf("agent: %s: Yellow destination location %q has no native map", o, destination.Location)
		}
		if err := yellowcontroller.GoTo(a.m, a.romData, mapID, destination.X, destination.Y); err != nil {
			return result, fmt.Errorf("agent: %s: %w", o, err)
		}
		return result, nil
	default:
		return result, fmt.Errorf("agent: %s: %w", o, errYellowControllerUnavailable)
	}
}

func yellowFishingRodID(id ItemID) (uint8, bool) {
	switch strings.ToLower(strings.TrimSpace(string(id))) {
	case "old rod":
		return yellowrom.OldRodItem, true
	case "good rod":
		return yellowrom.GoodRodItem, true
	case "super rod":
		return yellowrom.SuperRodItem, true
	default:
		return 0, false
	}
}

func yellowSpeciesID(romData []byte, id SpeciesID) (uint8, bool) {
	for dex := 1; dex <= 151; dex++ {
		raw, err := yellowrom.DexNumberInternalSpecies(romData, uint8(dex))
		if err != nil {
			continue
		}
		name, ok := gen1.Species(raw)
		if ok && SpeciesID(name) == id {
			return raw, true
		}
	}
	return 0, false
}

func yellowBagItemID(romData []byte, obs Observation, id ItemID) (uint8, bool) {
	for _, item := range obs.Bag {
		if ItemID(gameruntime.CanonicalID(item.Name)) != id || item.Quantity <= 0 {
			continue
		}
		for raw := 1; raw <= 0xff; raw++ {
			name, err := yellowrom.ItemName(romData, uint8(raw))
			if err != nil {
				continue
			}
			if ItemID(gameruntime.CanonicalID(name)) == id {
				return uint8(raw), true
			}
		}
	}
	return 0, false
}

func yellowMartItemID(romData []byte, mapID uint8, id ItemID) (uint8, bool) {
	items, err := yellowrom.MartItems(romData, mapID)
	if err != nil {
		return 0, false
	}
	for _, raw := range items {
		name, err := yellowrom.ItemName(romData, raw)
		if err != nil {
			continue
		}
		if ItemID(gameruntime.CanonicalID(name)) == id {
			return raw, true
		}
	}
	return 0, false
}

func (a *yellowObjectiveAdapter) WithinObjectiveBudget(o Objective, fn func() error) error {
	deadline := a.m.FrameCount() + objectiveFrameBudget
	err := a.m.WithFrameDeadline(deadline, fn)
	if errors.Is(err, emu.ErrFrameDeadline) {
		return fmt.Errorf("agent: %s: objective frame watchdog: %w", o, err)
	}
	return err
}

func (a *yellowObjectiveAdapter) SettlePostcondition(Objective) error { return nil }

func (a *yellowObjectiveAdapter) VerifyPostcondition(o Objective, initial, final Observation, result ObjectiveResult) error {
	_, err := verifyObjectivePostcondition(o, initial, final, result)
	return err
}

func (a *yellowObjectiveAdapter) NormalizeFailure(phase gameruntime.FailurePhase, err error, _ Observation) gameruntime.Failure {
	failure := gameruntime.Failure{
		Phase: phase, Class: gameruntime.FailureClassUnknown, Cause: "yellow_objective_failure",
	}
	if errors.Is(err, errYellowControllerUnavailable) {
		failure.Class = gameruntime.FailureClassBlocked
		failure.Cause = "yellow_controller_unavailable"
		failure.Recoverable = false
	}
	return failure
}

func (a *yellowObjectiveAdapter) CaptureFailure(Objective, error) error {
	// Red's RAM forensics decoder must never run against Yellow. Phase 5 can
	// attach Yellow-native controller evidence once those controllers exist.
	return nil
}

func (a *yellowObjectiveAdapter) ObjectiveCatalog(obs Observation) ObjectiveCatalog {
	catalog := ObjectiveCatalog{
		CurrentCenter: strings.Contains(strings.ToUpper(obs.MapName), "POKECENTER"),
	}
	if obs.PartyCount == 0 || !obs.Story.Has(yellowprofile.ProgressYellowLabRivalResolved) {
		catalog.Starters = []CatalogStarter{{Species: "pikachu"}}
	}
	return catalog
}
