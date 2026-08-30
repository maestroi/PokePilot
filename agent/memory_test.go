package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// memoryFixture fills a Knowledge with entries in every field, and returns
// it with the intent sentence and age a run would be carrying at that moment.
func memoryFixture(t *testing.T) (*Knowledge, string, int) {
	t.Helper()
	k := NewKnowledge(map[uint8][]uint8{0x01: {0x02}, 0x02: {0x01}})
	k.SawMap(0x01)
	k.SawMap(0x03)
	k.Places["pallet town"] = true
	k.Places["viridian city"] = true
	k.Done(Objective{Kind: KindErrand})
	k.Done(Objective{Kind: KindTalk, X: 6, Y: 3})
	k.TalkedTo(0x28, 6, 3)
	k.TalkedTo(0x36, 7, 10)
	k.SawDialogue([]string{
		"You can pass here\nonly if you have\nthe CASCADEBADGE!",
		"I'm raising #MON too!", // chatter: must NOT be harvested
	}, "ROUTE_23", 4, 57)
	return k, "earn the boulder badge", 3
}

// assertEmptyKnowledge asserts the CLEAN START: every field empty, nothing
// half-loaded. This is what the corruption cases must produce — not an
// error a caller can ignore into a partial Knowledge.
func assertEmptyKnowledge(t *testing.T, k *Knowledge) {
	t.Helper()
	if len(k.Visited) != 0 {
		t.Errorf("Visited = %v, want empty", k.Visited)
	}
	if len(k.Places) != 0 {
		t.Errorf("Places = %v, want empty", k.Places)
	}
	if len(k.Completed) != 0 {
		t.Errorf("Completed = %v, want empty", k.Completed)
	}
	if len(k.Talked) != 0 {
		t.Errorf("Talked = %v, want empty", k.Talked)
	}
	if len(k.Requirements) != 0 {
		t.Errorf("Requirements = %v, want empty", k.Requirements)
	}
}

// TestMemoryRoundTrip: a Knowledge with entries in every field survives
// write-then-read intact, including the intent and its age. Adjacency must
// come from the argument (rebuilt from the ROM by the caller), never from
// the file.
func TestMemoryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "round-002-frame-0000012345-talk-at-6-3.state")
	if err := os.WriteFile(statePath, []byte("state-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	want, intent, age := memoryFixture(t)
	if err := writeMemoryFile(statePath, want, intent, age); err != nil {
		t.Fatalf("writeMemoryFile: %v", err)
	}

	var log bytes.Buffer
	got := LoadCheckpointMemory(statePath, map[uint8][]uint8{0x09: {0x0a}}, &log)

	for _, id := range []uint8{0x01, 0x03} {
		if !got.Knowledge.Visited[id] {
			t.Errorf("Visited missing %02x", id)
		}
	}
	if len(got.Knowledge.Visited) != 2 {
		t.Errorf("Visited = %v, want exactly 0x01 and 0x03", got.Knowledge.Visited)
	}
	for _, name := range []string{"pallet town", "viridian city"} {
		if !got.Knowledge.Places[name] {
			t.Errorf("Places missing %q", name)
		}
	}
	for _, s := range []string{Objective{Kind: KindErrand}.String(), Objective{Kind: KindTalk, X: 6, Y: 3}.String()} {
		if !got.Knowledge.Completed[s] {
			t.Errorf("Completed missing %q", s)
		}
	}
	if len(got.Knowledge.Talked[0x28]) != 1 || !got.Knowledge.Talked[0x28][[2]uint8{6, 3}] {
		t.Errorf("Talked[0x28] = %v, want (6,3)", got.Knowledge.Talked[0x28])
	}
	if len(got.Knowledge.Talked[0x36]) != 1 || !got.Knowledge.Talked[0x36][[2]uint8{7, 10}] {
		t.Errorf("Talked[0x36] = %v, want (7,10)", got.Knowledge.Talked[0x36])
	}
	if got.Intent != intent || got.IntentAge != age {
		t.Errorf("Intent/Age = (%q, %d), want (%q, %d)", got.Intent, got.IntentAge, intent, age)
	}
	// The harvested wall survives the round-trip verbatim; the chatter that
	// sat beside it in the same dialogue does not.
	if len(got.Knowledge.Requirements) != 1 {
		t.Fatalf("Requirements = %v, want the one harvested line", got.Knowledge.Requirements)
	}
	wall := got.Knowledge.Requirements[0]
	if wall.Text != "You can pass here\nonly if you have\nthe CASCADEBADGE!" {
		t.Errorf("Requirement text = %q, want the harvested line verbatim", wall.Text)
	}
	// Where it was heard and how often survive too: a resumed run that
	// forgets it has already hit this wall walks into it again.
	if wall.Place != "ROUTE_23" || wall.X != 4 || wall.Y != 57 || wall.Times != 1 {
		t.Errorf("Requirement = %+v, want it located at ROUTE_23 (4,57), heard once", wall)
	}
	// Adjacency is route geometry: it comes from the argument, not the file.
	if len(got.Knowledge.Adjacency) != 1 || len(got.Knowledge.Adjacency[0x09]) != 1 || got.Knowledge.Adjacency[0x09][0] != 0x0a {
		t.Errorf("Adjacency = %v, want the caller's geometry, not the file's", got.Knowledge.Adjacency)
	}
	if log.Len() != 0 {
		t.Errorf("clean load logged %q, want silence", log.String())
	}
}

// TestMemoryCorruptedFiles: a wrong-version file, a truncated file and a
// file of garbage each produce a CLEAN EMPTY START plus a log line — never
// a partial load, never a panic.
func TestMemoryCorruptedFiles(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "round-001-frame-0000000001-go-to-pewter-city.state")
	if err := os.WriteFile(statePath, []byte("state-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSuffix(filepath.Base(statePath), ".state")
	knPath := filepath.Join(dir, knowledgeFileName(base))

	good, intent, age := memoryFixture(t)
	goodBytes, err := encodeMemoryFile(good, intent, age)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		data []byte
	}{
		{"wrong version", []byte(`{"version": 999, "visited": [1, 3], "places": ["pallet town"], "completed": ["take a starter"], "talked": [{"map": 40, "x": 6, "y": 3}]}`)},
		{"truncated", goodBytes[:len(goodBytes)/2]},
		{"garbage", []byte("this is not json at all \x00\x01")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(knPath, tc.data, 0o644); err != nil {
				t.Fatal(err)
			}
			var log bytes.Buffer
			got := LoadCheckpointMemory(statePath, map[uint8][]uint8{}, &log)
			assertEmptyKnowledge(t, got.Knowledge)
			if got.Intent != "" || got.IntentAge != 0 {
				t.Errorf("Intent/Age = (%q, %d), want clean start", got.Intent, got.IntentAge)
			}
			if !strings.Contains(log.String(), "starting with empty knowledge") {
				t.Errorf("log = %q, want a line saying the run starts empty", log.String())
			}
		})
	}
}

// TestMemoryMissingFile: no knowledge file beside the state is also a clean
// start with a log line — the state stands on its own.
func TestMemoryMissingFile(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "round-001-frame-0000000001-go-to-pewter-city.state")
	if err := os.WriteFile(statePath, []byte("state-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	got := LoadCheckpointMemory(statePath, map[uint8][]uint8{}, &log)
	assertEmptyKnowledge(t, got.Knowledge)
	if !strings.Contains(log.String(), "starting with empty knowledge") {
		t.Errorf("log = %q, want a line saying the run starts empty", log.String())
	}
}

// TestMemoryIntentOverCap: an intent longer than IntentCap cannot have come
// from a reply that passed WithArgs validation; the file is not what we
// wrote, so it is rejected whole.
func TestMemoryIntentOverCap(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "round-001-frame-0000000001-go-to-pewter-city.state")
	if err := os.WriteFile(statePath, []byte("state-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSuffix(filepath.Base(statePath), ".state")
	knPath := filepath.Join(dir, knowledgeFileName(base))
	data := []byte(`{"version": ` + strconv.Itoa(memoryVersion) + `, "intent": "` + strings.Repeat("a", IntentCap+1) + `"}`)
	if err := os.WriteFile(knPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	got := LoadCheckpointMemory(statePath, map[uint8][]uint8{}, &log)
	assertEmptyKnowledge(t, got.Knowledge)
	if got.Intent != "" {
		t.Errorf("Intent = %q, want clean start", got.Intent)
	}
	if !strings.Contains(log.String(), "starting with empty knowledge") {
		t.Errorf("log = %q, want a line saying the run starts empty", log.String())
	}
}
