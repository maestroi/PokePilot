package story

import (
	"errors"
	"testing"

	yellowprofile "github.com/maestroi/pokepilot/yellow/profile"
)

func bedroom() yellowprofile.OpeningFacts {
	return yellowprofile.OpeningFacts{Map: 0x26, X: 3, Y: 6, Controllable: true}
}

func TestPhaseForFollowsTheDecompScriptOrder(t *testing.T) {
	lab := yellowprofile.OaksLabMap
	for _, tc := range []struct {
		name  string
		facts yellowprofile.OpeningFacts
		want  OpeningPhase
	}{
		{"fresh bedroom", bedroom(), OpeningWalkToGate},
		{"pallet town, gate not fired", yellowprofile.OpeningFacts{Map: yellowprofile.PalletTownMap, X: 5, Y: 6, Controllable: true}, OpeningWalkToGate},
		{"oak stops the player", yellowprofile.OpeningFacts{Map: yellowprofile.PalletTownMap, Y: 0, OakAppeared: true}, OpeningAdvanceScript},
		{"oak's pikachu catch", yellowprofile.OpeningFacts{Map: yellowprofile.PalletTownMap, OakAppeared: true, InBattle: true}, OpeningAdvanceScript},
		{"lab entered, speech pending", yellowprofile.OpeningFacts{Map: lab, OakAppeared: true, FollowedOak: true, Controllable: true}, OpeningAdvanceScript},
		{"oak asks to choose", yellowprofile.OpeningFacts{Map: lab, X: 5, Y: 3, OakAppeared: true, FollowedOak: true, OakAskedToChoose: true, Controllable: true}, OpeningTakeBall},
		{"rival snatches the ball", yellowprofile.OpeningFacts{Map: lab, X: 7, Y: 4, OakAppeared: true, FollowedOak: true, OakAskedToChoose: true}, OpeningAdvanceScript},
		{"pikachu received", yellowprofile.OpeningFacts{Map: lab, X: 5, Y: 3, PartyCount: 1, OakAppeared: true, FollowedOak: true, OakAskedToChoose: true, GotStarter: true, Controllable: true}, OpeningWalkToRival},
		{"rival battle", yellowprofile.OpeningFacts{Map: lab, PartyCount: 1, InBattle: true, OakAppeared: true, FollowedOak: true, OakAskedToChoose: true, GotStarter: true}, OpeningFightRival},
		{"rival leaving", yellowprofile.OpeningFacts{Map: lab, PartyCount: 1, OakAppeared: true, FollowedOak: true, OakAskedToChoose: true, GotStarter: true, BattledRival: true}, OpeningAdvanceScript},
		{"opening done", yellowprofile.OpeningFacts{Map: lab, PartyCount: 1, OakAppeared: true, FollowedOak: true, OakAskedToChoose: true, GotStarter: true, BattledRival: true, Controllable: true}, OpeningDone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := PhaseFor(tc.facts)
			if err != nil {
				t.Fatalf("PhaseFor: %v", err)
			}
			if got != tc.want {
				t.Fatalf("PhaseFor = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestPhaseForRefusesLabPhasesOutsideTheLab(t *testing.T) {
	for _, facts := range []yellowprofile.OpeningFacts{
		{Map: yellowprofile.PalletTownMap, OakAppeared: true, FollowedOak: true, OakAskedToChoose: true, Controllable: true},
		{Map: yellowprofile.PalletTownMap, PartyCount: 1, OakAppeared: true, FollowedOak: true, OakAskedToChoose: true, GotStarter: true, Controllable: true},
	} {
		if _, err := PhaseFor(facts); !errors.Is(err, ErrOpeningUnexpectedState) {
			t.Fatalf("PhaseFor(%+v) err = %v, want ErrOpeningUnexpectedState", facts, err)
		}
	}
}

// scriptedGame models the observable effect of each opening action the way
// the decomp's scripts produce it. Each action advances the story exactly as
// the real game would when the mechanics work.
type scriptedGame struct {
	facts yellowprofile.OpeningFacts
	calls []OpeningPhase
	// overrides replace an action's effect, keyed by phase, to model failures.
	overrides map[OpeningPhase]func(*scriptedGame) error
}

func (g *scriptedGame) Facts() yellowprofile.OpeningFacts { return g.facts }

func (g *scriptedGame) do(phase OpeningPhase, effect func(f *yellowprofile.OpeningFacts)) error {
	g.calls = append(g.calls, phase)
	if override := g.overrides[phase]; override != nil {
		return override(g)
	}
	effect(&g.facts)
	return nil
}

func (g *scriptedGame) WalkToGate() error {
	return g.do(OpeningWalkToGate, func(f *yellowprofile.OpeningFacts) {
		f.Map, f.X, f.Y = yellowprofile.PalletTownMap, 10, 0
		f.OakAppeared, f.Controllable = true, false
	})
}

func (g *scriptedGame) AdvanceScript() error {
	return g.do(OpeningAdvanceScript, func(f *yellowprofile.OpeningFacts) {
		switch {
		case f.BattledRival:
			f.Controllable = true
		case f.GotStarter:
			// The challenge text ends in the battle.
			f.InBattle = true
		case f.OakAskedToChoose:
			// Rival takes the Eevee ball, Oak hands over Pikachu.
			f.GotStarter, f.PartyCount, f.X, f.Y, f.Controllable = true, 1, 5, 3, true
		default:
			// Oak's Pikachu catch, the walk into the lab, the choose speech.
			f.Map, f.X, f.Y = yellowprofile.OaksLabMap, 5, 3
			f.FollowedOak, f.OakAskedToChoose, f.InBattle, f.Controllable = true, true, false, true
		}
	})
}

func (g *scriptedGame) TakeBall() error {
	return g.do(OpeningTakeBall, func(f *yellowprofile.OpeningFacts) {
		f.X, f.Y, f.Controllable = 7, 4, false
	})
}

func (g *scriptedGame) WalkToRival() error {
	return g.do(OpeningWalkToRival, func(f *yellowprofile.OpeningFacts) {
		f.X, f.Y, f.Controllable = 5, 6, false
	})
}

func (g *scriptedGame) FightRival() error {
	return g.do(OpeningFightRival, func(f *yellowprofile.OpeningFacts) {
		f.InBattle, f.BattledRival = false, true
	})
}

func TestRunOpeningPlaysTheWholeOpeningFromTheBedroom(t *testing.T) {
	g := &scriptedGame{facts: bedroom()}
	if err := RunOpening(g, MilestoneLabRivalResolved); err != nil {
		t.Fatalf("RunOpening: %v (calls %v)", err, g.calls)
	}
	want := []OpeningPhase{
		OpeningWalkToGate, OpeningAdvanceScript, OpeningTakeBall, OpeningAdvanceScript,
		OpeningWalkToRival, OpeningAdvanceScript, OpeningFightRival, OpeningAdvanceScript,
	}
	if len(g.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", g.calls, want)
	}
	for i := range want {
		if g.calls[i] != want[i] {
			t.Fatalf("calls = %v, want %v", g.calls, want)
		}
	}
	if !MilestoneLabRivalResolved.Reached(g.facts) {
		t.Fatalf("final facts %+v do not satisfy the milestone", g.facts)
	}
}

func TestRunOpeningStopsAtTheRequestedMilestone(t *testing.T) {
	g := &scriptedGame{facts: bedroom()}
	if err := RunOpening(g, MilestoneStarterReceived); err != nil {
		t.Fatalf("RunOpening: %v", err)
	}
	if g.facts.BattledRival || g.facts.InBattle {
		t.Fatalf("ran past the starter milestone: %+v (calls %v)", g.facts, g.calls)
	}
	for _, call := range g.calls {
		if call == OpeningWalkToRival || call == OpeningFightRival {
			t.Fatalf("starter milestone walked into the rival: calls %v", g.calls)
		}
	}
}

func TestRunOpeningResumesMidOpening(t *testing.T) {
	g := &scriptedGame{facts: yellowprofile.OpeningFacts{
		Map: yellowprofile.OaksLabMap, X: 5, Y: 3, PartyCount: 1, Controllable: true,
		OakAppeared: true, FollowedOak: true, OakAskedToChoose: true, GotStarter: true,
	}}
	if err := RunOpening(g, MilestoneLabRivalResolved); err != nil {
		t.Fatalf("RunOpening: %v", err)
	}
	if g.calls[0] != OpeningWalkToRival {
		t.Fatalf("resumed with %s, want %s (calls %v)", g.calls[0], OpeningWalkToRival, g.calls)
	}
	for _, call := range g.calls {
		if call == OpeningWalkToGate || call == OpeningTakeBall {
			t.Fatalf("replayed an earlier opening step: calls %v", g.calls)
		}
	}
}

func TestRunOpeningIsANoOpWhenAlreadyDone(t *testing.T) {
	g := &scriptedGame{facts: yellowprofile.OpeningFacts{
		Map: 0x00, PartyCount: 1, Controllable: true,
		OakAppeared: true, FollowedOak: true, OakAskedToChoose: true, GotStarter: true, BattledRival: true,
	}}
	if err := RunOpening(g, MilestoneLabRivalResolved); err != nil {
		t.Fatalf("RunOpening: %v", err)
	}
	if len(g.calls) != 0 {
		t.Fatalf("done opening issued actions: %v", g.calls)
	}
}

func TestRunOpeningToleratesAnInterruptThatMovesTheStory(t *testing.T) {
	interrupted := errors.New("text box interrupted movement")
	g := &scriptedGame{facts: bedroom()}
	g.overrides = map[OpeningPhase]func(*scriptedGame) error{
		OpeningWalkToGate: func(g *scriptedGame) error {
			g.facts.Map, g.facts.X, g.facts.Y = yellowprofile.PalletTownMap, 10, 0
			g.facts.OakAppeared, g.facts.Controllable = true, false
			return interrupted
		},
	}
	if err := RunOpening(g, MilestoneLabRivalResolved); err != nil {
		t.Fatalf("an interrupt that fired the gate was treated as failure: %v", err)
	}
}

func TestRunOpeningFailsWhenAnActionErrorsWithoutProgress(t *testing.T) {
	blocked := errors.New("walk blocked")
	g := &scriptedGame{facts: bedroom()}
	g.overrides = map[OpeningPhase]func(*scriptedGame) error{
		OpeningWalkToGate: func(*scriptedGame) error { return blocked },
	}
	err := RunOpening(g, MilestoneLabRivalResolved)
	if !errors.Is(err, blocked) {
		t.Fatalf("err = %v, want the action's error", err)
	}
	if len(g.calls) != 1 {
		t.Fatalf("retried a failed action with no state change: %v", g.calls)
	}
}

func TestRunOpeningReportsStallAsTypedError(t *testing.T) {
	g := &scriptedGame{facts: bedroom()}
	g.overrides = map[OpeningPhase]func(*scriptedGame) error{
		OpeningWalkToGate: func(*scriptedGame) error { return nil },
	}
	err := RunOpening(g, MilestoneLabRivalResolved)
	if !errors.Is(err, ErrOpeningStalled) {
		t.Fatalf("err = %v, want ErrOpeningStalled", err)
	}
	if len(g.calls) != 2 {
		t.Fatalf("stall detection took %d actions, want 2", len(g.calls))
	}
}

func TestMilestoneRequiresAStableBoundary(t *testing.T) {
	midScript := yellowprofile.OpeningFacts{PartyCount: 1, GotStarter: true, BattledRival: true}
	if MilestoneStarterReceived.Reached(midScript) || MilestoneLabRivalResolved.Reached(midScript) {
		t.Fatal("milestone reached while the script still held control")
	}
	inBattle := yellowprofile.OpeningFacts{PartyCount: 1, GotStarter: true, Controllable: true, InBattle: true}
	if MilestoneStarterReceived.Reached(inBattle) {
		t.Fatal("milestone reached inside a battle")
	}
}
