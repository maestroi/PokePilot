package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

const (
	forensicRAMVersion = 2
	defaultRAMKeep     = 32
	// Capture filename prefixes. Each is its own eviction ring, and each
	// sorts lexically in frame order because the frame is zero-padded.
	failurePrefix = "failure-frame-"
	stallPrefix   = "stall-frame-"
	// RAMForensicsDirEnv enables failure-time forensic captures. It is an
	// environment variable instead of an agent objective or planner option:
	// evidence collection is an operator concern and must never become a
	// choice the model can make about its own run.
	RAMForensicsDirEnv  = "POKEPILOT_RAM_DIR"
	RAMForensicsKeepEnv = "POKEPILOT_RAM_KEEP"
)

// forensicRAMMeta is the human-readable sidecar for a raw 64 KiB RAM capture
// and checked emulator state. The .ram and .state files are the source of
// truth; this metadata makes a failure searchable without first restoring it.
type forensicRAMMeta struct {
	Version        int          `json:"version"`
	Kind           string       `json:"kind"`
	Frame          uint64       `json:"frame"`
	Cycle          uint64       `json:"cycle"`
	Objective      string       `json:"objective"`
	Error          string       `json:"error"`
	ROMSHA256      string       `json:"rom_sha256"`
	StateHash      string       `json:"state_hash,omitempty"`
	StateHashError string       `json:"state_hash_error,omitempty"`
	Opcode         uint8        `json:"opcode"`
	CPU            emu.CPUState `json:"cpu"`
	PPU            emu.PPUState `json:"ppu"`
	Map            uint8        `json:"map"`
	X              uint8        `json:"x"`
	Y              uint8        `json:"y"`
	Controllable   bool         `json:"controllable"`
	InBattle       bool         `json:"in_battle"`
	MenuCurrent    int          `json:"menu_current"`
	MenuMax        int          `json:"menu_max"`
}

// captureObjectiveFailure preserves the exact RAM and emulator state in which
// Execute is returning a gameplay error, before agent.Run's between-round
// recovery is allowed to close dialogue or otherwise settle the game. This is
// the useful boundary for reverse-engineering menu and map-transition failures.
//
// Capture is disabled unless POKEPILOT_RAM_DIR is non-empty. A capture error
// is deliberately returned separately and must never replace the objective's
// real gameplay error.
func captureObjectiveFailure(m *emu.Emu, obj Objective, cause error) error {
	return captureRAM(m, failurePrefix, "objective_failure", obj.String(), cause.Error(), checkpointSlug(obj))
}

// CaptureStall preserves RAM when the run is stalled but NOTHING has failed:
// every objective succeeded and the world did not move. MEASURED 2026-09-02:
// eight rounds of "go to pewter city -> done" alternating with the gym, no
// error at any point, so captureObjectiveFailure recorded nothing — and the
// fact that settled it (the badge was already held) was sitting in RAM the
// whole time. Failure captures and stall captures cover disjoint pathologies.
//
// Called on the EDGE of the replan signal, not every stalled round: one
// capture per stall episode. Like every capture here it is best-effort and
// operator-gated by POKEPILOT_RAM_DIR.
func CaptureStall(m *emu.Emu, intent, reason string) error {
	slug := slugify(strings.TrimSpace(intent))
	if slug == "" {
		slug = "no-intent" // a stall with no carried intent is still evidence
	}
	return captureRAM(m, stallPrefix, "planner_stall", intent, reason, slug)
}

func captureRAM(m *emu.Emu, prefix, kind, objective, cause, slug string) error {
	dir := strings.TrimSpace(os.Getenv(RAMForensicsDirEnv))
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ram forensics mkdir %s: %w", dir, err)
	}

	var mem state.Mem
	frame, err := m.SnapshotMemory(mem[:])
	if err != nil {
		return fmt.Errorf("ram forensics snapshot: %w", err)
	}
	gs := state.Decode(&mem)
	base, err := uniqueFailureBase(dir, fmt.Sprintf("%s%010d-%s", prefix, frame, slug))
	if err != nil {
		return err
	}
	ramPath := filepath.Join(dir, base+".ram")
	statePath := filepath.Join(dir, base+".state")
	metaPath := filepath.Join(dir, base+".json")
	if err := os.WriteFile(ramPath, mem[:], 0o644); err != nil {
		return fmt.Errorf("ram forensics write %s: %w", ramPath, err)
	}

	checkedState, err := m.SaveStateChecked()
	if err != nil {
		_ = os.Remove(ramPath)
		return fmt.Errorf("ram forensics checked state: %w", err)
	}
	if err := os.WriteFile(statePath, checkedState, 0o644); err != nil {
		_ = os.Remove(ramPath)
		return fmt.Errorf("ram forensics write %s: %w", statePath, err)
	}

	cpu := m.CPUState()
	ppu := m.PPUState()
	stateHash, hashErr := m.StateHashHex()
	meta := forensicRAMMeta{
		Version:      forensicRAMVersion,
		Kind:         kind,
		Frame:        frame,
		Cycle:        m.Cycle(),
		Objective:    objective,
		Error:        cause,
		ROMSHA256:    m.ROMSHA256Hex(),
		StateHash:    stateHash,
		Opcode:       m.Peek8(cpu.PC),
		CPU:          cpu,
		PPU:          ppu,
		Map:          gs.Player.MapID,
		X:            gs.Player.X,
		Y:            gs.Player.Y,
		Controllable: state.Controllable(&mem),
		InBattle:     gs.Battle != nil,
		MenuCurrent:  gs.Menu.Current,
		MenuMax:      gs.Menu.Max,
	}
	if hashErr != nil {
		meta.StateHashError = hashErr.Error()
	}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		_ = os.Remove(ramPath)
		_ = os.Remove(statePath)
		return fmt.Errorf("ram forensics metadata: %w", err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(metaPath, b, 0o644); err != nil {
		_ = os.Remove(ramPath)
		_ = os.Remove(statePath)
		return fmt.Errorf("ram forensics write %s: %w", metaPath, err)
	}
	// Per-prefix ring: a burst of objective failures must not evict the
	// stall evidence, which is rarer and harder to reproduce.
	return evictRAMCaptures(dir, prefix, ramKeep())
}

func ramKeep() int {
	s := strings.TrimSpace(os.Getenv(RAMForensicsKeepEnv))
	if s == "" {
		return defaultRAMKeep
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultRAMKeep
	}
	return n
}

// uniqueFailureBase avoids overwriting evidence when an objective can fail
// twice without advancing a frame (for example, an already-open stuck menu).
func uniqueFailureBase(dir, base string) (string, error) {
	for i := 0; ; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%02d", base, i+1)
		}
		_, err := os.Stat(filepath.Join(dir, candidate+".ram"))
		switch {
		case err == nil:
			continue
		case os.IsNotExist(err):
			return candidate, nil
		default:
			return "", fmt.Errorf("ram forensics stat %s: %w", candidate, err)
		}
	}
}

// evictRAMCaptures keeps the latest keep forensic bundles. Filenames start
// with a zero-padded frame, so lexical order is capture order apart from
// same-frame suffixes, which sort in creation order.
func evictRAMCaptures(dir, prefix string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("ram forensics read %s: %w", dir, err)
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".ram") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names[:max(0, len(names)-keep)] {
		base := strings.TrimSuffix(name, ".ram")
		for _, ext := range []string{".ram", ".state", ".json"} {
			if err := os.Remove(filepath.Join(dir, base+ext)); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("ram forensics evict %s%s: %w", base, ext, err)
			}
		}
	}
	return nil
}
