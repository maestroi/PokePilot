package main

import (
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// dialogueTracer records on-screen dialogue into the emulator's trace.
//
// It lives here rather than in emu because decoding text needs red/state, and
// emu must stay ignorant of Pokemon. emu calls this on the goroutine stepping
// the emulator, so reading memory here does not race.
type dialogueTracer struct {
	mem *state.Mem

	last    string // last line emitted
	pending string // last line seen, not yet stable
	stable  bool
}

func newDialogueTracer() *dialogueTracer {
	return &dialogueTracer{mem: new(state.Mem)}
}

// sample decodes what the text box currently says and emits it once the text
// stops growing. Gen 1 types dialogue out a character at a time, so a naive
// read emits a prefix of the line on every sample.
//
// ponytail: settles on two matching samples, so a pause after punctuation can
// still emit a growing prefix. The final entry is always the complete line.
// Emit on box-close instead if the extra entries get noisy.
func (d *dialogueTracer) sample(m *emu.Emu) {
	state.Snapshot(m, d.mem)
	if state.DecodeDialogue(d.mem) == nil {
		// Box closed: forget the line so saying it again later re-emits.
		d.last, d.pending, d.stable = "", "", false
		return
	}

	text := state.ScreenText(d.mem)
	switch {
	case text == "":
		return
	case text != d.pending:
		d.pending, d.stable = text, false
	case !d.stable:
		d.stable = true
		if text != d.last {
			d.last = text
			m.TraceNote("dialogue", text)
		}
	}
}
