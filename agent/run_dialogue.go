package agent

import (
	"strings"
	"sync"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// dialogueTape records what the game says and every map traversed while an
// objective is executing. Post-transaction observations cannot see text boxes
// or intermediate maps that opened and closed inside the transaction.
type dialogueTape struct {
	mu      sync.Mutex
	lines   []string
	last    string
	pending string
	stable  bool
	maps    map[uint8]bool
}

func (d *dialogueTape) sample(m *emu.Emu) {
	var mem state.Mem
	state.Snapshot(m, &mem)
	text := ""
	if ds := state.DecodeDialogue(&mem); ds != nil {
		text = ds.Text
	}
	if d.observeText(text) {
		m.TraceNote("dialogue", text)
	}
	d.noteMap(mem.U8(sym.CurMap))
}

func (d *dialogueTape) noteMap(id uint8) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.maps == nil {
		d.maps = map[uint8]bool{}
	}
	d.maps[id] = true
}

// observeText keeps settled dialogue utterances rather than each transient
// typed prefix or screen page.
func (d *dialogueTape) observeText(text string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch {
	case text == "":
		d.last, d.pending, d.stable = "", "", false
		return false
	case text != d.pending:
		d.pending, d.stable = text, false
		return false
	case !d.stable:
		d.stable = true
		if text == d.last {
			return false
		}
		switch {
		case d.last == "" || len(d.lines) == 0:
			d.lines = append(d.lines, text)
		case strings.HasPrefix(text, d.last):
			d.lines[len(d.lines)-1] = text
		default:
			d.lines[len(d.lines)-1] += " " + text
		}
		d.last = text
		if len(d.lines) > dialogueCap {
			d.lines = d.lines[len(d.lines)-dialogueCap:]
		}
		return true
	}
	return false
}

// seenMaps returns map ids sampled since the last call and clears the tape's
// transient copy; Knowledge is the durable owner.
func (d *dialogueTape) seenMaps() []uint8 {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]uint8, 0, len(d.maps))
	for id := range d.maps {
		out = append(out, id)
	}
	d.maps = nil
	return out
}

func (d *dialogueTape) recent() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, len(d.lines))
	copy(out, d.lines)
	return out
}

func appendHistory(h []RoundRecord, r RoundRecord) []RoundRecord {
	out := make([]RoundRecord, 0, len(h)+1)
	if len(h) >= historyCap {
		out = append(out, h[len(h)-historyCap+1:]...)
	} else {
		out = append(out, h...)
	}
	return append(out, r)
}
