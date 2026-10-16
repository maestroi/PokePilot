package editplan

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/media/producer"
)

func fixture(t *testing.T) (producer.Assessment, []farm.MediaTimeline) {
	t.Helper()
	timeline := farm.MediaTimeline{
		Run:     farm.MediaRunSummary{RunID: "edit-run", Status: "done", Goal: "Badge"},
		Attempt: 1, EndFrame: 120000, FramesPerSecond: 60,
		Events: []farm.MediaEvent{
			{Type: "gym_battle", StartFrame: 26000, Frame: 30000, Summary: "Gym battle", Evidence: "gym"},
			{Type: "badge_acquired", Frame: 33000, Summary: "Badge collected", Evidence: "badge"},
			{Type: "blackout", Frame: 60000, Summary: "Blacked out", Evidence: "blackout"},
			{Type: "failure_recovered", Frame: 74000, Summary: "Recovered", Evidence: "recovery"},
			{Type: "run_finished", Frame: 100000, Summary: "Won", Evidence: "end"},
		},
	}.Normalized()
	a, err := producer.Assess(context.Background(), producer.Input{RunID: "edit-run", Timelines: []farm.MediaTimeline{timeline}}, nil, producer.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	return a, []farm.MediaTimeline{timeline}
}

func TestBuildDeterministicEvidenceAndCut(t *testing.T) {
	a, tl := fixture(t)
	p, err := Build(a, tl, Episode)
	if err != nil {
		t.Fatal(err)
	}
	q, err := Build(a, tl, Episode)
	if err != nil {
		t.Fatal(err)
	}
	if p.Hash != q.Hash || p.DurationMS != q.DurationMS {
		t.Fatal("non-deterministic output")
	}
	if err := Validate(p, a, tl); err != nil {
		t.Fatal(err)
	}
	var keep, cut bool
	for _, s := range p.Segments {
		if s.Action == Keep && s.Speed == 1 && len(s.EventIDs) > 0 {
			keep = true
		}
		if s.Action == Cut {
			cut = true
		}
	}
	if !keep || !cut {
		t.Fatalf("expected real-time evidence and cuts, got %+v", p.Segments)
	}
	if p.DurationMS >= p.Sources[0].DurationMS {
		t.Fatal("episode did not shorten long replay")
	}
}

func TestManualSpeedOverlaysAndNarration(t *testing.T) {
	a, tl := fixture(t)
	p, err := Build(a, tl, Episode)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for i := range p.Segments {
		if p.Segments[i].Action == Keep && p.Segments[i].Speed == 1 {
			p.Segments[i].Overlay = "LEVELING"
			p.Segments[i].Caption = "Gym showdown"
			p.Segments[i].NarrationCue = "Voiceover after verification"
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no keep segment")
	}
	p = Seal(p)
	if err := Validate(p, a, tl); err != nil {
		t.Fatal(err)
	}
	for i := range p.Segments {
		if p.Segments[i].Action == Cut && p.Segments[i].EndMS-p.Segments[i].StartMS > 5000 {
			p.Segments[i].Action = Keep
			p.Segments[i].Speed = 8
			break
		}
	}
	p = Seal(p)
	if err := Validate(p, a, tl); err != nil {
		t.Fatal(err)
	}
}

func TestUnsafePlansRejected(t *testing.T) {
	a, tl := fixture(t)
	original, err := Build(a, tl, Episode)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*Plan){
		"overlap": func(p *Plan) {
			for i := 1; i < len(p.Segments); i++ {
				if p.Segments[i].Action == Keep {
					p.Segments[i].StartMS = 0
					break
				}
			}
		},
		"speed": func(p *Plan) {
			for i := range p.Segments {
				if p.Segments[i].Action == Keep {
					p.Segments[i].Speed = 3
					break
				}
			}
		},
		"out of bounds": func(p *Plan) {
			for i := range p.Segments {
				if p.Segments[i].Action == Keep {
					p.Segments[i].EndMS = 999999999
					break
				}
			}
		},
		"unsupported transition": func(p *Plan) { p.Segments[0].Transition = "crossfade" },
		"tamper overlay":         func(p *Plan) { p.Segments[0].Overlay = "forged" },
		"missing evidence": func(p *Plan) {
			for i := range p.Segments {
				if len(p.Segments[i].EventIDs) > 0 {
					p.Segments[i].EventIDs = []string{"evt-fake"}
					break
				}
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			data, _ := json.Marshal(original)
			var p Plan
			if err := json.Unmarshal(data, &p); err != nil {
				t.Fatal(err)
			}
			mutate(&p)
			if name != "tamper overlay" {
				p = Seal(p)
			}
			if err := Validate(p, a, tl); err == nil {
				t.Fatal("accepted invalid edit")
			}
		})
	}
}

func TestShortProfileAndSourceFingerprint(t *testing.T) {
	a, tl := fixture(t)
	_, err := Build(a, tl, Short)
	if err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("expected short planning limit, got %v", err)
	}
	p, err := Build(a, tl, Episode)
	if err != nil {
		t.Fatal(err)
	}
	tl[0].EndFrame++
	if err := Validate(p, a, tl); err == nil {
		t.Fatal("source changed but plan accepted")
	}
}

func TestAttemptOffset(t *testing.T) {
	n, err := OffsetMS([]Source{{1, 60000}, {3, 90000}}, 3, 200)
	if err != nil || n != 60200 {
		t.Fatalf("offset=%d, err=%v", n, err)
	}
	if _, err := OffsetMS([]Source{{1, 1}}, 2, 0); err == nil {
		t.Fatal("unknown attempt accepted")
	}
}
