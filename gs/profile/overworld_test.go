package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/gs/sym"
)

type fakeGSReader map[uint16]byte

func (r fakeGSReader) Peek8(addr uint16) byte { return r[addr] }

func (r fakeGSReader) PeekInto(addr uint16, dst []byte) {
	for i := range dst {
		dst[i] = r[addr+uint16(i)]
	}
}

func readyGSReader() fakeGSReader {
	return fakeGSReader{
		sym.MapGroup:       24,
		sym.MapNumber:      7,
		sym.XCoord:         4,
		sym.YCoord:         2,
		sym.MapWidth:       4,
		sym.MapHeight:      3,
		sym.MapStatus:      gen2MapStatusHandle,
		sym.MapEventStatus: gen2MapEventsOn,
		sym.ScriptMode:     gen2ScriptOff,
		sym.ScriptRunning:  0,
		sym.ScriptFlags:    0,
		sym.BattleMode:     0,
	}
}

func TestDecodeOverworldReadyGoldState(t *testing.T) {
	reader := readyGSReader()
	state := NewGold().DecodeOverworld(reader)
	if state.NativeMapID != 0x1807 {
		t.Fatalf("NativeMapID = %#04x, want 0x1807", state.NativeMapID)
	}
	if state.X != 4 || state.Y != 2 {
		t.Fatalf("position = (%d,%d), want (4,2)", state.X, state.Y)
	}
	if !state.Controllable || !state.MovementIdle || state.InBattle || state.InDialogue {
		t.Fatalf("unexpected ready state: %+v", state)
	}
}

func TestDecodeOverworldRejectsScriptBattleAndMovement(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(fakeGSReader)
		check func(game.OverworldState) bool
	}{
		{
			name:  "script",
			edit:  func(r fakeGSReader) { r[sym.ScriptRunning] = 1 },
			check: func(s game.OverworldState) bool { return !s.Controllable && s.InDialogue },
		},
		{
			name:  "battle",
			edit:  func(r fakeGSReader) { r[sym.BattleMode] = 1 },
			check: func(s game.OverworldState) bool { return !s.Controllable && s.InBattle },
		},
		{
			name:  "movement",
			edit:  func(r fakeGSReader) { r[sym.PlayerStepFlags] = gen2PlayerStepContinue },
			check: func(s game.OverworldState) bool { return s.Controllable && !s.MovementIdle },
		},
		{
			name:  "events-off",
			edit:  func(r fakeGSReader) { r[sym.MapEventStatus] = 1 },
			check: func(s game.OverworldState) bool { return !s.Controllable },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := readyGSReader()
			test.edit(reader)
			state := NewGold().DecodeOverworld(reader)
			if !test.check(state) {
				t.Fatalf("unexpected state: %+v", state)
			}
		})
	}
}

func TestDecodeLiveTopologyExtractsPaddedBlocksAndObjects(t *testing.T) {
	reader := readyGSReader()
	reader[sym.MapWidth] = 2
	reader[sym.MapHeight] = 2
	stride := 2 + 2*gen2LiveMapBorderBlocks
	first := gen2LiveMapBorderBlocks*stride + gen2LiveMapBorderBlocks
	reader[sym.OverworldMap+uint16(first)] = 0x11
	reader[sym.OverworldMap+uint16(first+1)] = 0x12
	reader[sym.OverworldMap+uint16(first+stride)] = 0x21
	reader[sym.OverworldMap+uint16(first+stride+1)] = 0x22
	reader[sym.PlayerState] = 4 // PLAYER_SURF

	object := sym.ObjectStructs + sym.ObjectStructLen
	reader[object] = 1
	reader[object+1] = 3
	reader[object+0x10] = 9
	reader[object+0x11] = 11

	state, err := NewGold().DecodeLiveTopology(reader)
	if err != nil {
		t.Fatalf("DecodeLiveTopology: %v", err)
	}
	want := []byte{0x11, 0x12, 0x21, 0x22}
	if len(state.Blocks) != len(want) {
		t.Fatalf("blocks = %v, want %v", state.Blocks, want)
	}
	for i := range want {
		if state.Blocks[i] != want[i] {
			t.Fatalf("blocks = %v, want %v", state.Blocks, want)
		}
	}
	if state.Traversal != game.TraversalWater {
		t.Fatalf("Traversal = %v, want water", state.Traversal)
	}
	if len(state.LiveObjects) != 1 {
		t.Fatalf("LiveObjects = %+v, want one object", state.LiveObjects)
	}
	got := state.LiveObjects[0]
	if got.Slot != 3 || got.X != 5 || got.Y != 7 {
		t.Fatalf("live object = %+v, want slot 3 at (5,7)", got)
	}
}

func TestDecodeGen2LiveMapBlocksRejectsOversizedMap(t *testing.T) {
	reader := readyGSReader()
	if _, err := decodeGen2LiveMapBlocks(reader, 255, 255); err == nil {
		t.Fatal("oversized live map unexpectedly decoded")
	}
}
