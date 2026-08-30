package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// memoryVersion is written into every knowledge file and required back on
// read. Bump it whenever the serialised shape changes: a reader that does
// not understand the bytes must start clean, never half-load — the
// fixture-cache rule (validate on write AND on read, version the filename),
// learned the hard way. The version is in the file NAME as well, so a wrong
// or old reader can tell at a glance which files it cannot claim.
const memoryVersion = 2

// knowledgeFileName is the ONLY way to name a knowledge file: from the base
// name of the checkpoint state it was written beside. There is no function
// that takes a bare knowledge path, so a knowledge file cannot be loaded
// next to a save state it was not captured with — the pairing the whole
// safety argument rests on is structural, not conventional.
func knowledgeFileName(stateBase string) string {
	return fmt.Sprintf("%s.knowledge-v%d.json", stateBase, memoryVersion)
}

// knowledgePathForState names the knowledge file paired with one state path.
func knowledgePathForState(statePath string) string {
	base := strings.TrimSuffix(filepath.Base(statePath), ".state")
	return filepath.Join(filepath.Dir(statePath), knowledgeFileName(base))
}

// isKnowledgeName reports whether a checkpoint directory entry is a
// knowledge file of any version (eviction must find them all).
func isKnowledgeName(name string) bool {
	return strings.HasSuffix(name, ".json") && strings.Contains(name, "knowledge-v")
}

// memoryFile is the serialised form of a run's knowledge, captured beside
// the checkpoint save state it describes. It holds only what the game has
// SHOWN the player: maps stood on, place names spoken or visited, objectives
// completed, objects talked to, the raw requirement-shaped sentences the
// game has said — plus the planner's carried intent and its age (S9-4). Deliberately NOT here: Adjacency, which is route geometry
// rebuilt from the ROM by world.BuildGraph every run (it is large, and a
// stale copy would outlive a map fix); and Observation, History and the
// offered list, which are re-derived from the game state in one round.
type memoryFile struct {
	Version   int         `json:"version"`
	Visited   []uint8     `json:"visited"`
	Places    []string    `json:"places"`
	Completed    []string    `json:"completed"`
	Talked       []talkedKey `json:"talked"`
	Requirements []string    `json:"requirements,omitempty"`
	Intent    string      `json:"intent,omitempty"`
	IntentAge int         `json:"intent_age,omitempty"`
}

// talkedKey is one "talked to this object" record: map-local coordinates are
// not globally unique, so the map id travels with them (see Knowledge.Talked).
type talkedKey struct {
	Map uint8 `json:"map"`
	X   uint8 `json:"x"`
	Y   uint8 `json:"y"`
}

// encodeMemoryFile renders the knowledge captured at this moment — plus the
// intent sentence and its age the run is carrying — as the versioned on-disk
// form. The struct IS the validation on write: every field has a fixed
// type, the version is stamped, and there is no path by which an unknown
// shape reaches the file.
func encodeMemoryFile(k *Knowledge, intent string, intentAge int) ([]byte, error) {
	mem := memoryFile{Version: memoryVersion, Intent: intent, IntentAge: intentAge}
	for id := range k.Visited {
		mem.Visited = append(mem.Visited, id)
	}
	for name := range k.Places {
		mem.Places = append(mem.Places, name)
	}
	for s := range k.Completed {
		mem.Completed = append(mem.Completed, s)
	}
	for mapID, tiles := range k.Talked {
		for tile := range tiles {
			mem.Talked = append(mem.Talked, talkedKey{Map: mapID, X: tile[0], Y: tile[1]})
		}
	}
	mem.Requirements = append(mem.Requirements, k.Requirements...)
	return json.Marshal(mem)
}

// writeMemoryFile writes the knowledge captured at this moment beside the
// checkpoint state at statePath. It is called from the same function that
// wrote the state (checkpointRing.write), so the two cannot drift out of
// step: a knowledge file always describes the save state beside it, and
// never a game state that has not seen what it claims to know. The write is
// atomic (temp file + rename) so a crash mid-write cannot leave a truncated
// file beside a valid state for a reader to half-trust.
func writeMemoryFile(statePath string, k *Knowledge, intent string, intentAge int) error {
	data, err := encodeMemoryFile(k, intent, intentAge)
	if err != nil {
		return fmt.Errorf("encode knowledge: %w", err)
	}
	target := knowledgePathForState(statePath)
	tmp, err := os.CreateTemp(filepath.Dir(statePath), ".knowledge-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp knowledge file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename has succeeded
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write knowledge: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close knowledge: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("rename knowledge into place: %w", err)
	}
	return nil
}

// ResumedMemory is what a checkpoint pair hands back to a run that starts
// from it: the knowledge captured when the state was taken, and the intent
// sentence (and its age) the run was carrying at that moment.
type ResumedMemory struct {
	Knowledge *Knowledge
	Intent    string
	IntentAge int
}

// LoadCheckpointMemory reads the knowledge file paired with the checkpoint
// state at statePath and folds it into a fresh Knowledge over the given
// route geometry.
//
// The pairing is structural: the knowledge path is derived from the state
// path (knowledgeFileName), so this function cannot be pointed at a
// knowledge file whose save state it was not written beside. Restoring
// knowledge onto a DIFFERENT game state would let the run "know" places
// that save has never been; the pairing is the whole safety argument for
// restoring knowledge at all.
//
// Every failure mode — missing file, wrong version, truncated bytes, pure
// garbage — returns an EMPTY ResumedMemory (a clean start) and writes one
// log line. It never returns a partial load and never panics: a knowledge
// file that claims the run knows things this save state has not seen is
// worse than no file at all.
func LoadCheckpointMemory(statePath string, adjacency map[uint8][]uint8, log io.Writer) ResumedMemory {
	empty := ResumedMemory{Knowledge: NewKnowledge(adjacency)}
	base := filepath.Base(statePath)
	if !strings.HasSuffix(base, ".state") {
		logMemory(log, "%s is not a checkpoint .state file; starting with empty knowledge", statePath)
		return empty
	}
	data, err := os.ReadFile(knowledgePathForState(statePath))
	if err != nil {
		logMemory(log, "no readable knowledge file beside %s (%v); starting with empty knowledge", statePath, err)
		return empty
	}
	var mem memoryFile
	if err := json.Unmarshal(data, &mem); err != nil {
		logMemory(log, "knowledge file beside %s is unreadable (%v); starting with empty knowledge", statePath, err)
		return empty
	}
	if mem.Version != memoryVersion {
		logMemory(log, "knowledge file beside %s is version %d, want %d; starting with empty knowledge",
			statePath, mem.Version, memoryVersion)
		return empty
	}
	if len(mem.Intent) > IntentCap {
		// An over-cap intent cannot have come from a WithArgs that passed
		// validation; the file is not what we wrote.
		logMemory(log, "knowledge file beside %s carries an intent of %d bytes, over the cap of %d; starting with empty knowledge",
			statePath, len(mem.Intent), IntentCap)
		return empty
	}
	k := NewKnowledge(adjacency)
	k.restore(mem)
	return ResumedMemory{Knowledge: k, Intent: mem.Intent, IntentAge: mem.IntentAge}
}

// logMemory writes the one line a failed knowledge load leaves behind. Nil
// log means no logging, like every other log writer in this package.
func logMemory(log io.Writer, format string, args ...any) {
	if log != nil {
		fmt.Fprintf(log, "agent: memory: "+format+"\n", args...)
	}
}
