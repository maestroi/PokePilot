package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

func encodeGSName(name string) []byte {
	out := make([]byte, sym.PlayerNameLen)
	for i := range out {
		out[i] = gsNameTerminator
	}
	for i := 0; i < len(name) && i < len(out); i++ {
		c := name[i]
		switch {
		case c >= 'A' && c <= 'Z':
			out[i] = 0x80 + (c - 'A')
		case c >= 'a' && c <= 'z':
			out[i] = 0xa0 + (c - 'a')
		}
	}
	if len(name) < len(out) {
		out[len(name)] = gsNameTerminator
	}
	return out
}

func putGSBytes(mem fakeGSReader, addr uint16, raw []byte) {
	for i, b := range raw {
		mem[addr+uint16(i)] = b
	}
}

func TestDecodeGSName(t *testing.T) {
	raw := encodeGSName("GOLD")
	if got := decodeGSName(raw); got != "GOLD" {
		t.Fatalf("decodeGSName = %q, want GOLD", got)
	}
}

func TestGoldBootStateSteersToFirstBuiltInName(t *testing.T) {
	mem := fakeGSReader{
		sym.TwoDMenuNumRows: 5,
		sym.MenuCursorY:     1,
	}
	state := NewGold().DecodeBootState(mem)
	if !state.NameMenu || state.NextInput != game.BootInputDown {
		t.Fatalf("row 1 state = %+v, want name menu + down", state)
	}

	mem[sym.MenuCursorY] = 2
	state = NewGold().DecodeBootState(mem)
	if state.NextInput != game.BootInputConfirm || state.SelectedPresetName != "GOLD" {
		t.Fatalf("row 2 state = %+v, want confirm GOLD", state)
	}
	if len(state.PresetNames) != 5 || state.PresetNames[1] != "GOLD" {
		t.Fatalf("presets = %v, want Gold name menu", state.PresetNames)
	}
}

func TestSilverBootStateUsesSilverPreset(t *testing.T) {
	mem := fakeGSReader{
		sym.TwoDMenuNumRows: 5,
		sym.MenuCursorY:     2,
	}
	state := NewSilver().DecodeBootState(mem)
	if state.SelectedPresetName != "SILVER" || state.NextInput != game.BootInputConfirm {
		t.Fatalf("state = %+v, want SILVER confirm", state)
	}
}

func TestGSBootStateSelectsNewGameWhenSaveExists(t *testing.T) {
	mem := fakeGSReader{
		sym.SaveFileExists:  1,
		sym.TwoDMenuNumRows: 3,
		sym.MenuCursorY:     1,
	}
	state := NewGold().DecodeBootState(mem)
	if state.NextInput != game.BootInputDown {
		t.Fatalf("state = %+v, want down from CONTINUE to NEW GAME", state)
	}
	mem[sym.MenuCursorY] = 2
	state = NewGold().DecodeBootState(mem)
	if state.NextInput != game.BootInputConfirm {
		t.Fatalf("state = %+v, want confirm NEW GAME", state)
	}
}

func TestGSBootStateUsesConfirmForClockAndOak(t *testing.T) {
	state := NewGold().DecodeBootState(fakeGSReader{})
	if state.NextInput != game.BootInputConfirm || state.Ready {
		t.Fatalf("state = %+v, want ordinary confirm", state)
	}
}

func TestGSBootStateWaitsForBedroomAndThenReportsReady(t *testing.T) {
	mem := fakeGSReader{
		sym.MapGroup:  24,
		sym.MapNumber: 7,
		sym.XCoord:    3,
		sym.YCoord:    6,
		sym.MapHeight: 4,
		sym.MapWidth:  4,
	}
	putGSBytes(mem, sym.PlayerName, encodeGSName("GOLD"))

	loading := NewGold().DecodeBootState(mem)
	if loading.Ready || loading.NextInput != game.BootInputWait {
		t.Fatalf("loading = %+v, want bedroom wait", loading)
	}

	mem[sym.MapStatus] = gen2MapStatusHandle
	mem[sym.MapEventStatus] = gen2MapEventsOn
	ready := NewGold().DecodeBootState(mem)
	if !ready.Ready || !ready.Controllable || ready.NextInput != game.BootInputWait {
		t.Fatalf("ready = %+v, want controllable bedroom", ready)
	}
	if ready.NativeMapID != 0x1807 || ready.PlayerName != "GOLD" {
		t.Fatalf("ready identity = map %#04x name %q", ready.NativeMapID, ready.PlayerName)
	}
}
