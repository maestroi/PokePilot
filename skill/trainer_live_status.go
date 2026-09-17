package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/world"
)

// TrainerStatusAtLive is the observation-side trainer status for the current
// loaded map. Unlike the legacy ROM-only reachability check, it decodes the
// live wOverworldMap block buffer so ReplaceTileBlock scripts can close or open
// paths before a generic trainer objective is offered.
func TrainerStatusAtLive(romData []byte, mem *state.Mem, mapID, homeX, homeY uint8) (TrainerStatus, error) {
	h, err := graphForROM(romData).ParseMap(romData, mapID)
	if err != nil {
		return TrainerStatus{}, fmt.Errorf("skill: TrainerStatusAtLive: parse map %#04x: %w", mapID, err)
	}
	target, err := trainerTargetAt(romData, h, homeX, homeY)
	if err != nil {
		return TrainerStatus{}, err
	}
	return TrainerStatus{
		Defeated:      target.flag.setMem(mem),
		Challengeable: ordinaryTrainerClass(target.object.TrainerClass) && genericTrainerReachableLive(romData, mem, h, homeX, homeY),
	}, nil
}

func genericTrainerReachableLive(romData []byte, mem *state.Mem, h rom.MapHeader, homeX, homeY uint8) bool {
	if mem == nil || mem.U8(tablesForROM(romData).wram.CurMap) != h.ID {
		return true
	}
	g, err := liveMapGridFromMem(mem, romData, h)
	if err != nil {
		// Observation remains fail-open on incomplete geometry just like the
		// existing trainer status path; only measured no-path evidence hides an
		// otherwise ordinary trainer.
		return true
	}

	hidden := state.HiddenObjectIDs(mem)
	blocked := map[[2]int]bool{}
	for i, object := range h.Objects {
		if hidden[uint8(i+1)] || object.Movement != rom.MovementStay {
			continue
		}
		blocked[[2]int{int(object.X), int(object.Y)}] = true
	}
	_, _, err = world.FindPathAdjacent(g,
		int(mem.U8(tablesForROM(romData).wram.XCoord)), int(mem.U8(tablesForROM(romData).wram.YCoord)),
		int(homeX), int(homeY), blocked)
	return err == nil
}
