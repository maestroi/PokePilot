package agent

import "fmt"

// providerBlockRequirements projects current-round structured provider block
// evidence into the planner's existing Requirements view. The structured
// ObjectiveBlockEvidence remains the runtime source of truth; this projection is
// deliberately ephemeral and is rebuilt every round rather than persisted in
// Knowledge, so material world-state changes naturally remove stale blockers.
func providerBlockRequirements(blocked []ObjectiveBlockEvidence) []Requirement {
	if len(blocked) == 0 {
		return nil
	}
	ordered := append([]ObjectiveBlockEvidence(nil), blocked...)
	sortBlockEvidence(ordered)

	out := make([]Requirement, 0, min(len(ordered), requirementCap))
	seen := make(map[string]bool, len(ordered))
	for _, block := range ordered {
		text := fmt.Sprintf("%s blocked: %s", block.Family, block.Reason)
		if block.Requirement != "" {
			text += "; requires " + block.Requirement
		}
		place := string(block.Place)
		key := text + "\x00" + place
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Requirement{Text: text, Place: place, Times: 1})
		if len(out) == requirementCap {
			break
		}
	}
	return out
}
