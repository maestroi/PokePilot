package skill

import (
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// TestYellowFollowerLive pins the Pikachu follower model against real
// post-rival RAM rather than a hand-built snapshot. The fixture is the moment
// the opening ends: the player has Pikachu, the lab sequence is done, and the
// follower is standing in the world. Two invariants live here.
//
// First, the follower really is in Yellow sprite RAM at the slot the blocker
// model names (wSpritePikachuStateData1 is struct 15; the picture id is the
// Pikachu overworld sprite). A model that names the wrong slot is inert -- it
// would pass a synthetic test that never wrote the byte -- so this reads the
// byte the cartridge produced.
//
// Second, only Yellow treats that slot as passable: CollisionCheckOnLand lets
// the player bump the Pikachu a bounded number of times and then walk through,
// so planning around it strands the player. Red has no follower; its slot 15
// is an ordinary NPC that must stay solid. The two table sets must disagree.
func TestYellowFollowerLive(t *testing.T) {
	romData, err := os.ReadFile("../roms/pokemon_yellow.gb")
	if err != nil {
		t.Skip("no yellow rom")
	}
	st, err := os.ReadFile("failure/yellow_post_rival.state")
	if err != nil {
		t.Skip("no fixture")
	}
	e, err := emu.OpenCGBBytes(romData)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err := e.LoadState(st); err != nil {
		t.Fatal(err)
	}
	var mem state.Mem
	state.Snapshot(e, &mem)

	// The follower sprite is present in Yellow sprite RAM.
	if mem.U8(0xC1F0) == 0 {
		t.Fatal("no Pikachu follower sprite at wSpritePikachuStateData1")
	}
	t.Logf("follower picture id: %#x", mem.U8(0xC1F0))

	// Yellow's table set must treat the follower as passable.
	yt := tablesForROM(romData)
	if yt.passableSpriteSlot != pikaFollowerSlot {
		t.Fatalf("yellow passableSpriteSlot = %d, want 15", yt.passableSpriteSlot)
	}
	// Red's table set must not have a passable slot (Red has no follower).
	rt := tablesForROM(nil)
	if rt.passableSpriteSlot != 0 {
		t.Fatalf("red passableSpriteSlot = %d, want 0", rt.passableSpriteSlot)
	}
}
