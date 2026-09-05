package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/red/rom"
)

// The base item table predates generic machine handling and named only the
// story HMs it needed. Extend that same argument vocabulary at init time so
// Objective.String/Validate can render and validate every TM/HM without a
// second item-name system.
func init() {
	for i := 0; i < rom.NumHMs; i++ {
		id := rom.HM01Item + uint8(i)
		name := fmt.Sprintf("hm%02d", i+1)
		itemTable[name] = id
		itemByID[id] = name
	}
	for i := 0; i < rom.NumTMs; i++ {
		id := rom.TM01Item + uint8(i)
		name := fmt.Sprintf("tm%02d", i+1)
		itemTable[name] = id
		itemByID[id] = name
	}
}
