package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// Replan records the world as Travel re-read it after a battle: the map and
// tile the player stands on. It is the point the remainder of the journey
// was planned from.
type Replan struct {
	Map  uint8
	X, Y uint8
}

// TravelResult reports what happened on the way.
type TravelResult struct {
	Battles    int      // wild battles fought
	BlackedOut bool     // at least one battle ended in ResultLost
	Replans    []Replan // one entry per battle, in order fought
}

// worldStableBudget bounds each settle wait; worldStableFrames is how long
// the world must stand still before Travel trusts a re-read.
const (
	worldStableBudget = 1200
	worldStableFrames = 100
)

// Travel walks to dest like GoTo, but resolves the wild encounters that
// interrupt a route through tall grass instead of aborting on them. Each
// encounter is fought with policy.
//
// After every battle the world is re-read from RAM once it has settled, and
// the remainder is planned from that: a win leaves the player on the
// encounter tile, and a blackout rewrites the position to the center's
// spawn tile before wCurMap flips, so planning from the pre-battle plan —
// or from inside that pre-flip window — would keep walking the map the
// player is leaving.
//
// Only ErrBattle is intercepted: any other GoTo failure (blocked tile, no
// route, ...) is returned unchanged. maxBattles bounds the fight loop;
// zero or negative is an error, not "unlimited".
func Travel(m *emu.Emu, romData []byte, dest Destination, policy MovePolicy, maxBattles int) (TravelResult, error) {
	if maxBattles <= 0 {
		return TravelResult{}, fmt.Errorf("skill: Travel: maxBattles must be > 0, got %d", maxBattles)
	}

	var res TravelResult
	for {
		err := GoTo(m, romData, dest)
		if err == nil {
			return res, nil
		}
		if !errors.Is(err, ErrBattle) {
			return res, err
		}
		if res.Battles >= maxBattles {
			return res, fmt.Errorf("skill: Travel: still interrupted by a wild battle after %d battles (maxBattles): %v",
				maxBattles, err)
		}
		res.Battles++
		pre := currentWorld(m)
		outcome, err := Battle(m, policy)
		if err != nil {
			return res, fmt.Errorf("skill: Travel: battle %d: %w", res.Battles, err)
		}
		if outcome == state.ResultLost {
			// Losing is a typed outcome, not a failure: the next pass
			// re-plans from the Pokemon Center the blackout landed on.
			res.BlackedOut = true
		}
		res.Replans = append(res.Replans, settleWorld(m, pre, outcome == state.ResultLost))
	}
}

// currentWorld reads the map and tile the player stands on from RAM.
func currentWorld(m *emu.Emu) Replan {
	return Replan{m.Peek8(sym.CurMap), m.Peek8(sym.XCoord), m.Peek8(sym.YCoord)}
}

// settleWorld steps until the (map, x, y) triple has stood still for
// worldStableFrames consecutive frames and returns that settled world.
// On a loss it first waits for the map to change: a blackout lands the
// position on the center's spawn tile before wCurMap flips, and that
// pre-flip window is itself stable, so a plain stability wait would settle
// on the stale map (the measured "step down blocked at (5,6)" walked a
// 0x0C plan while on 0x00).
func settleWorld(m *emu.Emu, pre Replan, lost bool) Replan {
	if lost {
		if _, err := m.StepUntil(worldStableBudget, func(m *emu.Emu) bool {
			return m.Peek8(sym.CurMap) != pre.Map
		}); err != nil {
			// ponytail: blackout transition longer than worldStableBudget ->
			// fall through with the last read (today's behavior) rather than
			// failing; raise worldStableBudget if that is ever measured.
		}
	}
	last := currentWorld(m)
	stable := 0
	for i := 0; i < worldStableBudget; i++ {
		m.StepFrame()
		cur := currentWorld(m)
		if cur == last {
			stable++
			if stable >= worldStableFrames {
				return cur
			}
		} else {
			stable = 0
		}
		last = cur
	}
	return last
}
