// Package story is Pokémon Yellow's story controller: the game-specific
// mechanics that drive Yellow's own scripted progression (the Pikachu
// opening today). It owns the "how"; it never decides success. The agent's
// objective runtime verifies each goal afterwards against the Yellow story
// projection in yellow/profile.
//
// The controller is a resumable phase machine rather than a linear script:
// every step re-reads Yellow's native story flags, picks the one action that
// state calls for, and requires the next read to show progress. A run that
// starts mid-opening (a checkpoint, an interrupted objective) therefore
// resumes where the game actually is instead of replaying from the bedroom.
package story

import (
	"errors"
	"fmt"

	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

// OpeningPhase names the one action Yellow's opening needs next.
type OpeningPhase uint8

const (
	// OpeningWalkToGate: walk to Pallet Town's north exit so Oak stops the
	// player (PalletTownDefaultScript fires at wYCoord == 0).
	OpeningWalkToGate OpeningPhase = iota
	// OpeningAdvanceScript: the game is driving (Oak's Pikachu cutscene, the
	// walk into the lab, the rival taking the Eevee ball, Oak handing over
	// Pikachu, the rival leaving). Only text is advanced.
	OpeningAdvanceScript
	// OpeningTakeBall: Oak asked the player to choose; walk under the lone
	// Poké Ball and press A on it so the rival snatches it.
	OpeningTakeBall
	// OpeningWalkToRival: Pikachu is in the party; walk to row 6, where the
	// rival challenges (OaksLabRivalChallengesPlayerScript).
	OpeningWalkToRival
	// OpeningFightRival: the lab battle is running.
	OpeningFightRival
	// OpeningDone: the lab rival battle is resolved and control is back.
	OpeningDone
)

func (p OpeningPhase) String() string {
	switch p {
	case OpeningWalkToGate:
		return "walk_to_gate"
	case OpeningAdvanceScript:
		return "advance_script"
	case OpeningTakeBall:
		return "take_ball"
	case OpeningWalkToRival:
		return "walk_to_rival"
	case OpeningFightRival:
		return "fight_rival"
	case OpeningDone:
		return "done"
	}
	return fmt.Sprintf("opening_phase(%d)", uint8(p))
}

// OpeningMilestone is the story fact an opening run is asked to reach.
type OpeningMilestone uint8

const (
	// MilestoneStarterReceived: Pikachu is in the party (EVENT_GOT_STARTER).
	MilestoneStarterReceived OpeningMilestone = iota + 1
	// MilestoneLabRivalResolved: the lab rival battle is over
	// (EVENT_BATTLED_RIVAL_IN_OAKS_LAB). This is the end of the opening.
	MilestoneLabRivalResolved
)

func (m OpeningMilestone) String() string {
	switch m {
	case MilestoneStarterReceived:
		return "starter_received"
	case MilestoneLabRivalResolved:
		return "lab_rival_resolved"
	}
	return fmt.Sprintf("opening_milestone(%d)", uint8(m))
}

// Reached reports whether f satisfies the milestone at a stable boundary.
// Control must be back: a flag set mid-script is not yet a place the next
// objective can start from.
func (m OpeningMilestone) Reached(f yellowprofile.OpeningFacts) bool {
	if !f.Controllable || f.InBattle {
		return false
	}
	switch m {
	case MilestoneStarterReceived:
		return f.GotStarter && f.PartyCount > 0
	case MilestoneLabRivalResolved:
		return f.BattledRival
	}
	return false
}

// ErrOpeningStalled is returned when an opening step leaves Yellow's story
// state exactly as it found it twice in a row: the controller can no longer
// tell what drives the game forward, so it stops instead of looping.
var ErrOpeningStalled = errors.New("yellow story: opening made no progress")

// ErrOpeningUnexpectedState is returned when the observed state does not fit
// any opening phase (for example Oak's lab flags set with the player outside
// the lab and in control). The controller refuses to guess.
var ErrOpeningUnexpectedState = errors.New("yellow story: opening state has no owning phase")

// PhaseFor chooses the next opening action from the observed facts alone.
// It is pure so the whole decision table is testable without a ROM.
func PhaseFor(f yellowprofile.OpeningFacts) (OpeningPhase, error) {
	switch {
	case f.BattledRival && f.Controllable && !f.InBattle:
		return OpeningDone, nil
	case f.InBattle:
		// Before Oak appears the only battle is his scripted Pikachu catch,
		// which plays itself; the lab battle is the player's to fight.
		if f.GotStarter && !f.BattledRival {
			return OpeningFightRival, nil
		}
		return OpeningAdvanceScript, nil
	case !f.Controllable:
		return OpeningAdvanceScript, nil
	case !f.OakAppeared && !f.FollowedOak:
		return OpeningWalkToGate, nil
	case !f.OakAskedToChoose:
		// Oak appeared (or the lab was entered) but the choose speech has
		// not run yet, and the player is nominally in control: the script
		// between them is still settling.
		return OpeningAdvanceScript, nil
	case !f.GotStarter:
		if f.Map != yellowprofile.OaksLabMap {
			return 0, fmt.Errorf("%w: Oak asked to choose but player is on map %#04x at (%d,%d)",
				ErrOpeningUnexpectedState, f.Map, f.X, f.Y)
		}
		return OpeningTakeBall, nil
	case !f.BattledRival:
		if f.Map != yellowprofile.OaksLabMap {
			return 0, fmt.Errorf("%w: Pikachu received but player left the lab (map %#04x at (%d,%d)) before the rival battle",
				ErrOpeningUnexpectedState, f.Map, f.X, f.Y)
		}
		return OpeningWalkToRival, nil
	}
	return OpeningDone, nil
}

// OpeningDriver performs one opening action against a live game. Each
// method only claims the mechanics ran; RunOpening re-reads Facts to decide
// whether they worked.
type OpeningDriver interface {
	Facts() yellowprofile.OpeningFacts
	WalkToGate() error
	AdvanceScript() error
	TakeBall() error
	WalkToRival() error
	FightRival() error
}

// maxOpeningSteps bounds one RunOpening call. The full opening from the
// bedroom is at most a dozen phase changes; a run that needs more is looping.
const maxOpeningSteps = 32

// RunOpening drives Yellow's opening until target is reached. It returns nil
// only when target.Reached holds on a fresh read. An action error is
// tolerated when the next read shows the story moved (a scripted interrupt
// is often the very trigger the action was walking into); two consecutive
// steps with identical facts end the run with ErrOpeningStalled.
func RunOpening(d OpeningDriver, target OpeningMilestone) error {
	if d == nil {
		return fmt.Errorf("yellow story: nil opening driver")
	}
	facts := d.Facts()
	stalls := 0
	for step := 0; step < maxOpeningSteps; step++ {
		if target.Reached(facts) {
			return nil
		}
		phase, err := PhaseFor(facts)
		if err != nil {
			return err
		}
		if phase == OpeningDone {
			// The opening is over but the target is not reached: only a
			// target outside the opening can get here.
			return fmt.Errorf("%w: opening finished without %s", ErrOpeningUnexpectedState, target)
		}
		actionErr := runPhase(d, phase)
		next := d.Facts()
		if next == facts {
			if actionErr != nil {
				return fmt.Errorf("yellow story: opening %s: %w", phase, actionErr)
			}
			stalls++
			if stalls >= 2 {
				return fmt.Errorf("%w: phase %s on map %#04x at (%d,%d)", ErrOpeningStalled, phase, facts.Map, facts.X, facts.Y)
			}
			continue
		}
		stalls = 0
		facts = next
	}
	if target.Reached(facts) {
		return nil
	}
	return fmt.Errorf("%w: %s not reached within %d steps (map %#04x at (%d,%d))",
		ErrOpeningStalled, target, maxOpeningSteps, facts.Map, facts.X, facts.Y)
}

func runPhase(d OpeningDriver, phase OpeningPhase) error {
	switch phase {
	case OpeningWalkToGate:
		return d.WalkToGate()
	case OpeningAdvanceScript:
		return d.AdvanceScript()
	case OpeningTakeBall:
		return d.TakeBall()
	case OpeningWalkToRival:
		return d.WalkToRival()
	case OpeningFightRival:
		return d.FightRival()
	}
	return fmt.Errorf("yellow story: no action for phase %s", phase)
}
