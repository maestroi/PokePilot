package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/maestroi/pokepilot/emu"
)

// checkpointRing is the bounded record of a run: one save state per
// objective, kept as a ring of the last keep entries.
type checkpointRing struct {
	dir       string
	keep      int
	lastState string
}

func (c *checkpointRing) write(m *emu.Emu, round int, obj Objective, k *Knowledge, coverage *coverageTracker, intent string, intentAge int, plans ...Plan) error {
	return c.writeNamed(m, round, checkpointSlug(obj), k, coverage, intent, intentAge, plans...)
}

// writeBoundary captures a safe between-objective state with the agent memory
// that describes that exact emulator state. Cooperative cancellation happens
// only at this boundary, so pause/resume can continue without rewinding to the
// checkpoint taken before the previous objective ran.
func (c *checkpointRing) writeBoundary(m *emu.Emu, round int, name string, k *Knowledge, coverage *coverageTracker, intent string, intentAge int, plans ...Plan) error {
	return c.writeNamed(m, round, slugify(name), k, coverage, intent, intentAge, plans...)
}

func (c *checkpointRing) writeNamed(m *emu.Emu, round int, name string, k *Knowledge, coverage *coverageTracker, intent string, intentAge int, plans ...Plan) error {
	b, err := m.SaveState()
	if err != nil {
		return fmt.Errorf("SaveState: %w", err)
	}
	if len(b) == 0 {
		return fmt.Errorf("SaveState returned an empty state for round %d", round)
	}
	path := filepath.Join(c.dir, fmt.Sprintf("round-%03d-frame-%010d-%s.state",
		round, m.FrameCount(), name))
	if err := writeFileAtomic(path, ".state-*.tmp", b); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := writeMemoryFile(path, k, intent, intentAge, plans...); err != nil {
		return fmt.Errorf("knowledge round %d: %w", round, err)
	}
	if err := writeCoverageFile(path, coverage); err != nil {
		return fmt.Errorf("coverage round %d: %w", round, err)
	}
	if err := embedCoverageInKnowledgeFile(path, coverage); err != nil {
		return fmt.Errorf("embed coverage round %d: %w", round, err)
	}
	c.lastState = path
	return c.evict()
}

// writeFileAtomic writes data to target through a same-directory temp file and
// a rename. The caller supplies the temp-file pattern so a leftover from a
// crashed run is traceable to its writer.
//
// The rename is the point. The farm checkpoint uploader polls this directory on
// a timer and publishes whatever it reads, and os.WriteFile truncates the
// target before writing it: a reader that arrives inside that window observes
// an empty or partial save and publishes it as the run's latest checkpoint.
// That is how a complete 321 KB pre-objective checkpoint became a 0-byte resume
// state whose highest embedded frame then outranked every checkpoint the run
// produced afterwards, permanently restarting the run from a fresh cartridge.
func writeFileAtomic(target, pattern string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(target), pattern)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}

// rewriteKnowledge updates the knowledge sidecar of the checkpoint just
// written, without saving emulator state. A poisoned machine cannot be
// snapshotted; the pre-objective state file stays, and the failure has to
// be durable or the next resume selects the same objective again.
func (c *checkpointRing) rewriteKnowledge(k *Knowledge, intent string, intentAge int, plans ...Plan) error {
	if c == nil || c.lastState == "" {
		return nil
	}
	return writeMemoryFile(c.lastState, k, intent, intentAge, plans...)
}

func (c *checkpointRing) evict() error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", c.dir, err)
	}
	names := make([]string, 0, len(entries))
	stateSet := map[string]bool{}
	for _, en := range entries {
		name := en.Name()
		if strings.HasPrefix(name, "round-") && strings.HasSuffix(name, ".state") {
			names = append(names, name)
			stateSet[name] = true
		}
	}
	for _, en := range entries {
		name := en.Name()
		if !strings.HasPrefix(name, "round-") {
			continue
		}
		var base string
		switch {
		case isKnowledgeName(name):
			base = strings.TrimSuffix(strings.TrimSuffix(name, ".json"), fmt.Sprintf(".knowledge-v%d", memoryVersion))
		case isCoverageName(name):
			base = strings.TrimSuffix(name, fmt.Sprintf(".coverage-v%d.json", coverageFileVersion))
		default:
			continue
		}
		if stateSet[base+".state"] {
			continue
		}
		if err := os.Remove(filepath.Join(c.dir, name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("evict %s: %w", name, err)
		}
	}
	sort.Strings(names)
	for _, n := range names[:max(0, len(names)-c.keep)] {
		statePath := filepath.Join(c.dir, n)
		if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("evict %s: %w", n, err)
		}
		if err := os.Remove(knowledgePathForState(statePath)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("evict %s: %w", n, err)
		}
		if err := os.Remove(coveragePathForState(statePath)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("evict %s: %w", n, err)
		}
	}
	return nil
}

func checkpointSlug(o Objective) string { return slugify(o.String()) }

func slugify(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}
