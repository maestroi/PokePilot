package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

func travelRocketWarp(m *emu.Emu, policy MovePolicy, goTo func() error) error {
	_, err := travel(m, policy, 20,
		goTo,
		func() DialogueRecoveryResult { return RecoverDialogue(m, dialogueRecoveryBudget) },
		func() bool { return m.Peek8(sym.StatusFlags4)&blackoutBit != 0 },
		fightOnly(m, policy),
	)
	return err
}

// descendRocketHideout uses only ordinary Traverse. B2F/B3F forced arrow
// tiles are now local movement-graph edges supplied by the Red adapter, so
// Traverse's field-path approach plans through them and re-plans from live RAM
// after each forced landing exactly like any other navigation action.
func descendRocketHideout(m *emu.Emu, romData []byte, policy MovePolicy) error {
	for {
		switch m.Peek8(sym.CurMap) {
		case rocketHideoutB1FMap:
			edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB1FMap, To: rocketHideoutB2FMap, WarpX: 23, WarpY: 2}
			if err := travelRocketWarp(m, policy, func() error { return Traverse(m, romData, edge) }); err != nil {
				return fmt.Errorf("skill: RocketHideout: B1F -> B2F: %w", err)
			}
		case rocketHideoutB2FMap:
			edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB2FMap, To: rocketHideoutB3FMap, WarpX: 21, WarpY: 8}
			if err := travelRocketWarp(m, policy, func() error { return Traverse(m, romData, edge) }); err != nil {
				return fmt.Errorf("skill: RocketHideout: cross B2F forced-movement floor: %w", err)
			}
		case rocketHideoutB3FMap:
			edge := world.Edge{Kind: world.EdgeWarp, From: rocketHideoutB3FMap, To: rocketHideoutB4FMap, WarpX: 19, WarpY: 18}
			if err := travelRocketWarp(m, policy, func() error { return Traverse(m, romData, edge) }); err != nil {
				return fmt.Errorf("skill: RocketHideout: cross B3F forced-movement floor: %w", err)
			}
		case rocketHideoutB4FMap:
			return nil
		default:
			return fmt.Errorf("skill: RocketHideout: cannot descend from map %#04x", m.Peek8(sym.CurMap))
		}
	}
}
