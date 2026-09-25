package game

import (
	"errors"
	"reflect"
	"testing"
)

func testBattleState() BattleDecisionState {
	return BattleDecisionState{
		Context:  BattleContextWild,
		Active:   BattleMon{Species: "squirtle", Level: 10, HP: 20, MaxHP: 30},
		Opponent: BattleMon{Species: "geodude", Level: 9, HP: 25, MaxHP: 25},
		Moves: []BattleMoveOption{
			{Slot: 0, Move: "tackle", PP: 10},
			{Slot: 1, Move: "tail whip", PP: 0, Unusable: BattleUnusableNoPP},
			{Slot: 2, Move: "bubble", PP: 5},
		},
		Switches: []BattleSwitchOption{
			{Slot: 1, BattleMon: BattleMon{Species: "pidgey", HP: 12, MaxHP: 20}},
			{Slot: 2, BattleMon: BattleMon{Species: "rattata", MaxHP: 20}, Unusable: BattleUnusableFainted},
		},
		Items:  []BattleItemOption{{Item: "Super Potion", Target: 0, Quantity: 2}},
		CanRun: true,
	}
}

func TestBattleActionsAreDerivedFromLegalOptions(t *testing.T) {
	s := testBattleState()
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, a := range s.Actions() {
		ids = append(ids, a.ID())
	}
	want := []string{"move:0", "move:2", "switch:1", "item:super potion:0", "run"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("actions = %v, want %v", ids, want)
	}
	for _, id := range want {
		a, err := s.Legal(id)
		if err != nil || a.ID() != id {
			t.Fatalf("Legal(%q) = %v, %v", id, a, err)
		}
		parsed, err := ParseBattleAction(id)
		if err != nil || parsed.ID() != id {
			t.Fatalf("round trip %q = %v, %v", id, parsed, err)
		}
	}
}

func TestBattleLegalRejectsUndeclaredActions(t *testing.T) {
	s := testBattleState()
	for _, id := range []string{
		"move:1",              // exhausted PP
		"move:3",              // empty slot
		"switch:0",            // already active
		"switch:2",            // fainted
		"item:potion:0",       // not declared
		"item:super potion:1", // declared for another target only
		"struggle",
		"move:-1",
		"",
	} {
		if _, err := s.Legal(id); !errors.Is(err, ErrIllegalBattleAction) {
			t.Errorf("Legal(%q) err = %v, want ErrIllegalBattleAction", id, err)
		}
	}

	moveOnly := s.MoveOnly()
	for _, id := range []string{"switch:1", "item:super potion:0", "run"} {
		if _, err := moveOnly.Legal(id); !errors.Is(err, ErrIllegalBattleAction) {
			t.Errorf("move-only Legal(%q) err = %v", id, err)
		}
	}
	if _, err := moveOnly.Legal("move:2"); err != nil {
		t.Fatal(err)
	}
}

func TestBattleValidateRejectsContradictions(t *testing.T) {
	cases := map[string]func(*BattleDecisionState){
		"trainer run":        func(s *BattleDecisionState) { s.Context = BattleContextTrainer },
		"unknown context":    func(s *BattleDecisionState) { s.Context = "safari" },
		"usable without pp":  func(s *BattleDecisionState) { s.Moves[0].PP = 0 },
		"duplicate move":     func(s *BattleDecisionState) { s.Moves[2].Slot = 0 },
		"switch to active":   func(s *BattleDecisionState) { s.Switches[0].Slot = 0 },
		"legal fainted":      func(s *BattleDecisionState) { s.Switches[1].Unusable = "" },
		"duplicate switch":   func(s *BattleDecisionState) { s.Switches[1].Slot = 1 },
		"empty item":         func(s *BattleDecisionState) { s.Items[0].Quantity = 0 },
		"item unknown slot":  func(s *BattleDecisionState) { s.Items[0].Target = 5 },
		"item id with colon": func(s *BattleDecisionState) { s.Items[0].Item = "a:b" },
		"unbounded recent":   func(s *BattleDecisionState) { s.Recent = make([]string, MaxBattleRecent+1) },
		"no legal action": func(s *BattleDecisionState) {
			s.Moves, s.Switches, s.Items, s.CanRun = nil, nil, nil, false
		},
	}
	for name, mutate := range cases {
		s := testBattleState()
		s.Moves = append([]BattleMoveOption(nil), s.Moves...)
		s.Switches = append([]BattleSwitchOption(nil), s.Switches...)
		s.Items = append([]BattleItemOption(nil), s.Items...)
		mutate(&s)
		if err := s.Validate(); !errors.Is(err, ErrInvalidBattleState) {
			t.Errorf("%s: Validate err = %v, want ErrInvalidBattleState", name, err)
		}
	}
}
