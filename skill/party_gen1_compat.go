package skill

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// SetLead remains Gen-I compatibility for now. Battle switching is portable,
// but overworld party reordering still depends on Red's concrete party and
// controllability layout and will move with the field/party preparation slice.
func SetLead(m *emu.Emu, slot int) error {
	var mem state.Mem
	state.Snapshot(m, &mem)
	party := state.DecodeParty(&mem)
	if slot < 0 || slot >= int(party.Count) {
		return fmt.Errorf("skill: SetLead: slot %d out of range for a party of %d", slot, party.Count)
	}
	if slot == 0 {
		return nil
	}
	if !state.Controllable(&mem) {
		return fmt.Errorf("skill: SetLead: player not controllable on map %#04x", m.Peek8(sym.CurMap))
	}
	return PromoteToLead(m, slot)
}
