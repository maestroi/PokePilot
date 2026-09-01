package farm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSpecJSONRoundTrip(t *testing.T) {
	want := Spec{
		RunID: "r1", Seed: 42, Planner: "llm", Starter: "squirtle",
		Dest: "viridian pokemon center", Goal: "Earn the Boulder Badge.",
		FPS: 0, MaxRounds: 32, MaxFrames: 1000, Endless: true, RandomSeed: true,
	}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Spec
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != want {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
	for _, field := range []string{`"run_id"`, `"seed"`, `"planner"`, `"starter"`, `"dest"`, `"goal"`, `"fps"`, `"max_rounds"`, `"max_frames"`, `"endless"`, `"random_seed"`} {
		if !json.Valid(b) {
			t.Fatalf("invalid JSON: %s", b)
		}
		if !contains(string(b), field) {
			t.Errorf("marshaled spec missing field %s: %s", field, b)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestHeartbeatReplyJSON(t *testing.T) {
	b, err := json.Marshal(HeartbeatReply{Cancel: true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got HeartbeatReply
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.Cancel {
		t.Fatalf("cancel round-tripped false")
	}
}

func TestHeartbeatCarriesLLMStats(t *testing.T) {
	want := Heartbeat{
		RunID: "r1", Frame: 100,
		Stats: &LLMStats{
			Round: 3, RoundsLeft: 29, Calls: 4, Rounds: 3, Rejected: 1, Repeats: 1,
			AvgOffered: 5.5, LastSeconds: 4.4, AvgSeconds: 3.1,
			PromptTokens: 947, CompletionTokens: 36,
			Intent: "get a move on the badge", IntentAge: 2,
			Choices: []ChoiceCount{{Objective: "go to pallet town", Count: 2}},
		},
	}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Heartbeat
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Stats == nil || !reflect.DeepEqual(*got.Stats, *want.Stats) {
		t.Fatalf("stats round trip = %+v, want %+v", got.Stats, want.Stats)
	}
	for _, field := range []string{`"stats"`, `"rounds_left"`, `"avg_offered"`, `"prompt_tokens"`, `"choices"`, `"objective"`} {
		if !contains(string(b), field) {
			t.Errorf("marshaled heartbeat missing %s: %s", field, b)
		}
	}

	b, err = json.Marshal(Heartbeat{RunID: "r2"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if contains(string(b), `"stats"`) {
		t.Errorf("nil stats must be omitted: %s", b)
	}
}

func TestHeartbeatCarriesMapOverlay(t *testing.T) {
	want := Heartbeat{
		RunID: "map-run", Map: 0x0d, X: 7, Y: 31,
		Sprites: []MapSprite{{X: 8, Y: 31, PictureID: 0x22, Slot: 3}},
		Trail:   [][2]uint8{{5, 31}, {6, 31}, {7, 31}},
	}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Heartbeat
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got.Sprites, want.Sprites) || !reflect.DeepEqual(got.Trail, want.Trail) {
		t.Fatalf("map overlay round trip = sprites %#v trail %#v, want %#v %#v", got.Sprites, got.Trail, want.Sprites, want.Trail)
	}
	for _, field := range []string{`"sprites"`, `"trail"`, `"picture_id"`, `"slot"`} {
		if !contains(string(b), field) {
			t.Errorf("marshaled heartbeat missing %s: %s", field, b)
		}
	}

	b, err = json.Marshal(Heartbeat{RunID: "old"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if contains(string(b), `"sprites"`) || contains(string(b), `"trail"`) {
		t.Errorf("empty overlay must be omitted: %s", b)
	}
}

func TestFinishReportJSONRoundTrip(t *testing.T) {
	data := []byte("checkpoint-state")
	sum := sha256.Sum256(data)
	want := FinishReport{
		RunID:         "r1",
		Attempt:       1,
		Reason:        "error",
		Detail:        "stuck",
		TraceTail:     []string{"a", "b"},
		SaveState:     []byte("final-state"),
		RunnerVersion: "7b66005",
		SeedBurn:      0,
		Artifacts: []Artifact{{
			Name:      "round-001-frame-0000000100-goto.state",
			MediaType: "application/octet-stream",
			SHA256:    hex.EncodeToString(sum[:]),
			Data:      data,
		}},
	}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, field := range []string{`"runner_version"`, `"seed_burn"`, `"artifacts"`, `"name"`, `"media_type"`, `"sha256"`, `"data"`} {
		if !contains(string(b), field) {
			t.Errorf("marshaled finish missing %s: %s", field, b)
		}
	}
	if !contains(string(b), `"seed_burn":0`) {
		t.Errorf("zero seed_burn must be preserved: %s", b)
	}
	var got FinishReport
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

func TestFinishReportCarriesProgressAtTwoPoints(t *testing.T) {
	want := FinishReport{
		RunID:  "r1",
		Reason: "budget",
		ProgressEarly: &Progress{Round: 0, Badges: 0, Events: 5, Maps: 1, Map: 0x31, MapName: "Pallet Town"},
		ProgressFinal: &Progress{Round: 20, Badges: 0, Events: 5, Maps: 3, Map: 0x0d, MapName: "Route 2"},
	}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, field := range []string{`"progress_early"`, `"progress_final"`, `"round"`, `"badges"`, `"events"`, `"maps"`, `"map"`, `"map_name"`} {
		if !contains(string(b), field) {
			t.Errorf("marshaled finish missing %s: %s", field, b)
		}
	}
	var got FinishReport
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
	var bare FinishReport
	bareB, err := json.Marshal(bare)
	if err != nil {
		t.Fatalf("marshal bare: %v", err)
	}
	if contains(string(bareB), "progress_early") || contains(string(bareB), "progress_final") {
		t.Errorf("unsampled report must omit the progress keys: %s", bareB)
	}
}

func TestFinishReportJSONOmitsNewFields(t *testing.T) {
	var got FinishReport
	if err := json.Unmarshal([]byte(`{"run_id":"old","reason":"done"}`), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.RunID != "old" || got.Reason != "done" {
		t.Fatalf("legacy fields = %+v", got)
	}
	if got.RunnerVersion != "" || got.SeedBurn != 0 || got.Artifacts != nil {
		t.Fatalf("omitted evidence must stay zero: %+v", got)
	}
	if got.ProgressEarly != nil || got.ProgressFinal != nil {
		t.Fatalf("a legacy dump carries no progress samples: %+v", got)
	}
}

func TestValidateFinishArtifacts(t *testing.T) {
	data := []byte("ok")
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	ok := FinishReport{SeedBurn: 0, Artifacts: []Artifact{{Name: "periodic-00000018000.state", MediaType: "application/octet-stream", SHA256: hash, Data: data}}}
	if err := ValidateFinishArtifacts(ok); err != nil { t.Fatalf("valid report: %v", err) }
	dup := ok; dup.Artifacts = []Artifact{ok.Artifacts[0], ok.Artifacts[0]}; if err := ValidateFinishArtifacts(dup); err == nil { t.Fatal("duplicate names must be rejected") }
	empty := ok; empty.Artifacts = []Artifact{{Name: "", MediaType: "application/octet-stream", SHA256: hash, Data: data}}; if err := ValidateFinishArtifacts(empty); err == nil { t.Fatal("empty names must be rejected") }
	pathName := ok; pathName.Artifacts = []Artifact{{Name: "../evil.state", MediaType: "application/octet-stream", SHA256: hash, Data: data}}; if err := ValidateFinishArtifacts(pathName); err == nil { t.Fatal("path names must be rejected") }
	space := ok; space.Artifacts = []Artifact{{Name: "bad name.state", MediaType: "application/octet-stream", SHA256: hash, Data: data}}; if err := ValidateFinishArtifacts(space); err == nil { t.Fatal("non-ASCII-conservative names must be rejected") }
	neg := ok; neg.SeedBurn = -1; if err := ValidateFinishArtifacts(neg); err == nil { t.Fatal("negative seed burn must be rejected") }
	mismatch := ok; mismatch.Artifacts = []Artifact{{Name: "periodic-00000018000.state", MediaType: "application/octet-stream", SHA256: strings.Repeat("0", 64), Data: data}}; if err := ValidateFinishArtifacts(mismatch); err == nil { t.Fatal("mismatched SHA-256 must be rejected") }
	upper := ok; upper.Artifacts = []Artifact{{Name: "periodic-00000018000.state", MediaType: "application/octet-stream", SHA256: strings.ToUpper(hash), Data: data}}; if err := ValidateFinishArtifacts(upper); err == nil { t.Fatal("uppercase SHA-256 must be rejected") }
	over := ok
	over.Artifacts = []Artifact{{Name: "huge.state", MediaType: "application/octet-stream", SHA256: hash, Data: make([]byte, MaxFinishArtifactBytes+1)}}
	over.Artifacts[0].SHA256 = sha256Hex(over.Artifacts[0].Data)
	if err := ValidateFinishArtifacts(over); err == nil { t.Fatal("over-budget payload must be rejected") }
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
