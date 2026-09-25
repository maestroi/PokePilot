package farm

import (
	"encoding/json"
	"strings"
)

// RunGoal is the run's task statement as it appears on the Spec wire. It is a
// distinct type rather than a plain string because the contract has three
// states, not two:
//
//   - not provided: the run has no explicit goal and a play style may supply
//     its own terminal default during normalization;
//   - provided and empty: Free play. An operator explicitly asked for no
//     goal, and no default may be substituted;
//   - provided and non-empty: the explicit task statement.
//
// Collapsing the first two states would silently turn a Free play run into a
// default-goal run on the next lease or clone. Encoding that distinction in
// the type keeps Spec a normal, self-describing struct: no Spec-level custom
// JSON is needed, because `omitzero` together with IsZero omits only the
// genuinely unset state.
type RunGoal struct {
	value *string
}

// IsZero reports whether the goal was never provided. encoding/json uses it
// (via `omitzero`) to omit the field entirely for unset goals.
func (g RunGoal) IsZero() bool { return g.value == nil }

// Provided reports whether the goal was explicitly supplied, including an
// explicit empty Free play goal.
func (g RunGoal) Provided() bool { return g.value != nil }

// String returns the goal statement, or empty when none was provided. Callers
// that need to distinguish Free play from "absent" must use Provided. It
// satisfies fmt.Stringer for logging and diagnostics.
func (g RunGoal) String() string {
	if g.value == nil {
		return ""
	}
	return *g.value
}

// SetGoal replaces the goal with an explicit value, which also marks it as
// provided.
func (g *RunGoal) SetGoal(goal string) {
	value := goal
	g.value = &value
}

// GoalFrom returns a provided goal with the given value. It is the
// constructor for callers outside this package, which cannot set the
// unexported value directly.
func GoalFrom(goal string) RunGoal {
	return RunGoal{value: &goal}
}

// UnmarshalJSON distinguishes an absent goal field (nil, not provided) from
// an explicit empty string (provided Free play goal).
func (g *RunGoal) UnmarshalJSON(data []byte) error {
	var value *string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	g.value = value
	return nil
}

// MarshalJSON encodes a provided goal, including an explicit empty string.
// The unset state never reaches this method because Spec tags the field
// `omitzero`.
func (g RunGoal) MarshalJSON() ([]byte, error) {
	if g.value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*g.value)
}

// NormalizedGoal returns the trimmed goal statement. Normalization belongs to
// the run wiring, not to the wire type: the stored value stays verbatim.
func (g RunGoal) NormalizedGoal() string { return strings.TrimSpace(g.String()) }

// HeldGoal returns the goal a holder should record for a Spec, plus whether the
// operator supplied one. It is the projection the wall's Tile uses so a single
// lease/derive cycle keeps the goal's tri-state intact:
//
//   - a provided goal is carried verbatim, including an explicit empty Free
//     play goal;
//   - an unset goal stays unset (provided == false), so a later lease does not
//     turn it into an explicit empty goal.
//
// An unset goal whose play style carries a default is deliberately left unset
// here: the runner applies that default when it starts the run, and if it does,
// the wall records the resolved value on the next derive.
func HeldGoal(spec Spec) (string, bool) {
	if spec.Goal.Provided() {
		return spec.Goal.String(), true
	}
	return "", false
}
