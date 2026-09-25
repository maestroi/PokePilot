package skill

import (
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// battleScreenHas is retained only for legacy Gen-I party helpers that have
// not yet moved to PartyMenuDecoder. The main Battle controller no longer
// reads Red tile text directly.
func battleScreenHas(m *emu.Emu, marker string) bool {
	var mem state.Mem
	state.Snapshot(m, &mem)
	return strings.Contains(state.ScreenText(&mem), marker)
}
