package farm

import (
	"encoding/json"
	"strings"
	"sync"
)

// playStyleByRun extends the long-lived Spec wire without changing the legacy
// struct layout used throughout the wall/runner code. The JSON methods below
// round-trip play_style keyed by run_id; old payloads simply have no entry.
//
// This is intentionally wire policy, not planner policy: farm cannot import
// agent. The runner normalizes the value before constructing planner behavior.
var playStyleByRun sync.Map // map[string]string

// RememberPlayStyle records the wire value for a run. Empty style removes the
// extension, preserving the historical no-field encoding for legacy specs.
func RememberPlayStyle(runID, style string) {
	runID = strings.TrimSpace(runID)
	style = strings.ToLower(strings.TrimSpace(style))
	if runID == "" {
		return
	}
	if style == "" {
		playStyleByRun.Delete(runID)
		return
	}
	playStyleByRun.Store(runID, style)
}

// PlayStyleForRun returns the selected wire profile, or empty for a legacy
// spec. Empty deliberately means "use the agent's backwards-compatible
// default", not Adventure.
func PlayStyleForRun(runID string) string {
	if v, ok := playStyleByRun.Load(strings.TrimSpace(runID)); ok {
		return v.(string)
	}
	return ""
}

func PlayStyleForSpec(s Spec) string { return PlayStyleForRun(s.RunID) }

// CopyPlayStyle is used by orchestrators that spawn a logical successor run.
// It is harmless when the parent is legacy/empty.
func CopyPlayStyle(fromRunID, toRunID string) {
	if style := PlayStyleForRun(fromRunID); style != "" {
		RememberPlayStyle(toRunID, style)
	}
}

// MarshalJSON adds play_style to Spec without forcing every existing Spec
// literal in the repository to grow a field immediately.
func (s Spec) MarshalJSON() ([]byte, error) {
	type plain Spec
	return json.Marshal(struct {
		plain
		PlayStyle string `json:"play_style,omitempty"`
	}{
		plain:     plain(s),
		PlayStyle: PlayStyleForRun(s.RunID),
	})
}

// UnmarshalJSON accepts both old specs and the extended play_style wire and
// remembers the extension by run_id for subsequent lease/recording use.
func (s *Spec) UnmarshalJSON(data []byte) error {
	type plain Spec
	var in struct {
		plain
		PlayStyle string `json:"play_style,omitempty"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	*s = Spec(in.plain)
	RememberPlayStyle(s.RunID, in.PlayStyle)
	return nil
}
