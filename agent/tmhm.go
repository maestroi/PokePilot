package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// offerWithTMHM extends the portable objective menu with Red-owned progression,
// opportunistic pickup context, targeted party training, known-habitat catch
// transitions, and owned machines. Generic Offer retains its portable
// lead-training and pickup actions; this adapter can additionally prove whether
// each concrete Red party member is trainable, which already-observed species
// can be hunted on a reachable map, and whether a machine pickup is immediately
// useful.
func offerWithTMHM(m *emu.Emu, romData []byte, obs Observation, known *Knowledge) []Objective {
	out := OfferWithProgression(obs, known, newRedObjectiveAdapter(m, romData))
	out = filterRedProgressionStageObjectives(obs, out)
	out = filterRedScriptedTalkObjectives(obs, out)
	out = filterRedServiceTalkObjectives(romData, obs, out)
	out = appendRedNPCRewardObjectives(obs, known, out)
	out = appendKnownCatchObjectives(romData, obs, known, out)
	out = appendDexCatchObjectives(obs, known, out)
	out = appendDexGiftObjectives(obs, known, out)
	out = appendDexTradeObjectives(romData, obs, known, out)
	out = appendDexEvolutionObjectives(obs, known, out)
	var mem state.Mem
	state.Snapshot(m, &mem)
	party := state.DecodeParty(&mem)
	out = enhancePickupObjectives(romData, party, obs, out)
	out = insertPartyTrainingObjectives(obs, known, out, func(slot, targetLevel int) (TrainingEstimate, error) {
		return currentPartyTrainingEstimate(&mem, romData, obs.Map, slot, targetLevel, trainSessionBattleBudget)
	})
	return appendTMHMObjectives(romData, party, state.DecodeInventory(&mem), out)
}

const (
	route22MapID      uint8 = 0x21
	route22RivalHomeX uint8 = 25
	route22RivalHomeY uint8 = 5

	mtMoonPokecenterMapID       uint8 = 0x44
	mtMoonMagikarpSalesmanHomeX uint8 = 10
	mtMoonMagikarpSalesmanHomeY uint8 = 6
)

// filterRedScriptedTalkObjectives removes Red map objects that look like plain
// people in ROM data but are actually owned by map scripts rather than the A
// button interaction path. Route 22's rival is the first measured case: his
// object home is (25,5), but both rival encounters are coordinate-triggered
// story sequences. The first one starts when the player steps on (29,4)/(29,5)
// and is already driven by GetPokeBalls; trying to TalkAt the moving sprite can
// reach him at (28,5) and then fail because A correctly opens no dialogue.
//
// Keep this filter in the Red adapter instead of generic Offer: script-owned
// actors are game facts, while the portable objective layer should remain able
// to offer ordinary people on arbitrary games/maps.
func filterRedScriptedTalkObjectives(obs Observation, out []Objective) []Objective {
	if obs.Map != route22MapID {
		return out
	}
	filtered := make([]Objective, 0, len(out))
	for _, o := range out {
		if o.Kind == KindTalk && o.X == route22RivalHomeX && o.Y == route22RivalHomeY {
			continue
		}
		filtered = append(filtered, o)
	}
	return filtered
}

// redGenericTalkOwnedElsewhere is the single Red ownership test shared by the
// offer filter and the executor. The offer filter is the normal path, but the
// executor must enforce the same boundary as a last line of defense for stale
// checkpoints, direct/repro objectives, or future callers that bypass Offer.
// Generic Talk is never allowed to enter a known service or gameplay choice.
func redGenericTalkOwnedElsewhere(romData []byte, mapID, x, y uint8) bool {
	if redOwnedChoiceActor(mapID, x, y) {
		return true
	}
	actors, err := rom.SpecialInteractionActors(romData, mapID)
	if err != nil {
		return false
	}
	for _, actor := range actors {
		if actor.X == x && actor.Y == y {
			return true
		}
	}
	return false
}

// validateRedTalkObjective fails before controller input when a generic Talk
// targets an interaction owned by a semantic service/choice action. Wrapping
// ErrNoDialogue deliberately maps this to the existing recoverable "blocked"
// outcome: the boundary is still stable and Run can replan instead of turning
// a stale/unsafe Talk objective into a terminal unanswered choice.
func validateRedTalkObjective(romData []byte, mapID uint8, o Objective) error {
	if o.Kind != KindTalk || !redGenericTalkOwnedElsewhere(romData, mapID, o.X, o.Y) {
		return nil
	}
	return fmt.Errorf("interaction at (%d,%d) is owned by a semantic action, not generic talk: %w", o.X, o.Y, skill.ErrNoDialogue)
}

// filterRedServiceTalkObjectives removes actors whose A-button interaction is
// owned by another semantic action instead of ordinary NPC dialogue. The ROM's
// TX_SCRIPT_* dispatch bytes identify nurses, Mart clerks, cable-club staff and
// other built-in services without maintaining a coordinate list. Custom
// text_asm choice actors are classified separately: fishing gurus and Oak's
// aides are owned by semantic reward objectives, while paid/other choice actors
// are suppressed until their owning verb executes them deliberately.
func filterRedServiceTalkObjectives(romData []byte, obs Observation, out []Objective) []Objective {
	filtered := make([]Objective, 0, len(out))
	for _, o := range out {
		if o.Kind == KindTalk && redGenericTalkOwnedElsewhere(romData, obs.Map, o.X, o.Y) {
			continue
		}
		filtered = append(filtered, o)
	}
	return filtered
}

// redOwnedChoiceActor identifies Red NPCs whose interaction immediately asks a
// gameplay choice that generic KindTalk does not own. Keep this classification
// at the game-adapter seam: the core concept is "choice-owned interaction",
// while the concrete map/object identities are Red facts.
func redOwnedChoiceActor(mapID, x, y uint8) bool {
	if mapID == mtMoonPokecenterMapID && x == mtMoonMagikarpSalesmanHomeX && y == mtMoonMagikarpSalesmanHomeY {
		return true
	}
	return redChoiceInteractionActor(mapID, x, y)
}

func appendTMHMObjectives(romData []byte, party state.PartyState, inventory state.InventoryState, out []Objective) []Objective {
	for _, item := range inventory.Items {
		if item.Quantity == 0 {
			continue
		}
		machine, err := rom.LookupTMHM(romData, item.ID)
		if err != nil {
			continue
		}
		decision, err := skill.DecideTMHM(romData, party, item.ID, false)
		if err != nil || decision.Existing {
			continue
		}
		out = append(out, Objective{
			Kind: KindUseItem,
			Item: machineItemID(machine),
			Slot: decision.PartySlot,
			Note: tmhmDecisionNote(machine, decision),
		})
	}
	return out
}

func tmhmDecisionNote(machine rom.Machine, decision skill.TMHMDecision) string {
	name := fmt.Sprintf("TM%02d", machine.Number)
	semantics := "consumable/finite"
	if machine.HM {
		name = fmt.Sprintf("HM%02d", machine.Number-rom.NumTMs)
		semantics = "reusable item; learned HM cannot be forgotten"
	}
	placement := "empty move slot"
	if decision.ReplaceSlot >= 0 {
		placement = fmt.Sprintf("replace move slot %d", decision.ReplaceSlot)
	}
	return fmt.Sprintf("(%s teaches move %d; %s; party slot %d score %d->%d; %s)",
		name, machine.Move, semantics, decision.PartySlot, decision.BeforeScore, decision.AfterScore, placement)
}

// normalizeObjectiveBoundary is Pokémon Red's implementation of the portable
// boundary contract. It may perform only semantically reversible cleanup: page
// ordinary text and back out of a menu whose meaning is unambiguously "back".
// It never answers a gameplay choice, starts/finishes a battle, or guesses
// through an unknown non-controllable state.
func normalizeObjectiveBoundary(m *emu.Emu) error {
	const maxPasses = 4
	for pass := 0; pass < maxPasses; pass++ {
		var mem state.Mem
		state.Snapshot(m, &mem)
		if state.Controllable(&mem) && state.DecodeDialogue(&mem) == nil && !state.MenuUp(&mem) {
			return nil
		}
		if state.DecodeBattle(&mem) != nil {
			return fmt.Errorf("%w: battle still in progress", ErrObjectiveBoundaryDirty)
		}
		if skill.DismissableObjectiveMenu(&mem) {
			if err := skill.CloseOpenMenuToOverworld(m); err != nil {
				return fmt.Errorf("%w: close leftover menu: %v", ErrObjectiveBoundaryDirty, err)
			}
			continue
		}
		if state.DecodeTwoOptionMenu(&mem) != nil {
			return ErrObjectiveBoundaryChoice
		}
		if state.MenuUp(&mem) {
			return fmt.Errorf("%w: non-dismissable menu remains open", ErrObjectiveBoundaryDirty)
		}
		if state.DecodeDialogue(&mem) != nil {
			res := skill.RecoverDialogue(m, roundRecoveryBudget)
			switch res.Stop {
			case skill.DialogueRecovered:
				continue
			case skill.DialogueMenuOpen:
				// The text page legitimately transitioned into a menu. Recovery
				// must not press A there, but that is not a dirty boundary by
				// itself: loop once more so the typed menu cleanup above owns it.
				continue
			case skill.DialogueChoiceRequired:
				return ErrObjectiveBoundaryChoice
			default:
				return fmt.Errorf("%w: leftover dialogue did not recover: %s", ErrObjectiveBoundaryDirty, recoveryStopName(res.Stop))
			}
		}
		return fmt.Errorf("%w: player is not controllable and no recoverable menu or dialogue is open", ErrObjectiveBoundaryDirty)
	}
	return fmt.Errorf("%w: cleanup did not converge after %d passes", ErrObjectiveBoundaryDirty, maxPasses)
}

func prepareObjectiveBoundary(m *emu.Emu) error {
	return normalizeObjectiveBoundary(m)
}

func settleObjectiveBoundary(m *emu.Emu) error {
	return normalizeObjectiveBoundary(m)
}
