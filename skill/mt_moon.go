package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

var ErrMtMoonPostcondition = errors.New("skill: Mt. Moon story postcondition failed")

func init() {
	// pokered/data/maps/objects/MtMoonB2F.asm: object home tiles. The
	// approach tile is not adjacent to anything the player talks to — it is
	// where MtMoonB2FDefaultScript checks wXCoord/wYCoord and starts the
	// fight, so standing on it IS the interaction.
	interactionPlaces["mt moon dome fossil"] = Destination{Map: 0x3d, X: 12, Y: 6}
	interactionPlaces["mt moon fossil approach"] = Destination{Map: 0x3d, X: 13, Y: 8}
}

// MtMoonProgressionAvailable reports whether the fossil objective can be
// started from mapID: Route 4, Mt. Moon's three floors, and its Center. The
// objective walks itself to B2F, but only from somewhere the walk is short
// and the corridor is the thing in the way.
func MtMoonProgressionAvailable(mapID uint8) bool {
	return mapID == 0x0f || mapID == 0x3b || mapID == 0x3c || mapID == 0x3d || mapID == 0x44
}

// MtMoonFossil beats Mt. Moon's exit Super Nerd and takes the Dome Fossil,
// which is what makes the deepest floor crossable.
//
// The gate is his body, not the story flag. B2F's fossil corridor narrows to
// (12,8) and (13,8); he stands on the first and stepping on the second is
// what starts his battle, so blocking both leaves no path:
//
//	PROBE_MAP=0x3d PROBE_AT=21,17 PROBE_TO=5,7 PROBE_BLOCK=12,8;13,8
//	  -> world: no path
//
// Beating him does not move him — MtMoonB2FMoveSuperNerdScript runs only
// after a fossil is taken — so "defeated" alone would leave a run walking
// into the same corridor forever, which is exactly what it did for eighty
// rounds of run-17rjs2d1uf1kw3.
//
// The Dome choice is owned here. Travel only reports the missing capability
// and generic boundary cleanup never answers the prompt; an existing Helix
// choice is respected rather than replayed.
func MtMoonFossil(m *emu.Emu, data []byte, policy MovePolicy) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	if state.DecodeStoryFacts(&mem, state.DecodeInventory(&mem)).MtMoonFossilAcquired && state.Controllable(&mem) {
		return nil
	}
	if policy == nil {
		return ErrRouteTransitionNeedsBattlePolicy
	}
	approach, _ := Place("mt moon fossil approach")
	if _, err := TravelFlee(m, data, approach, policy, 40); err != nil {
		return err
	}
	state.Snapshot(m, &mem)
	if !state.HasEvent(&mem, state.EventBeatMtMoonSuperNerd) {
		// The approach tile triggers the map-owned encounter, sometimes a
		// few frames after Travel reaches it. Own that deferred script here.
		if err := driveStoryUntil(m, gymBattleWaitBudget, func(mm *state.Mem) bool { return state.DecodeBattle(mm) != nil }); err != nil {
			return err
		}
		if err := finishStoryBattle(m, "Mt. Moon Super Nerd", policy); err != nil {
			return err
		}
		if err := driveStoryUntil(m, 3000, func(mm *state.Mem) bool {
			return state.HasEvent(mm, state.EventBeatMtMoonSuperNerd) && state.Controllable(mm)
		}); err != nil {
			return fmt.Errorf("%w: %v", ErrMtMoonPostcondition, err)
		}
	}
	state.Snapshot(m, &mem)
	if !state.HasEvent(&mem, state.EventBeatMtMoonSuperNerd) {
		return fmt.Errorf("%w: Super Nerd victory event absent", ErrMtMoonPostcondition)
	}
	if !state.HasEvent(&mem, state.EventGotDomeFossil) && !state.HasEvent(&mem, state.EventGotHelixFossil) {
		if len(state.DecodeInventory(&mem).Items) >= gen1BagCapacity {
			return ErrNoSafeBagSpace
		}
		fossil, _ := Place("mt moon dome fossil")
		if err := talkBeside(m, data, fossil.X, fossil.Y, policy); err != nil {
			return err
		}
		if err := Face(m, fossil.X, fossil.Y); err != nil {
			return err
		}
		m.Tap(emu.A, 3, 7)
		if err := driveStoryUntil(m, 3000, func(mm *state.Mem) bool { return state.DecodeTwoOptionMenu(mm) != nil }); err != nil {
			return err
		}
		if err := selectTwoOption(m, 0); err != nil {
			return err
		}
		if err := driveStoryUntil(m, 3000, func(mm *state.Mem) bool {
			return state.HasEvent(mm, state.EventGotDomeFossil) && state.Controllable(mm)
		}); err != nil {
			return err
		}
	}
	// The fossil script moves the NPC and removes the other fossil after the
	// acquisition flag; settle the whole owned sequence before returning.
	if err := driveStoryUntil(m, 3000, func(mm *state.Mem) bool {
		return state.Controllable(mm) && mm.U8(sym.FontLoaded) == 0 && mm.U8(sym.MtMoonB2FCurScript) == 0
	}); err != nil {
		return err
	}
	state.Snapshot(m, &mem)
	if !state.DecodeStoryFacts(&mem, state.DecodeInventory(&mem)).MtMoonFossilAcquired {
		return ErrMtMoonPostcondition
	}
	return nil
}
