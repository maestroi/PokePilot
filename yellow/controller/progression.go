package controller

import (
	"fmt"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gen1"
	"github.com/maestroi/pokepilot/world"
	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
	"github.com/maestroi/pokepilot/yellow/sym"
)

const (
	yellowViridianMartMap    uint8 = 0x2a
	yellowPewterGymMap       uint8 = 0x36
	yellowMtMoonB2FMap       uint8 = 0x3d
	yellowBillsHouseMap      uint8 = 0x58
	yellowSSAnneCaptainsRoom uint8 = 0x65
	yellowStorySettleFrames        = 8
	yellowStoryFrameBudget         = 18000
)

// yellowStoryHas reads the same semantic projection the agent later uses for
// postcondition verification. Story controllers therefore never declare a
// milestone from a transient menu/map state.
func yellowStoryHas(m *emu.Emu, romData []byte, id game.ProgressID) (bool, error) {
	obs, err := yellowprofile.New().DecodeObservation(m, romData)
	if err != nil {
		return false, err
	}
	return obs.Story.Has(id), nil
}

func settleYellowStoryProgress(m *emu.Emu, romData []byte, id game.ProgressID, acceptYes bool) error {
	stable := 0
	for frame := 0; frame < yellowStoryFrameBudget; frame++ {
		obs, err := yellowprofile.New().DecodeObservation(m, romData)
		if err != nil {
			return fmt.Errorf("yellow story %s: observe: %w", id, err)
		}
		complete := obs.Story.Has(id)
		if complete && obs.Controllable && !obs.InBattle && m.Peek8(sym.FontLoaded) == 0 {
			stable++
			if stable >= yellowStorySettleFrames {
				return nil
			}
			m.StepFrame()
			continue
		}
		stable = 0

		if obs.InBattle {
			result, err := Battle(m, romData)
			if err != nil {
				return fmt.Errorf("yellow story %s: battle: %w", id, err)
			}
			if result.Outcome == BattleOutcomeLost {
				return fmt.Errorf("yellow story %s: blacked out before milestone committed", id)
			}
			continue
		}

		text := strings.ToUpper(screenText(m))
		if m.Peek8(sym.MaxMenuItem) == 1 && strings.Contains(text, "YES") && strings.Contains(text, "NO") {
			if !acceptYes {
				return fmt.Errorf("%w: yellow story %s screen=%q", ErrDialogueChoiceRequired, id,
					strings.Join(strings.Fields(screenText(m)), " "))
			}
			if err := selectYellowTwoOption(m, false); err != nil {
				return fmt.Errorf("yellow story %s: select YES: %w", id, err)
			}
			continue
		}

		if m.Peek8(sym.FontLoaded) != 0 {
			m.Tap(emu.A, 3, 7)
			continue
		}
		if obs.Controllable && !complete {
			// The caller may need to perform another explicit story action.
			// Returning here keeps each interaction bounded instead of blindly
			// pressing A or movement after the script has handed control back.
			return fmt.Errorf("yellow story %s: interaction settled before progress committed on map %#02x at (%d,%d)",
				id, m.Peek8(sym.CurMap), m.Peek8(sym.XCoord), m.Peek8(sym.YCoord))
		}
		m.StepFrame()
	}
	return fmt.Errorf("yellow story %s: exceeded %d frames", id, yellowStoryFrameBudget)
}

func yellowStoryInteract(m *emu.Emu, romData []byte, x, y uint8, id game.ProgressID, acceptYes bool) error {
	if err := interactAt(m, romData, int(x), int(y)); err != nil {
		return fmt.Errorf("yellow story %s: interact at (%d,%d): %w", id, x, y, err)
	}
	return settleYellowStoryProgress(m, romData, id, acceptYes)
}

func yellowStoryHiddenEvent(m *emu.Emu, romData []byte, mapID uint8, x, y uint8, face world.Step, id game.ProgressID) error {
	if err := GoTo(m, romData, mapID, x, y); err != nil {
		return fmt.Errorf("yellow story %s: reach hidden event (%d,%d): %w", id, x, y, err)
	}
	if err := faceYellowStep(m, face); err != nil {
		return fmt.Errorf("yellow story %s: face hidden event: %w", id, err)
	}
	m.Tap(emu.A, 3, 7)
	return settleYellowStoryProgress(m, romData, id, false)
}

// AcquirePokedex owns Yellow's parcel round trip. The Viridian Mart parcel is
// an automatic map script; Oak's lab interaction then consumes it and runs the
// rival/Oak Pokédex cutscene. Both legs resume from durable Yellow facts.
func AcquirePokedex(m *emu.Emu, romData []byte) error {
	if m == nil {
		return fmt.Errorf("yellow Pokédex: nil emulator")
	}
	if done, err := yellowStoryHas(m, romData, gen1.ProgressPokedexAcquired); err != nil {
		return err
	} else if done {
		return nil
	}
	if ready, err := yellowStoryHas(m, romData, yellowprofile.ProgressYellowLabRivalResolved); err != nil {
		return err
	} else if !ready {
		if err := GetPikachuStarter(m, romData); err != nil {
			return fmt.Errorf("yellow Pokédex: finish opening: %w", err)
		}
	}

	parcel, err := yellowStoryHas(m, romData, yellowprofile.ProgressYellowOaksParcelReceived)
	if err != nil {
		return err
	}
	if !parcel {
		if err := GoTo(m, romData, yellowViridianMartMap, 3, 6); err != nil {
			return fmt.Errorf("yellow Pokédex: reach Viridian Mart: %w", err)
		}
		if err := settleYellowStoryProgress(m, romData, yellowprofile.ProgressYellowOaksParcelReceived, false); err != nil {
			return fmt.Errorf("yellow Pokédex: receive Oak's Parcel: %w", err)
		}
	}

	if err := GoTo(m, romData, mapOaksLab, 5, 3); err != nil {
		return fmt.Errorf("yellow Pokédex: return to Oak's lab: %w", err)
	}
	if err := yellowStoryInteract(m, romData, 5, 2, gen1.ProgressPokedexAcquired, false); err != nil {
		return fmt.Errorf("yellow Pokédex: deliver parcel: %w", err)
	}
	return nil
}

// DefeatBrock is the shared Boulder milestone executed with Yellow-native map,
// battle and badge state.
func DefeatBrock(m *emu.Emu, romData []byte) error {
	if done, err := yellowStoryHas(m, romData, gen1.ProgressBoulderBadge); err != nil {
		return err
	} else if done {
		return nil
	}
	if err := GoTo(m, romData, yellowPewterGymMap, 4, 2); err != nil {
		return fmt.Errorf("yellow Brock: reach Pewter Gym: %w", err)
	}
	if err := yellowStoryInteract(m, romData, 4, 1, gen1.ProgressBoulderBadge, false); err != nil {
		return fmt.Errorf("yellow Brock: challenge leader: %w", err)
	}
	return nil
}

// AcquireMtMoonFossil defeats the Super Nerd and deliberately chooses the Dome
// Fossil. Jessie/James are a separate Yellow override so a save made between
// the fossil and the exit battle remains resumable.
func AcquireMtMoonFossil(m *emu.Emu, romData []byte) error {
	if done, err := yellowStoryHas(m, romData, gen1.ProgressMtMoonFossilAcquired); err != nil {
		return err
	} else if done {
		return nil
	}
	defeated, err := yellowStoryHas(m, romData, yellowprofile.ProgressYellowMtMoonSuperNerdDefeated)
	if err != nil {
		return err
	}
	if !defeated {
		if err := GoTo(m, romData, yellowMtMoonB2FMap, 13, 8); err != nil {
			return fmt.Errorf("yellow Mt. Moon: reach Super Nerd trigger: %w", err)
		}
		if err := settleYellowStoryProgress(m, romData, yellowprofile.ProgressYellowMtMoonSuperNerdDefeated, false); err != nil {
			return fmt.Errorf("yellow Mt. Moon: defeat Super Nerd: %w", err)
		}
	}
	if err := yellowStoryInteract(m, romData, 12, 6, gen1.ProgressMtMoonFossilAcquired, true); err != nil {
		return fmt.Errorf("yellow Mt. Moon: take Dome Fossil: %w", err)
	}
	return nil
}

// ResolveMtMoonExit owns Yellow's extra Jessie/James battle after the fossil.
// The trigger is the game's scripted coordinate (3,5), not a guessed NPC talk.
func ResolveMtMoonExit(m *emu.Emu, romData []byte) error {
	if done, err := yellowStoryHas(m, romData, yellowprofile.ProgressYellowMtMoonExitResolved); err != nil {
		return err
	} else if done {
		return nil
	}
	if fossil, err := yellowStoryHas(m, romData, gen1.ProgressMtMoonFossilAcquired); err != nil {
		return err
	} else if !fossil {
		return fmt.Errorf("yellow Mt. Moon exit: fossil milestone is not complete")
	}
	if err := GoTo(m, romData, yellowMtMoonB2FMap, 3, 5); err != nil {
		return fmt.Errorf("yellow Mt. Moon exit: reach Jessie/James trigger: %w", err)
	}
	return settleYellowStoryProgress(m, romData, yellowprofile.ProgressYellowMtMoonExitResolved, false)
}

// AcquireSSTicket drives Bill's Yellow-specific transformation sequence using
// durable sub-events. The hidden PC event is activated from the real player
// coordinate (1,4) while facing up.
func AcquireSSTicket(m *emu.Emu, romData []byte) error {
	if done, err := yellowStoryHas(m, romData, gen1.ProgressSSTicketAcquired); err != nil {
		return err
	} else if done {
		return nil
	}
	if err := GoTo(m, romData, yellowBillsHouseMap, 3, 6); err != nil {
		return fmt.Errorf("yellow Bill: reach house: %w", err)
	}

	ready, err := yellowStoryHas(m, romData, yellowprofile.ProgressYellowBillSeparatorReady)
	if err != nil {
		return err
	}
	if !ready {
		if err := yellowStoryInteract(m, romData, 6, 5, yellowprofile.ProgressYellowBillSeparatorReady, true); err != nil {
			return fmt.Errorf("yellow Bill: agree to help: %w", err)
		}
	}

	used, err := yellowStoryHas(m, romData, yellowprofile.ProgressYellowBillSeparatorUsed)
	if err != nil {
		return err
	}
	if !used {
		if err := yellowStoryHiddenEvent(m, romData, yellowBillsHouseMap, 1, 4, world.StepUp,
			yellowprofile.ProgressYellowBillSeparatorUsed); err != nil {
			return fmt.Errorf("yellow Bill: use cell separator: %w", err)
		}
	}

	if done, err := yellowStoryHas(m, romData, gen1.ProgressSSTicketAcquired); err != nil {
		return err
	} else if done {
		return nil
	}
	// A save can land after the separator event but before the automatic ticket
	// dialogue has committed. Bill's human sprite is then at (4,4).
	if err := yellowStoryInteract(m, romData, 4, 4, gen1.ProgressSSTicketAcquired, false); err != nil {
		return fmt.Errorf("yellow Bill: receive S.S. Ticket: %w", err)
	}
	return nil
}

// AcquireHM01 follows the real ticket-gated harbor route. The 2F rival is a
// coordinate-triggered battle; GoTo's bounded interruption recovery resolves
// it before the Captain interaction gives HM01.
func AcquireHM01(m *emu.Emu, romData []byte) error {
	if done, err := yellowStoryHas(m, romData, gen1.ProgressHM01Acquired); err != nil {
		return err
	} else if done {
		return nil
	}
	if ticket, err := yellowStoryHas(m, romData, gen1.ProgressSSTicketAcquired); err != nil {
		return err
	} else if !ticket {
		return fmt.Errorf("yellow S.S. Anne: S.S. Ticket is not owned")
	}
	if err := GoTo(m, romData, yellowSSAnneCaptainsRoom, 3, 3); err != nil {
		return fmt.Errorf("yellow S.S. Anne: reach Captain: %w", err)
	}
	if err := yellowStoryInteract(m, romData, 4, 2, gen1.ProgressHM01Acquired, false); err != nil {
		return fmt.Errorf("yellow S.S. Anne: receive HM01: %w", err)
	}
	return nil
}
