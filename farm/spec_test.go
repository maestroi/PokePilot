package farm

import (
	"encoding/json"
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
