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

// IntentCap is the byte cap on a planner's intent sentence. It is stated in
// the reply schema's description AND enforced here as a typed rejection:
// an intent that costs more than this many prompt bytes on every subsequent
// round is a paragraph, not a sentence, and one that is silently truncated
// means something different from what the model said.
const IntentCap = 200

// ErrIntentTooLong is the typed rejection for an intent over IntentCap.
var ErrIntentTooLong = errors.New("agent: intent exceeds the byte cap")

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

// ReplyArgs is what the model may attach to its choice: the arguments of
// the objective it picked. Pointer fields mean "the model said nothing",
// which is legal — an objective already carries its own argument, and a
// bare {"choice": N} selects it unchanged. A present value must be valid
// for the chosen kind or the round stops with a typed error.
type ReplyArgs struct {
	Level    *int
	Species  string
	Item     string
	Quantity *int
	Flee     *bool // KindGoTo / KindHeal-with-Place: run wild encounters instead of fighting them
	// Intent applies to EVERY kind (unlike level/species/item): every
	// objective can be in service of something. It is the model's own
	// sentence, carried by Run onto the next round's Observation.
	Intent string
}

// WithArgs applies a model-supplied argument to an offered objective and
// returns the concrete objective to execute. Every value is checked against
// its stated range before it is accepted: a level outside 1..100, an
// unknown species, a quantity outside 1..99, or an unknown item is an error
// that stops the round. Nothing is clamped and nothing is best-matched,
// and an argument that does not apply to the chosen kind (a level on a
// "go to" objective) is an error too — a reply that says one thing and
// means another is exactly the silent wrong choice this exists to prevent.
func WithArgs(o Objective, a ReplyArgs) (Objective, error) {
	// Validate EVERY argument before any of them lands: a rejected reply
	// must not leave a half-updated objective behind.
	var species, item uint8
	if a.Intent != "" {
		// Rejected, not truncated: a cut sentence reads as valid while
		// saying something the model never said — the same class of bug as
		// the truncated-reply case ErrNotFinished exists to catch.
		if len(a.Intent) > IntentCap {
			return o, fmt.Errorf("%w: %d bytes exceeds the %d-byte cap", ErrIntentTooLong, len(a.Intent), IntentCap)
		}
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
		id, ok := SpeciesByName(a.Species)
		if !ok {
			return o, fmt.Errorf("agent: unknown species %q for %s", a.Species, o)
		}
		species = id
	}
	if a.Item != "" {
		if o.Kind != KindBuy {
			return o, fmt.Errorf("agent: item argument %q does not apply to %s", a.Item, o)
		}
		id, ok := ItemByName(a.Item)
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
	if a.Flee != nil {
		// Flee only applies to the kinds that travel: KindGoTo, and
		// KindHeal when it carries a Place (a heal in place walks nowhere,
		// so there is nothing to flee on). A flee on anything else is the
		// same silent-wrong-choice error as the other misplaced arguments.
		if o.Kind != KindGoTo && !(o.Kind == KindHeal && o.Place != "") {
			return o, fmt.Errorf("agent: flee argument %v does not apply to %s", *a.Flee, o)
		}
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
