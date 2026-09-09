package agent_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

// TestTrainerArrivalReplay uses the pre-objective artifact from farm run
// run-2txl3quu7z1p8juj7uehiayj5, round 119. Keep the private state outside Git;
// supply its path as POKEPILOT_TRAINER_ARRIVAL_STATE (see docs/QUALIFICATION.md).
func TestTrainerArrivalReplay(t *testing.T) {
	path := os.Getenv("POKEPILOT_TRAINER_ARRIVAL_STATE")
	romPath := os.Getenv("POKEMON_RED_ROM")
	if testing.Short() || path == "" || romPath == "" {
		t.Skip("requires private trainer-arrival state and ROM")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != "ffdd1adc8aa1bffc4ed99821ef7bd58e1a204537d7cab85c9f121fa091736c05" {
		t.Fatalf("unexpected replay state: %s", got)
	}
	for _, objective := range []bool{false, true} {
		t.Run(fmt.Sprintf("objective=%t", objective), func(t *testing.T) {
			m, err := emu.Open(romPath)
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			if err = m.LoadState(b); err != nil {
				t.Fatal(err)
			}
			dest, ok := skill.Place("cerulean gym")
			if !ok {
				t.Fatal("missing destination")
			}
			if objective {
				result, err := agent.Execute(m, m.ROM(), agent.Objective{Kind: agent.KindGoTo, Place: "cerulean gym", Flee: true})
				if err != nil || result.Outcome != agent.OutcomeCompleted {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			} else {
				result, err := skill.TravelFlee(m, m.ROM(), dest, skill.StatAwareMove(m.ROM()), 20)
				if err != nil || result.Battles != 2 || result.BlackedOut {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			}
			var mem state.Mem
			state.Snapshot(m, &mem)
			p := state.DecodePlayer(&mem)
			dx, dy := int(p.X)-int(dest.X), int(p.Y)-int(dest.Y)
			if dx < 0 {
				dx = -dx
			}
			if dy < 0 {
				dy = -dy
			}
			if p.MapID != dest.Map || dx+dy > 1 || !state.Controllable(&mem) || state.DecodeBattle(&mem) != nil {
				t.Fatalf("not a stable arrival: %+v", p)
			}
		})
	}
}
