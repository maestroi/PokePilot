package skill

import (
	"errors"
	"reflect"
	"testing"

	"github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
)

// Raw Gen I bytes used by these ROM-free fixtures.
const (
	testSquirtle   uint8 = 0xB1
	testGeodude    uint8 = 0xA9
	testPidgey     uint8 = 0x24
	testRattata    uint8 = 0xA5
	testTackle     uint8 = 0x21
	testTailWhip   uint8 = 0x27
	testBubble     uint8 = 0x91
	testWaterGun   uint8 = 0x37
	statusAsleep   uint8 = 0b011
	statusPoisoned uint8 = 1 << 3
)

func testGen1Snapshot() Gen1BattleSnapshot {
	b := state.BattleState{
		Kind:          state.BattleWild,
		EnemySpecies:  testGeodude,
		EnemyHP:       25,
		EnemyMaxHP:    25,
		EnemyLevel:    9,
		ActiveSpecies: testSquirtle,
		ActiveHP:      30,
		ActiveMaxHP:   30,
		ActiveLevel:   10,
		EnemyType1:    0x05,
		EnemyType2:    0x04,
		ActiveType1:   0x15,
		ActiveType2:   0x15,
	}
	b.Moves = [4]state.Move{{ID: testTackle, PP: 30}, {ID: testTailWhip, PP: 30}, {ID: testBubble, PP: 25}, {ID: testWaterGun, PP: 20}}
	return Gen1BattleSnapshot{
		Battle: b,
		Party: state.PartyState{Count: 3, Mons: []state.Mon{
			{Species: testSquirtle, Level: 10, HP: 30, MaxHP: 30},
			{Species: testPidgey, Level: 8, HP: 10, MaxHP: 25, Status: statusPoisoned},
			{Species: testRattata, Level: 6, HP: 0, MaxHP: 22},
		}},
	}
}

func actionIDs(s game.BattleDecisionState) []string {
	var out []string
	for _, a := range s.Actions() {
		out = append(out, a.ID())
	}
	return out
}

func TestGen1BattleDecisionOrdinaryFourMoveWild(t *testing.T) {
	s, err := BuildBattleDecisionState(nil, testGen1Snapshot(), BattleDecisionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := actionIDs(s), []string{"move:0", "move:1", "move:2", "move:3", "run"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
	if s.Active.Species != "squirtle" || s.Opponent.Species != "geodude" || s.Moves[3].Move != "water gun" {
		t.Fatalf("semantic ids not projected: %+v", s)
	}
	if !reflect.DeepEqual(s.Opponent.Types, []string{"rock", "ground"}) || !reflect.DeepEqual(s.Active.Types, []string{"water"}) {
		t.Fatalf("types = %v / %v", s.Opponent.Types, s.Active.Types)
	}
}

func TestGen1BattleDecisionMovesMirrorUsable(t *testing.T) {
	snap := testGen1Snapshot()
	snap.Battle.Moves[3].PP = 0  // exhausted
	snap.Battle.Moves[0].Disabled = true // tackle disabled
	snap.Battle.Kind = state.BattleTrainer
	s, err := BuildBattleDecisionState(nil, snap, BattleDecisionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var slots []int
	for _, a := range s.Actions() {
		if a.Kind != game.BattleActionMove {
			t.Fatalf("trainer battle declared non-move action %s", a.ID())
		}
		slots = append(slots, a.Slot)
	}
	if !reflect.DeepEqual(slots, snap.Battle.Usable()) {
		t.Fatalf("move actions %v, Usable %v", slots, snap.Battle.Usable())
	}
	if s.Moves[0].Unusable != game.BattleUnusableDisabled || s.Moves[3].Unusable != game.BattleUnusableNoPP {
		t.Fatalf("unusable reasons = %+v", s.Moves)
	}
	for _, id := range []string{"move:0", "move:3", "run"} {
		if _, err := s.Legal(id); !errors.Is(err, game.ErrIllegalBattleAction) {
			t.Errorf("Legal(%q) = %v", id, err)
		}
	}
}

func TestGen1BattleDecisionSwitchesAndItems(t *testing.T) {
	snap := testGen1Snapshot()
	snap.ActiveStatus = statusAsleep
	snap.Bag = []state.BagItem{{ID: itemPotion, Quantity: 2}, {ID: itemAwakening, Quantity: 1}, {ID: itemAntidote, Quantity: 1}, {ID: itemBurnHeal, Quantity: 3}}
	s, err := BuildBattleDecisionState(nil, snap, BattleDecisionOptions{Switches: true, Items: BattleMedicineItems()})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"move:0", "move:1", "move:2", "move:3",
		"switch:1",         // the fainted rattata and the active slot are never switch actions
		"item:potion:1",    // only the damaged pidgey; squirtle is at full HP
		"item:antidote:1",  // matches pidgey's poison
		"item:awakening:0", // the live battle status, not the party copy
		"run",
	}
	if got := actionIDs(s); !reflect.DeepEqual(got, want) {
		t.Fatalf("actions = %v\nwant      %v", got, want)
	}
	if s.Active.Status != "asleep" || s.Switches[0].Status != "poisoned" || s.Switches[1].Unusable != game.BattleUnusableFainted {
		t.Fatalf("status/switch projection = %+v / %+v", s.Active, s.Switches)
	}

	// Items the caller did not permit are never declared.
	s, err = BuildBattleDecisionState(nil, snap, BattleDecisionOptions{Switches: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Items) != 0 {
		t.Fatalf("items declared without permission: %+v", s.Items)
	}
}

func TestGen1BattleDecisionNotActionable(t *testing.T) {
	for _, battleType := range []uint8{1, 2} { // Old Man demo, Safari Zone
		snap := testGen1Snapshot()
		snap.BattleType = battleType
		if _, err := BuildBattleDecisionState(nil, snap, BattleDecisionOptions{}); !errors.Is(err, ErrBattleNotActionable) {
			t.Errorf("battle type %d err = %v", battleType, err)
		}
	}
	// A PP-dead trainer turn with nothing else permitted has no typed decision;
	// Battle's own STRUGGLE/switch recovery owns it.
	snap := testGen1Snapshot()
	snap.Battle.Kind = state.BattleTrainer
	for i := range snap.Battle.Moves {
		snap.Battle.Moves[i].PP = 0
	}
	if _, err := BuildBattleDecisionState(nil, snap, BattleDecisionOptions{}); !errors.Is(err, game.ErrInvalidBattleState) {
		t.Fatalf("PP-dead turn err = %v", err)
	}
}

func TestMoveOnlyBattleDecisionStateServesMovePolicy(t *testing.T) {
	b := testGen1Snapshot().Battle
	b.Moves[0].PP = 0
	s, err := MoveOnlyBattleDecisionState(nil, b)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := actionIDs(s), []string{"move:1", "move:2", "move:3"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("move-only actions = %v, want %v", got, want)
	}
}
