package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	gsprofile "github.com/maestroi/pokepilot/gs/profile"
	"github.com/maestroi/pokepilot/skill"
)

const (
	gsTrainBoardBudget    uint64 = 30_000
	gsTrainMaxPresses           = 120
	gsTrainTalkAttempts         = 3
)

// gsTrainStation holds the exact pret/pokegold station/object identities.
// The static warp from the Goldenrod station to Saffron exists even when
// the officer refuses travel. Only his scripted YES/PASS/power interaction
// may cross regions; pathing onto that warp is never a substitute.
type gsTrainStation struct {
	from, to                           game.RegionID
	depart, arrive                     string
	standX, standY, officerX, officerY uint8
}

func gsTrainStationFor(destination game.RegionID) (gsTrainStation, bool) {
	switch destination {
	case game.RegionKanto:
		return gsTrainStation{
			from: game.RegionJohto, to: game.RegionKanto,
			depart: "GOLDENROD_MAGNET_TRAIN_STATION", arrive: "SAFFRON_MAGNET_TRAIN_STATION",
			standX: 9, standY: 10, officerX: 9, officerY: 9,
		}, true
	case game.RegionJohto:
		return gsTrainStation{
			from: game.RegionKanto, to: game.RegionJohto,
			depart: "SAFFRON_MAGNET_TRAIN_STATION", arrive: "GOLDENROD_MAGNET_TRAIN_STATION",
			standX: 9, standY: 10, officerX: 9, officerY: 9,
		}, true
	default:
		return gsTrainStation{}, false
	}
}

// gsTrainPreflight reads the same directional transport prerequisites exposed
// in the spectator observation (PASS in key-item pocket and Kanto power).
// An absent or unavailable option returns a typed prerequisite error rather
// than guessing a static station warp can bypass the ticket officer.
func gsTrainPreflight(options []game.RegionTransportOption, from, to game.RegionID) error {
	for _, option := range options {
		if option.Service != "magnet_train" || option.FromRegion != from || option.ToRegion != to {
			continue
		}
		if option.Available && len(option.Missing) == 0 {
			return nil
		}
		missing := make([]game.Prerequisite, 0, len(option.Missing))
		for _, cap := range option.Missing {
			missing = append(missing, game.CapabilityPrerequisite(cap))
		}
		return &game.PrerequisiteMissingError{Missing: missing}
	}
	return fmt.Errorf("%w: Magnet Train has no %s -> %s service", errGSControllerUnavailable, from, to)
}

// gsTrainBoardAndVerify owns one officer conversation and the game-scripted
// train arrival. The prompt must actually be seen and answered YES, and the
// real destination station plus controllable region must be observed. It
// never writes event bytes, opens a static cross-region warp, or assumes
// that entering a vehicle animation completed travel.
func gsTrainBoardAndVerify(m *emu.Emu, romData []byte, profile *gsprofile.Profile, leg gsTrainStation) error {
	if m == nil || profile == nil {
		return fmt.Errorf("%w: Magnet Train requires a live Gold/Silver emulator", errGSSecondBadgeUnexpectedState)
	}
	from, err := gsOpeningMapID(leg.depart)
	if err != nil {
		return err
	}
	to, err := gsOpeningMapID(leg.arrive)
	if err != nil {
		return err
	}
	const label = "kanto:magnet-train"
	// Reaching (9,10) is not a warp: the script-owned officer is (9,9).
	// This stages the player on the public floor, never on the train-door
	// warp, so the guard and PASS checks cannot be bypassed.
	if err := gsSecondBadgeFaceFrom(m, romData, profile,
		skill.ExactNativeDestination(from, leg.standX, leg.standY),
		leg.officerX, leg.officerY, label); err != nil {
		return fmt.Errorf("gen2 Magnet Train: reach %s officer: %w", leg.depart, err)
	}
	m.StepFrames(gsLakeScriptSettleFrames)
	m.Tap(emu.A, 3, 7)
	start := m.FrameCount()
	textPresses, retries := 0, 0
	accepted := false
	for m.FrameCount()-start < gsTrainBoardBudget {
		state := profile.DecodeOverworld(m)
		switch state.NativeMapID {
		case to:
			if !accepted {
				return fmt.Errorf("%w: Magnet Train destination reached without a verified YES choice", errGSSecondBadgeUnexpectedState)
			}
			if err := driveGSSecondBadgeInterruption(m, profile, "magnet-train:arrival"); err != nil {
				return fmt.Errorf("gen2 Magnet Train: finish arrival script: %w", err)
			}
			obs, err := profile.DecodeObservation(m, romData)
			if err != nil {
				return fmt.Errorf("gen2 Magnet Train: decode arrival: %w", err)
			}
			if obs.NativeMapID != to || obs.Region != leg.to || !profile.DecodeOverworld(m).Controllable {
				return fmt.Errorf("%w: Magnet Train arrived at %#04x in region %s, want %#04x in %s",
					errGSSecondBadgeUnexpectedState, obs.NativeMapID, obs.Region, to, leg.to)
			}
			return nil
		case from:
			// Still in the departure station: the owned script or menu is
			// allowed to finish here, but may not invent another map.
		default:
			return fmt.Errorf("%w: Magnet Train script left %s for unrelated map %#04x",
				errGSSecondBadgeUnexpectedState, leg.depart, state.NativeMapID)
		}
		if choice, visible := profile.DecodeTwoOption(m); visible {
			if accepted {
				return fmt.Errorf("%w: Magnet Train opened a second unowned choice", errGSSecondBadgeUnexpectedState)
			}
			// YES is menu option zero. Move from NO first if a previous
			// cursor position survived. Only a verified YES key accepts.
			if choice.Current != 0 {
				m.Tap(emu.Up, 3, 7)
			} else {
				m.Tap(emu.A, 3, 7)
				accepted = true
			}
			continue
		}
		if state.InBattle {
			return fmt.Errorf("%w: unexpected battle while boarding Magnet Train", errGSSecondBadgeUnexpectedState)
		}
		if state.InDialogue && state.MovementIdle {
			if textPresses >= gsTrainMaxPresses {
				return fmt.Errorf("%w: Magnet Train dialogue exceeded %d presses", errGSSecondBadgeStalled, gsTrainMaxPresses)
			}
			m.Tap(emu.A, 3, 7)
			textPresses++
			continue
		}
		if state.Controllable {
			if accepted {
				return fmt.Errorf("%w: Magnet Train accepted boarding but never left %s", errGSSecondBadgeUnexpectedState, leg.depart)
			}
			if retries >= gsTrainTalkAttempts {
				return fmt.Errorf("%w: Magnet Train officer never opened a YES/NO boarding choice", errGSSecondBadgeStalled)
			}
			// The first A might have fallen into a post-navigation input
			// blackout. Retry only from the proved officer staging tile.
			if state.X != leg.standX || state.Y != leg.standY {
				return fmt.Errorf("%w: Magnet Train lost officer staging tile before boarding", errGSSecondBadgeUnexpectedState)
			}
			m.StepFrames(50)
			m.Tap(emu.A, 3, 7)
			retries++
			continue
		}
		m.StepFrame()
	}
	return fmt.Errorf("%w: Magnet Train transfer to %s exceeded %d frames",
		errGSSecondBadgeStalled, leg.to, gsTrainBoardBudget)
}

// executeGSMagnetTrainCrossing is deliberately a named game-owned transport
// transaction, not an ordinary native warp. It can be used to return to
// Kanto after a Johto blackout once Copycat's PASS has been obtained, and
// from Kanto to Johto when an explicit transport is requested.
func executeGSMagnetTrainCrossing(m *emu.Emu, romData []byte, destination game.RegionID) error {
	if m == nil {
		return fmt.Errorf("%w: missing emulator for Magnet Train crossing", errGSSecondBadgeUnexpectedState)
	}
	leg, ok := gsTrainStationFor(destination)
	if !ok {
		return fmt.Errorf("%w: unknown Magnet Train destination %s", errGSControllerUnavailable, destination)
	}
	profile, err := gsOpeningProfile(romData)
	if err != nil {
		return err
	}
	if err := gsTrainPreflight(profile.DecodeRegionTransports(m), leg.from, leg.to); err != nil {
		return err
	}
	obs, err := profile.DecodeObservation(m, romData)
	if err != nil {
		return fmt.Errorf("gen2 Magnet Train: observe departure: %w", err)
	}
	if obs.Region != leg.from || !profile.DecodeOverworld(m).Controllable {
		return fmt.Errorf("%w: Magnet Train to %s requires a controllable %s departure, got %s",
			errGSSecondBadgeUnexpectedState, leg.to, leg.from, obs.Region)
	}
	return gsTrainBoardAndVerify(m, romData, profile, leg)
}

// gsKantoRestoreRegion is used only for Kanto-owned objectives after a
// checkpoint/blackout that unexpectedly resumed in Johto. Without an
// observed retail PASS it refuses a fictional cross-region walk.
func gsKantoRestoreRegion(m *emu.Emu, romData []byte, profile *gsprofile.Profile) error {
	if m == nil || profile == nil {
		return fmt.Errorf("%w: missing region-recovery state", errGSSecondBadgeUnexpectedState)
	}
	obs, err := profile.DecodeObservation(m, romData)
	if err != nil {
		return fmt.Errorf("gen2 Kanto: decode current region: %w", err)
	}
	switch obs.Region {
	case game.RegionKanto:
		return nil
	case game.RegionJohto:
		if !gsFirstBadgeProgressComplete(profile, m, gsprofile.ProgressKantoRailPassReceived) {
			return fmt.Errorf("%w: cannot return from Johto to Kanto without Copycat's observed rail PASS",
				errGSSecondBadgeUnexpectedState)
		}
		return executeGSMagnetTrainCrossing(m, romData, game.RegionKanto)
	default:
		return fmt.Errorf("%w: Kanto objective resumed from unknown region %s", errGSSecondBadgeUnexpectedState, obs.Region)
	}
}
