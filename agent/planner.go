package agent

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Planner chooses the next objective. Offered lists the objectives that
// are valid right now; a planner MUST return one of them.
type Planner interface {
	Next(obs Observation, offered []Objective) (Objective, error)
}

// ErrDone is returned by a planner with nothing left to do.
var ErrDone = errors.New("agent: nothing left to do")

// ScriptedPlanner walks a fixed list of objectives in order. It is the
// default: deterministic, free, and the reason tests never call a model.
type ScriptedPlanner struct {
	objs []Objective
	next int
}

// NewScriptedPlanner returns a ScriptedPlanner that yields objs in order.
func NewScriptedPlanner(objs ...Objective) *ScriptedPlanner {
	return &ScriptedPlanner{objs: objs}
}

// Next returns the next unconsumed objective, and ErrDone once the list is
// exhausted. It deliberately ignores obs and offered: the list IS the plan,
// so there is nothing to consult or check against.
func (p *ScriptedPlanner) Next(obs Observation, offered []Objective) (Objective, error) {
	if p.next >= len(p.objs) {
		return Objective{}, ErrDone
	}
	o := p.objs[p.next]
	p.next++
	return o, nil
}

// Chosen returns the offered objective matching s, or an error naming
// what was offered. It never guesses: an unmatched string is an error,
// never a nearest match.
//
// s matches an objective's String() form, trimmed and compared
// case-insensitively. A bare 1-based index ("3") is accepted too, because
// small models reliably emit an index and unreliably echo a sentence.
// There is no fuzzy, prefix, or edit-distance matching: a planner that
// picks the wrong objective because it nearly spelled one is worse than a
// planner that errors.
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

// offeredList renders the offered objectives as a numbered one-liner for
// error messages, so a planner's reply can be read against them.
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
