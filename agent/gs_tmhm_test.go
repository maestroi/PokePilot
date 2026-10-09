package agent

import (
	"os"
	"testing"
)

// Gen-II move ids (pokegold constants/move_constants.asm).
const (
	gsMoveCut          = 0x0f
	gsMoveHeadbutt     = 0x1d
	gsMoveRazorLeaf    = 0x4b
	gsMovePoisonPowder = 0x4d
	gsMoveReflect      = 0x73
	gsMoveMudSlap      = 0xbd
	gsMoveSynthesis    = 0xeb
	gsSpeciesBayleef   = 153
)

func goldROMForTest(t *testing.T) []byte {
	t.Helper()
	path := os.Getenv("POKEMON_GOLD_ROM")
	if path == "" {
		t.Skip("POKEMON_GOLD_ROM not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// run-1auv5rq62ou1i16n25pxcc2izv: Bayleef carried one damaging move into a
// rival whose whole team resists Grass, with Mud-Slap in the pack.
func TestGSDecideMachineAddsCoverageOverRedundantStatus(t *testing.T) {
	romData := goldROMForTest(t)
	species := []uint8{gsSpeciesBayleef}
	eligible := []bool{true}
	moves := [][4]uint16{{gsMoveRazorLeaf, gsMoveReflect, gsMovePoisonPowder, gsMoveSynthesis}}

	const tmMudSlap, tmHeadbutt, hmCut = 31, 2, 51
	d, ok := gsDecideMachine(romData, tmMudSlap, species, eligible, moves, -1)
	if !ok || d.Move != gsMoveMudSlap || d.Slot != 0 || d.After <= d.Before {
		t.Fatalf("Mud-Slap decision = %+v ok=%v, want a gain for Bayleef", d, ok)
	}
	if d.Replace == 0 {
		t.Fatalf("Mud-Slap replaced Razor Leaf, the only damaging move: %+v", d)
	}
	if _, ok := gsDecideMachine(romData, tmHeadbutt, species, eligible, moves, -1); !ok {
		t.Fatal("Headbutt not offered over a third status move")
	}
	if d, ok := gsDecideMachine(romData, hmCut, species, eligible, moves, -1); ok {
		t.Fatalf("HM offered into a full move list: %+v", d)
	}

	known := [][4]uint16{{gsMoveRazorLeaf, gsMoveMudSlap, 0, 0}}
	if d, ok := gsDecideMachine(romData, tmMudSlap, species, eligible, known, -1); ok {
		t.Fatalf("machine offered for an already-known move: %+v", d)
	}
	if d, ok := gsDecideMachine(romData, hmCut, species, eligible, known, -1); !ok || d.Replace != -1 || d.Move != gsMoveCut {
		t.Fatalf("Cut into an empty slot = %+v ok=%v", d, ok)
	}
	if _, ok := gsDecideMachine(romData, tmMudSlap, species, []bool{false}, moves, -1); ok {
		t.Fatal("machine offered to an egg")
	}
}
