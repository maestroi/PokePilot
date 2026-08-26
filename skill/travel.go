package skill

import (
	"errors"
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// TravelResult reports what happened on the way.
type TravelResult struct {
	Battles    int  // wild battles fought
	BlackedOut bool // at least one battle ended in ResultLost
}

// Travel walks to dest like GoTo, but resolves the wild encounters that
// interrupt a route through tall grass instead of aborting on them.
// Each encounter is fought with policy; GoTo re-plans from RAM on every
// call, so resuming after a battle needs no bookkeeping here.
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
		outcome, err := Battle(m, policy)
		if err != nil {
			return res, fmt.Errorf("skill: Travel: battle %d: %w", res.Battles, err)
		}
		if outcome == state.ResultLost {
			// Losing is a typed outcome, not a failure: GoTo re-plans from
			// the Pokemon Center on the next pass.
			res.BlackedOut = true
		}
	}
}
