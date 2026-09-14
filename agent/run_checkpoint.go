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
	dir  string
	keep int
}

func (c *checkpointRing) write(m *emu.Emu, round int, obj Objective, k *Knowledge, intent string, intentAge int, plans ...Plan) error {
	b, err := m.SaveState()
	if err != nil {
		return fmt.Errorf("SaveState: %w", err)
	}
	path := filepath.Join(c.dir, fmt.Sprintf("round-%03d-frame-%010d-%s.state",
		round, m.FrameCount(), checkpointSlug(obj)))
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := writeMemoryFile(path, k, intent, intentAge, plans...); err != nil {
		return fmt.Errorf("knowledge round %d: %w", round, err)
	}
	return c.evict()
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
		if !strings.HasPrefix(name, "round-") || !isKnowledgeName(name) {
			continue
		}
		base := strings.TrimSuffix(strings.TrimSuffix(name, ".json"), fmt.Sprintf(".knowledge-v%d", memoryVersion))
		if stateSet[base+".state"] {
			continue
		}
		if err := os.Remove(filepath.Join(c.dir, name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("evict %s: %w", name, err)
		}
	}
	sort.Strings(names)
	for _, n := range names[:max(0, len(names)-c.keep)] {
		if err := os.Remove(filepath.Join(c.dir, n)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("evict %s: %w", n, err)
		}
		if err := os.Remove(knowledgePathForState(filepath.Join(c.dir, n))); err != nil && !os.IsNotExist(err) {
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
