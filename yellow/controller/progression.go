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
	yellowCeruleanGymMap     uint8 = 0x41
	yellowVermilionGymMap    uint8 = 0x5c
	yellowCeladonGymMap      uint8 = 0x86
	yellowLavenderCenterMap  uint8 = 0x8d
	yellowCeladonCenterMap   uint8 = 0x85
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
	idle := 0
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
			idle = 0
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
			idle = 0
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
			idle = 0
			m.Tap(emu.A, 3, 7)
			continue
		}
		if obs.Controllable && !complete {
			// Map-entry triggers can begin a few frames after Travel's arrival
			// boundary. Give them a small bounded grace period, but never turn
			// that grace into blind input after control has genuinely returned.
			idle++
			if idle >= 90 {
				return fmt.Errorf("yellow story %s: interaction settled before progress committed on map %#02x at (%d,%d)",
					id, m.Peek8(sym.CurMap), m.Peek8(sym.XCoord), m.Peek8(sym.YCoord))
			}
			m.StepFrame()
			continue
		}
		idle = 0
		m.StepFrame()
	}
	return fmt.Errorf("yellow story %s: exceeded %d frames", id, yellowStoryFrameBudget)
}

func yellowStoryInteract(m *emu.Emu, romData []byte, x, y uint8, id game.ProgressID, acceptYes bool) error {
	for attempt := 0; attempt < 4; attempt++ {
		if err := interactAt(m, romData, int(x), int(y)); err != nil {
			recovered, recoverErr := recoverYellowTravelInterruption(m, romData)
			if recoverErr != nil {
				return fmt.Errorf("yellow story %s: recover approach to (%d,%d): %w", id, x, y, recoverErr)
			}
			if recovered {
				continue
			}
			return fmt.Errorf("yellow story %s: interact at (%d,%d): %w", id, x, y, err)
		}
		return settleYellowStoryProgress(m, romData, id, acceptYes)
	}
	return fmt.Errorf("yellow story %s: repeated interruptions approaching (%d,%d)", id, x, y)
}

func yellowStoryHiddenEvent(m *emu.Emu, romData []byte, x, y uint8, id game.ProgressID) error {
	for attempt := 0; attempt < 4; attempt++ {
		if err := interactAt(m, romData, int(x), int(y)); err != nil {
			recovered, recoverErr := recoverYellowTravelInterruption(m, romData)
			if recoverErr != nil {
				return fmt.Errorf("yellow story %s: recover hidden event (%d,%d): %w", id, x, y, recoverErr)
			}
			if recovered {
				continue
			}
			return fmt.Errorf("yellow story %s: hidden event at (%d,%d): %w", id, x, y, err)
		}
		return settleYellowStoryProgress(m, romData, id, false)
	}
	return fmt.Errorf("yellow story %s: repeated interruptions at hidden event (%d,%d)", id, x, y)
}

func yellowStoryHiddenEventFrom(m *emu.Emu, romData []byte, mapID, standX, standY uint8, face world.Step, id game.ProgressID) error {
	if err := GoTo(m, romData, mapID, standX, standY); err != nil {
		return fmt.Errorf("yellow story %s: reach hidden-event stand tile (%d,%d): %w", id, standX, standY, err)
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
// durable sub-events. The PC hidden-event target is (1,4); the player stands
// immediately below it at (1,5) facing up.
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
		if err := yellowStoryHiddenEventFrom(m, romData, yellowBillsHouseMap, 1, 5, world.StepUp,
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

func yellowVermilionTrashCanCoords(index uint8) (x, y uint8, ok bool) {
	if index >= 15 {
		return 0, 0, false
	}
	return 1 + 2*(index/3), 7 + 2*(index%3), true
}

func yellowSecondTrashCanIndex(m *emu.Emu) (uint8, bool) {
	for _, index := range []uint8{
		m.Peek8(sym.SecondLockTrashCanIndex),
		m.Peek8(sym.SecondLockTrashCanIndex + 1),
	} {
		if index < 15 {
			return index, true
		}
	}
	return 0, false
}

func yellowResetInvalidSurgeSecondLock(m *emu.Emu, romData []byte) error {
	first := m.Peek8(sym.FirstLockTrashCanIndex)
	for index := uint8(0); index < 15; index++ {
		if index == first {
			continue
		}
		x, y, _ := yellowVermilionTrashCanCoords(index)
		if err := interactAt(m, romData, int(x), int(y)); err != nil {
			recovered, recoverErr := recoverYellowTravelInterruption(m, romData)
			if recoverErr != nil {
				return recoverErr
			}
			if recovered {
				continue
			}
			return err
		}
		if _, err := RecoverDialogue(m, romData); err != nil {
			return err
		}
		open, err := yellowStoryHas(m, romData, yellowprofile.ProgressYellowVermilionFirstLockOpen)
		if err != nil {
			return err
		}
		if !open {
			return nil
		}
	}
	return fmt.Errorf("yellow Surge puzzle: could not reset invalid sampled second-switch state")
}

func ensureYellowCutCarrier(m *emu.Emu, romData []byte) error {
	cap := fieldCapabilityFor(m, romData, FieldCut)
	if !cap.Usable && !cap.Preparable {
		// Yellow guarantees a Cut-compatible Charmander gift on Route 24.
		// Use it as the bounded recovery path when the current roster cannot
		// carry HM01, rather than letting a legitimate speedrun dead-end.
		if m.Peek8(sym.PartyCount) >= 6 {
			return fmt.Errorf("yellow Cut recovery: no compatible party member and party is full")
		}
		if err := ReceiveGift(m, romData, 0xb0); err != nil {
			return fmt.Errorf("yellow Cut recovery: receive Route 24 Charmander: %w", err)
		}
	}
	if _, err := EnsureFieldMove(m, romData, FieldCut); err != nil {
		return err
	}
	return nil
}

func openYellowVermilionGym(m *emu.Emu, romData []byte) error {
	if done, err := yellowStoryHas(m, romData, yellowprofile.ProgressYellowVermilionGateOpen); err != nil {
		return err
	} else if done {
		return nil
	}
	if m.Peek8(sym.CurMap) != yellowVermilionGymMap {
		return fmt.Errorf("yellow Surge puzzle: on map %#02x, want %#02x", m.Peek8(sym.CurMap), yellowVermilionGymMap)
	}

	for attempt := 0; attempt < 4; attempt++ {
		firstOpen, err := yellowStoryHas(m, romData, yellowprofile.ProgressYellowVermilionFirstLockOpen)
		if err != nil {
			return err
		}
		if !firstOpen {
			index := m.Peek8(sym.FirstLockTrashCanIndex)
			x, y, ok := yellowVermilionTrashCanCoords(index)
			if !ok {
				return fmt.Errorf("yellow Surge puzzle: first switch index %d is outside 0..14", index)
			}
			if err := yellowStoryHiddenEvent(m, romData, x, y, yellowprofile.ProgressYellowVermilionFirstLockOpen); err != nil {
				return fmt.Errorf("yellow Surge puzzle: first switch %d at (%d,%d): %w", index, x, y, err)
			}
		}

		if done, err := yellowStoryHas(m, romData, yellowprofile.ProgressYellowVermilionGateOpen); err != nil {
			return err
		} else if done {
			return nil
		}
		second, ok := yellowSecondTrashCanIndex(m)
		if !ok {
			// Yellow's three-choice sampler has a register bug and can read a
			// pair outside the 0..14 can range. Deliberately touch a wrong can
			// to make the game reset the first lock, then retry from live state.
			if err := yellowResetInvalidSurgeSecondLock(m, romData); err != nil {
				return err
			}
			continue
		}
		x, y, _ := yellowVermilionTrashCanCoords(second)
		err = yellowStoryHiddenEvent(m, romData, x, y, yellowprofile.ProgressYellowVermilionGateOpen)
		if err == nil {
			return nil
		}
		// Yellow can reset the first lock after a wrong/stale second attempt.
		// Re-read live puzzle state rather than preserving stale indices.
		firstStillOpen, stateErr := yellowStoryHas(m, romData, yellowprofile.ProgressYellowVermilionFirstLockOpen)
		if stateErr != nil {
			return stateErr
		}
		if firstStillOpen {
			return fmt.Errorf("yellow Surge puzzle: second switch %d at (%d,%d): %w", second, x, y, err)
		}
	}
	return fmt.Errorf("yellow Surge puzzle: exceeded reset recovery budget")
}

// DefeatMisty acquires the Cascade Badge required to legally use HM01 Cut.
func DefeatMisty(m *emu.Emu, romData []byte) error {
	if done, err := yellowStoryHas(m, romData, gen1.ProgressCascadeBadge); err != nil {
		return err
	} else if done {
		return nil
	}
	if err := GoTo(m, romData, yellowCeruleanGymMap, 4, 3); err != nil {
		return fmt.Errorf("yellow Misty: reach Cerulean Gym: %w", err)
	}
	if err := yellowStoryInteract(m, romData, 4, 2, gen1.ProgressCascadeBadge, false); err != nil {
		return fmt.Errorf("yellow Misty: challenge leader: %w", err)
	}
	return nil
}

// DefeatSurge prepares Cut, enters the tree-gated gym, solves Yellow's live
// two-candidate trash-can puzzle, and verifies the Thunder Badge.
func DefeatSurge(m *emu.Emu, romData []byte) error {
	if done, err := yellowStoryHas(m, romData, gen1.ProgressThunderBadge); err != nil {
		return err
	} else if done {
		return nil
	}
	if cascade, err := yellowStoryHas(m, romData, gen1.ProgressCascadeBadge); err != nil {
		return err
	} else if !cascade {
		return fmt.Errorf("yellow Surge: Cascade Badge is required for Cut")
	}
	if hm01, err := yellowStoryHas(m, romData, gen1.ProgressHM01Acquired); err != nil {
		return err
	} else if !hm01 {
		return fmt.Errorf("yellow Surge: HM01 is not owned")
	}
	if err := ensureYellowCutCarrier(m, romData); err != nil {
		return fmt.Errorf("yellow Surge: prepare Cut: %w", err)
	}
	if err := GoTo(m, romData, yellowVermilionGymMap, 5, 15); err != nil {
		return fmt.Errorf("yellow Surge: enter Vermilion Gym: %w", err)
	}
	if err := openYellowVermilionGym(m, romData); err != nil {
		return err
	}
	if err := GoTo(m, romData, yellowVermilionGymMap, 5, 2); err != nil {
		return fmt.Errorf("yellow Surge: cross opened gym gate: %w", err)
	}
	if err := yellowStoryInteract(m, romData, 5, 1, gen1.ProgressThunderBadge, false); err != nil {
		return fmt.Errorf("yellow Surge: challenge leader: %w", err)
	}
	return nil
}

// ReachLavender owns the bounded post-Surge Route 9/Rock Tunnel leg. Flash is
// optional for ROM-driven routing; GoTo will automatically use it when the
// party can prepare it.
func ReachLavender(m *emu.Emu, romData []byte) error {
	if done, err := yellowStoryHas(m, romData, gen1.ProgressPostSurgeLavenderReached); err != nil {
		return err
	} else if done {
		return nil
	}
	if thunder, err := yellowStoryHas(m, romData, gen1.ProgressThunderBadge); err != nil {
		return err
	} else if !thunder {
		return fmt.Errorf("yellow Rock Tunnel: Thunder Badge milestone is incomplete")
	}
	if err := ensureYellowCutCarrier(m, romData); err != nil {
		return fmt.Errorf("yellow Rock Tunnel: prepare Cut: %w", err)
	}
	if err := GoTo(m, romData, yellowLavenderCenterMap, 3, 3); err != nil {
		return fmt.Errorf("yellow Rock Tunnel: reach Lavender: %w", err)
	}
	if done, err := yellowStoryHas(m, romData, gen1.ProgressPostSurgeLavenderReached); err != nil {
		return err
	} else if !done {
		return fmt.Errorf("yellow Rock Tunnel: Lavender checkpoint not observed after arrival")
	}
	return nil
}

// ReachCeladonRecovered owns the Lavender -> Underground Path -> Celadon leg
// and leaves the party fully recovered for Erika.
func ReachCeladonRecovered(m *emu.Emu, romData []byte) error {
	if done, err := yellowStoryHas(m, romData, gen1.ProgressPostSurgeCeladonReady); err != nil {
		return err
	} else if done {
		return nil
	}
	if lavender, err := yellowStoryHas(m, romData, gen1.ProgressPostSurgeLavenderReached); err != nil {
		return err
	} else if !lavender {
		return fmt.Errorf("yellow Celadon: Lavender checkpoint is incomplete")
	}
	if err := GoTo(m, romData, yellowCeladonCenterMap, 3, 3); err != nil {
		return fmt.Errorf("yellow Celadon: reach Pokemon Center: %w", err)
	}
	if err := Heal(m, romData); err != nil {
		return fmt.Errorf("yellow Celadon: heal party: %w", err)
	}
	if done, err := yellowStoryHas(m, romData, gen1.ProgressPostSurgeCeladonReady); err != nil {
		return err
	} else if !done {
		return fmt.Errorf("yellow Celadon: recovered checkpoint not observed after healing")
	}
	return nil
}

// DefeatErika is the final milestone in this bounded campaign slice.
func DefeatErika(m *emu.Emu, romData []byte) error {
	if done, err := yellowStoryHas(m, romData, gen1.ProgressRainbowBadge); err != nil {
		return err
	} else if done {
		return nil
	}
	if ready, err := yellowStoryHas(m, romData, gen1.ProgressPostSurgeCeladonReady); err != nil {
		return err
	} else if !ready {
		return fmt.Errorf("yellow Erika: Celadon recovered checkpoint is incomplete")
	}
	if err := ensureYellowCutCarrier(m, romData); err != nil {
		return fmt.Errorf("yellow Erika: prepare Cut: %w", err)
	}
	if err := GoTo(m, romData, yellowCeladonGymMap, 4, 4); err != nil {
		return fmt.Errorf("yellow Erika: reach Celadon Gym: %w", err)
	}
	if err := yellowStoryInteract(m, romData, 4, 3, gen1.ProgressRainbowBadge, false); err != nil {
		return fmt.Errorf("yellow Erika: challenge leader: %w", err)
	}
	return nil
}
