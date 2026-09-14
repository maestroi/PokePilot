package farm

import (
	"fmt"
	"strings"
)

const PortableReproVersion = 1

// PortableReproFile identifies one file embedded in a portable GitHub-hosted
// repro bundle. The SHA-256 is checked again by pokerepro after download so a
// fixer never silently runs a truncated or mismatched checkpoint.
type PortableReproFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// PortableReproManifest is the small ROM-free contract inside repro.zip.
// Checkpoint and Knowledge are the only required game-state payloads. Goal and
// endpoint profile are optional because older finish reports did not persist
// them; pokerepro can still reproduce deterministic executor/pathing failures
// from the exact state plus paired agent knowledge.
type PortableReproManifest struct {
	Version          int               `json:"version"`
	IssueNumber      int64             `json:"issue_number,omitempty"`
	RunID            string            `json:"run_id"`
	Attempt          int               `json:"attempt"`
	ObservedRevision string            `json:"observed_revision,omitempty"`
	Fingerprint      string            `json:"fingerprint,omitempty"`
	ExternalID       string            `json:"external_id,omitempty"`
	Checkpoint       PortableReproFile `json:"checkpoint"`
	Knowledge        PortableReproFile `json:"knowledge"`
	Planner          string            `json:"planner,omitempty"`
	Goal             string            `json:"goal,omitempty"`
	LLMProfile       string            `json:"llm_profile,omitempty"`
	Objective        string            `json:"objective,omitempty"`
	Diagnostic       string            `json:"diagnostic,omitempty"`
}

func ValidatePortableReproManifest(m PortableReproManifest) error {
	if m.Version != PortableReproVersion {
		return fmt.Errorf("farm: portable repro version %d, want %d", m.Version, PortableReproVersion)
	}
	if strings.TrimSpace(m.RunID) == "" {
		return fmt.Errorf("farm: portable repro missing run_id")
	}
	if m.Attempt < 1 {
		return fmt.Errorf("farm: portable repro invalid attempt %d", m.Attempt)
	}
	if err := validatePortableReproFile("checkpoint", m.Checkpoint); err != nil {
		return err
	}
	if err := validatePortableReproFile("knowledge", m.Knowledge); err != nil {
		return err
	}
	if !strings.HasSuffix(m.Checkpoint.Name, ".state") {
		return fmt.Errorf("farm: portable repro checkpoint %q is not a state", m.Checkpoint.Name)
	}
	stem := strings.TrimSuffix(m.Checkpoint.Name, ".state")
	if !strings.HasPrefix(m.Knowledge.Name, stem+".knowledge-v") || !strings.HasSuffix(m.Knowledge.Name, ".json") {
		return fmt.Errorf("farm: portable repro knowledge %q does not pair with %q", m.Knowledge.Name, m.Checkpoint.Name)
	}
	return nil
}

func validatePortableReproFile(kind string, f PortableReproFile) error {
	name := strings.TrimSpace(f.Name)
	if name == "" || strings.ContainsAny(name, `/\\`) || name == "." || name == ".." {
		return fmt.Errorf("farm: portable repro invalid %s name %q", kind, f.Name)
	}
	if len(strings.TrimSpace(f.SHA256)) != 64 {
		return fmt.Errorf("farm: portable repro invalid %s sha256", kind)
	}
	if f.Size <= 0 {
		return fmt.Errorf("farm: portable repro invalid %s size %d", kind, f.Size)
	}
	return nil
}
