package agent

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Planner interface {
	Next(obs Observation, offered []Objective) (Objective, error)
}

var ErrDone = errors.New("agent: nothing left to do")

const IntentCap = 200

var ErrIntentTooLong = errors.New("agent: intent exceeds the byte cap")

type ScriptedPlanner struct {
	objs []Objective
	next int
}

func NewScriptedPlanner(objs ...Objective) *ScriptedPlanner {
	return &ScriptedPlanner{objs: objs}
}

func (p *ScriptedPlanner) Next(obs Observation, offered []Objective) (Objective, error) {
	if p.next >= len(p.objs) {
		return Objective{}, ErrDone
	}
	o := p.objs[p.next]
	p.next++
	return o, nil
}

func Chosen(offered []Objective, s string) (Objective, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Objective{}, fmt.Errorf("agent: no objective chosen; offered: %s", offeredList(offered))
	}
	if i, err := strconv.Atoi(s); err == nil {
		if i < 1 || i > len(offered) {
			return Objective{}, fmt.Errorf("agent: index %d out of range; offered: %s", i, offeredList(offered))
		}
		return offered[i-1], nil
	}
	want := strings.ToLower(s)
	for _, o := range offered {
		if strings.ToLower(o.String()) == want {
			return o, nil
		}
	}
	return Objective{}, fmt.Errorf("agent: %q is not one of the offered objectives; offered: %s", s, offeredList(offered))
}

type ReplyArgs struct {
	Level    *int
	Species  string
	Item     string
	Quantity *int
	Flee     *bool
	Intent   string
}

// WithArgs validates and attaches model-supplied semantic arguments. Resolving
// those names to a concrete game's numeric encoding is adapter work, not model
// reply parsing.
func WithArgs(o Objective, a ReplyArgs) (Objective, error) {
	var species SpeciesID
	var item ItemID
	if a.Intent != "" && len(a.Intent) > IntentCap {
		return o, fmt.Errorf("%w: %d bytes exceeds the %d-byte cap", ErrIntentTooLong, len(a.Intent), IntentCap)
	}
	if a.Level != nil {
		if o.Kind != KindTrain {
			return o, fmt.Errorf("agent: level argument %d does not apply to %s", *a.Level, o)
		}
		if *a.Level < 1 || *a.Level > 100 {
			return o, fmt.Errorf("agent: level %d out of range 1..100 for %s", *a.Level, o)
		}
	}
	if a.Species != "" {
		if o.Kind != KindCatch {
			return o, fmt.Errorf("agent: species argument %q does not apply to %s", a.Species, o)
		}
		id, ok := semanticSpecies(a.Species)
		if !ok {
			return o, fmt.Errorf("agent: unknown species %q for %s", a.Species, o)
		}
		species = id
	}
	if a.Item != "" {
		if o.Kind != KindBuy {
			return o, fmt.Errorf("agent: item argument %q does not apply to %s", a.Item, o)
		}
		id, ok := semanticItem(a.Item)
		if !ok {
			return o, fmt.Errorf("agent: unknown item %q for %s", a.Item, o)
		}
		item = id
	}
	if a.Quantity != nil {
		if o.Kind != KindBuy {
			return o, fmt.Errorf("agent: quantity argument %d does not apply to %s", *a.Quantity, o)
		}
		if *a.Quantity < 1 || *a.Quantity > 99 {
			return o, fmt.Errorf("agent: quantity %d out of range 1..99 for %s", *a.Quantity, o)
		}
	}
	if a.Flee != nil && o.Kind != KindGoTo && !(o.Kind == KindHeal && o.Place != "") {
		return o, fmt.Errorf("agent: flee argument %v does not apply to %s", *a.Flee, o)
	}
	if a.Level != nil {
		o.Level = uint8(*a.Level)
	}
	if a.Species != "" {
		o.Species = species
	}
	if a.Item != "" {
		o.Item = item
	}
	if a.Quantity != nil {
		o.Qty = *a.Quantity
	}
	if a.Flee != nil {
		o.Flee = *a.Flee
	}
	if a.Intent != "" {
		o.Intent = a.Intent
	}
	return o, nil
}

func offeredList(offered []Objective) string {
	if len(offered) == 0 {
		return "nothing was offered"
	}
	parts := make([]string, len(offered))
	for i, o := range offered {
		parts[i] = fmt.Sprintf("%d: %s", i+1, o)
	}
	return strings.Join(parts, ", ")
}
