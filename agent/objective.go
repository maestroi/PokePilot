package agent

import (
	"fmt"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/skill"
)

// Kind is what an objective does.
type Kind uint8

const (
	KindGoTo    Kind = iota // walk to a named place
	KindTalk                // face and talk to something at a coordinate
	KindStarter             // complete the opening story and take a starter
)

// Objective is one unit of intent a planner can choose.
type Objective struct {
	Kind  Kind
	Place string // KindGoTo: a name accepted by skill.Place
	X, Y  uint8  // KindTalk: the tile to face and talk to
	Note  string // human-readable, shown to a planner; never parsed
}

// Execute carries out one objective against the emulator. Every error it
// returns names the objective, so a failure deep in a long loop is
// attributable. An unknown Kind is an error, not a no-op.
func Execute(m *emu.Emu, romData []byte, o Objective) error {
	switch o.Kind {
	case KindGoTo:
		dest, ok := skill.Place(o.Place)
		if !ok {
			return fmt.Errorf("agent: %s: unknown place %q", o, o.Place)
		}
		if err := skill.GoTo(m, romData, dest); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindTalk:
		if err := skill.Face(m, o.X, o.Y); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		if _, err := skill.Talk(m); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	case KindStarter:
		// GetStarter is idempotent: it returns nil immediately when the
		// rival-battle event is already set, so no guard is needed here.
		if err := skill.GetStarter(m, romData, skill.StarterSquirtle, skill.StatAwareMove(romData)); err != nil {
			return fmt.Errorf("agent: %s: %w", o, err)
		}
		return nil
	}
	return fmt.Errorf("agent: unknown objective kind %d", int(o.Kind))
}

// String renders a short, plain, stable one-line description of the
// objective. It is shown to a planner, never parsed.
func (o Objective) String() string {
	switch o.Kind {
	case KindGoTo:
		return "go to " + o.Place
	case KindTalk:
		return fmt.Sprintf("talk at (%d,%d)", o.X, o.Y)
	case KindStarter:
		return "take a starter"
	}
	return fmt.Sprintf("unknown kind %d", int(o.Kind))
}
