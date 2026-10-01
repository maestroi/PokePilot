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

// A running script that still owns movement is a transition or cutscene, not a
// dialogue. Gold's bedroom stair warp sets exactly this pair for the frames
// between stepping onto the warp tile and the map actually changing; decoding
// it as InDialogue made native edge crossing return ErrDialogueInterrupted
// before the warp could land, which surfaced as
// "route interrupted by unowned script on map 0x1807 at (7,0)".
func TestDecodeOverworldScriptInMotionIsNotDialogue(t *testing.T) {
	reader := readyGSReader()
	reader[sym.ScriptRunning] = 1
	reader[sym.PlayerStepFlags] = gen2PlayerStepContinue

	state := NewGold().DecodeOverworld(reader)
	if state.InDialogue {
		t.Fatalf("scripted movement decoded as dialogue: %+v", state)
	}
	if state.Controllable || state.MovementIdle {
		t.Fatalf("scripted movement must still own the machine: %+v", state)
	}
	if !gsScriptActive(reader) {
		t.Fatal("script activity must stay observable independently of dialogue")
	}
	// The Gen-II opening driver still has to see the transition as a script it
	// may wait out, so the broad fact must survive the narrower dialogue.
	if facts := NewGold().DecodeOpening(reader); !facts.ScriptActive {
		t.Fatalf("opening facts lost the transition script: %+v", facts)
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

// A map's identity becomes current while a field script still owns the
// overworld; the block buffer keeps the previous map's bytes until the script
// returns control. Gold reaches MAPSTATUS_HANDLE before that rebuild, so the
// phase alone cannot be the readiness signal: a consumer that treated it as one
// read unwritten blocks (0xff) as collision and aborted routing on a map that
// had not been loaded.
func TestDecodeLiveTopologyWithholdsBlocksWhileScriptOwnsOverworld(t *testing.T) {
	reader := readyGSReader()
	reader[sym.ScriptMode] = 1
	reader[sym.ScriptRunning] = 0xff

	state, err := NewGold().DecodeLiveTopology(reader)
	if err != nil {
		t.Fatalf("DecodeLiveTopology: %v", err)
	}
	if state.MapShellPhase != game.MapShellSettled {
		t.Fatalf("MapShellPhase = %d, want the handle phase: the phase must not be mistaken for readiness", state.MapShellPhase)
	}
	if state.BlocksSettled {
		t.Fatal("BlocksSettled = true while a field script still owns the overworld")
	}

	reader[sym.ScriptMode] = gen2ScriptOff
	reader[sym.ScriptRunning] = 0
	state, err = NewGold().DecodeLiveTopology(reader)
	if err != nil {
		t.Fatalf("DecodeLiveTopology: %v", err)
	}
	if !state.BlocksSettled {
		t.Fatal("BlocksSettled = false for a controllable overworld")
	}
}

// A map with no dimensions has no blocks to decode yet, whatever the script
// state says.
func TestDecodeLiveTopologyWithholdsBlocksWithoutDimensions(t *testing.T) {
	reader := readyGSReader()
	reader[sym.MapWidth] = 0
	reader[sym.MapHeight] = 0

	state, err := NewGold().DecodeLiveTopology(reader)
	if err != nil {
		t.Fatalf("DecodeLiveTopology: %v", err)
	}
	if state.BlocksSettled {
		t.Fatal("BlocksSettled = true for a map with no dimensions")
	}
}

// Elm's phone call takes the overworld as a step finishes, freezing the step
// flags at CONTINUE|STOP under a dialogue frame. That is a dialogue, not a
// walk; the same flags with no text box (or no script) are still a step.
func TestDecodeOverworldFrozenStepUnderTextboxIsDialogue(t *testing.T) {
	frame := func(r fakeGSReader) {
		r[sym.TileMap+12*20+0], r[sym.TileMap+12*20+19] = 0x79, 0x7b
		r[sym.TileMap+17*20+0], r[sym.TileMap+17*20+19] = 0x7d, 0x7e
	}
	tests := []struct {
		name         string
		script, text bool
		wantDialogue bool
	}{
		{"script and text box", true, true, true},
		{"script without text box", true, false, false},
		{"text box without script", false, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := readyGSReader()
			r[sym.PlayerStepFlags] = gen2PlayerStepContinue | gen2PlayerStepStop
			if tc.script {
				r[sym.ScriptRunning] = 0xff
				r[sym.ScriptMode] = 1
			}
			if tc.text {
				frame(r)
			}
			if got := NewGold().DecodeOverworld(r).InDialogue; got != tc.wantDialogue {
				t.Fatalf("InDialogue = %v, want %v", got, tc.wantDialogue)
			}
		})
	}
}
