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
	// The offered menu renders each line as "sentence  (note)" — done/failed
	// counts, "(unvisited adjacent map)", etc — and a model told to "copy the
	// sentence after the number" sometimes copies the annotation too, reading
	// it as part of the sentence rather than menu decoration. MEASURED
	// 2026-09-08: both real rejections seen were exactly this shape, one
	// copying the menu's own note verbatim, one inventing its own gloss.
	// Stripping one trailing parenthetical and retrying is safe: it only
	// succeeds when the stripped form exactly matches an offered objective,
	// so a genuine objective that ends mid-sentence (none do; "talk at
	// (X,Y)" has no trailing annotation to strip past the coordinate) is
	// never affected.
	if stripped, ok := strings.CutSuffix(strings.TrimRight(s, " "), ")"); ok {
		if paren := strings.LastIndex(stripped, "("); paren > 0 {
			base := strings.ToLower(strings.TrimSpace(stripped[:paren]))
			for _, o := range offered {
				if strings.ToLower(o.String()) == base {
					return o, nil
				}
			}
		}
	}
	// A rejection re-ask quotes the offered menu back as "N: sentence"
	// lines (offeredList), and a model told to answer with the sentence
	// sometimes echoes that numeral prefix back too, reading it as part of
	// its own answer rather than menu decoration. MEASURED 2026-09-09
	// (run-16om5de3gn5u21q5ehht4ga6db, round 60, ask 3 of 3): the model
	// replied exactly "5: go to pewter pokemon center", which is offered
	// item 5 verbatim once the prefix is stripped — and the run died on
	// this alone with no strategist retries left. Stripping one leading
	// "digits:" and retrying is safe: it only succeeds when the remainder
	// exactly matches an offered objective, so it never invents a match.
	if i := strings.Index(s, ":"); i > 0 {
		if _, err := strconv.Atoi(strings.TrimSpace(s[:i])); err == nil {
			base := strings.ToLower(strings.TrimSpace(s[i+1:]))
			for _, o := range offered {
				if strings.ToLower(o.String()) == base {
					return o, nil
				}
			}
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
