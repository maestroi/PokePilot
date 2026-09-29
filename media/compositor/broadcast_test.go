package compositor

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/farm"
)

type captureRunner struct {
	binary string
	args   []string
	output string
	err    error
}

func (r *captureRunner) Run(_ context.Context, binary string, args ...string) (string, error) {
	r.binary = binary
	r.args = append([]string(nil), args...)
	return r.output, r.err
}

func TestBroadcastPlanTracksTimelineTransitions(t *testing.T) {
	timeline := farm.MediaTimeline{
		Run:             farm.MediaRunSummary{RunID: "run-1", Goal: "become champion", Planner: "finish the league"},
		EndFrame:        600,
		FramesPerSecond: 60,
		Snapshots: []farm.MediaSnapshot{
			{
				Frame:     120,
				Objective: "reach Indigo Plateau",
				Location:  farm.MediaLocation{Name: "Route 23", X: 4, Y: 9},
				Player: &farm.Player{
					Badges: []string{"Boulder", "Cascade"},
					Party:  []farm.PartyMon{{Name: "VENUSAUR", Level: 42, HP: 99, MaxHP: 120}},
				},
				Planner: farm.MediaPlannerState{Intent: "prepare for the Elite Four"},
			},
			{
				Frame:     300,
				Objective: "defeat Lorelei",
				Location:  farm.MediaLocation{Place: "Indigo Plateau", X: 7, Y: 3},
				Player: &farm.Player{
					Badges: []string{"Boulder", "Cascade", "Thunder"},
					Party:  []farm.PartyMon{{Name: "VENUSAUR", Level: 45, HP: 110, MaxHP: 128}},
				},
			},
		},
	}.Normalized()

	plan := buildBroadcastPlan("run-1", 2, timeline)
	if len(plan.States) != 3 {
		t.Fatalf("states=%d, want initial + two transitions", len(plan.States))
	}
	if got := plan.States[1]; got.StartMS != 2000 || got.EndMS != 5000 || got.Objective != "reach Indigo Plateau" {
		t.Fatalf("first transition=%+v", got)
	}
	if got := plan.States[2]; got.StartMS != 5000 || got.EndMS != 0 || got.Location != "Indigo Plateau  (7,3)" {
		t.Fatalf("second transition=%+v", got)
	}
	if got := plan.States[1].Party; len(got) != 1 || !strings.Contains(got[0], "L42") || !strings.Contains(got[0], "99/120") {
		t.Fatalf("party=%v", got)
	}
}

func TestWindowBroadcastPlanPreservesBoundaryStateAndEventRemainder(t *testing.T) {
	plan := broadcastPlan{
		States: []broadcastState{
			{StartMS: 0, EndMS: 5000, Elapsed: "00:00", Objective: "first"},
			{StartMS: 5000, EndMS: 10000, Elapsed: "00:05", Objective: "second"},
		},
		Events: []broadcastEventCard{
			{StartMS: 3500, EndMS: 7500, Lane: 1, Kind: "BADGE", Summary: "crosses"},
			{StartMS: 8000, EndMS: 9000, Lane: 0, Kind: "ITEM", Summary: "inside"},
		},
	}
	got := windowBroadcastPlan(plan, 4000, 8000)
	if len(got.States) != 2 {
		t.Fatalf("states=%+v", got.States)
	}
	if got.States[0].StartMS != 0 || got.States[0].EndMS != 1000 || got.States[0].Objective != "first" {
		t.Fatalf("initial boundary state=%+v", got.States[0])
	}
	if got.States[1].StartMS != 1000 || got.States[1].EndMS != 4000 || got.States[1].Objective != "second" {
		t.Fatalf("later state=%+v", got.States[1])
	}
	if len(got.Events) != 1 || got.Events[0].StartMS != 0 || got.Events[0].EndMS != 3500 {
		t.Fatalf("events=%+v", got.Events)
	}
}

func TestBroadcastOverlayTimelineKeepsEventBoundaries(t *testing.T) {
	dir := t.TempDir()
	plan := broadcastPlan{
		States: []broadcastState{
			{StartMS: 0, Objective: "first"},
			{StartMS: 2000, Objective: "second"},
		},
		Events: []broadcastEventCard{{StartMS: 1000, EndMS: 5000, Lane: 0, Kind: "BADGE"}},
	}
	playlist, err := writeBroadcastOverlayTimeline(dir, plan)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(playlist)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), "duration "); got != 4 {
		t.Fatalf("overlay transitions=%d, want 4:\n%s", got, data)
	}
}

func TestFFmpegCompositorInputCountDoesNotGrowWithTelemetry(t *testing.T) {
	runner := &captureRunner{}
	dir := t.TempDir()
	timeline := farm.MediaTimeline{
		Run:             farm.MediaRunSummary{RunID: "run-many-cards"},
		FramesPerSecond: 60,
		EndFrame:        60000,
	}
	for i := 0; i < 96; i++ {
		timeline.Snapshots = append(timeline.Snapshots, farm.MediaSnapshot{Frame: uint64(i * 600)})
	}
	for i := 0; i < 157; i++ {
		timeline.Events = append(timeline.Events, farm.MediaEvent{Frame: uint64(i * 300), Type: "checkpoint"})
	}
	compositor := NewFFmpeg("ffmpeg-test", runner)
	if err := compositor.Compose(context.Background(), Scene{
		RunID: "run-many-cards", Attempt: 1, RawVideo: filepath.Join(dir, "raw.mp4"),
		Destination: filepath.Join(dir, "broadcast.mp4"), Timeline: timeline.Normalized(),
	}); err != nil {
		t.Fatal(err)
	}
	if runner.binary != "ffmpeg-test" {
		t.Fatalf("binary=%q", runner.binary)
	}
	inputs := 0
	for _, arg := range runner.args {
		if arg == "-i" {
			inputs++
		}
	}
	if inputs != 2 {
		t.Fatalf("96 snapshots and 157 events yielded %d FFmpeg inputs, want raw video plus one overlay", inputs)
	}
}

func TestRenderFrameUsesBroadcastLayoutWithoutReplayServer(t *testing.T) {
	raw := image.NewRGBA(image.Rect(0, 0, 160, 144))
	for y := 0; y < raw.Bounds().Dy(); y++ {
		for x := 0; x < raw.Bounds().Dx(); x++ {
			raw.Set(x, y, color.RGBA{B: 255, A: 255})
		}
	}
	var input bytes.Buffer
	if err := png.Encode(&input, raw); err != nil {
		t.Fatal(err)
	}
	timeline := farm.MediaTimeline{
		Run:             farm.MediaRunSummary{RunID: "live-run", Goal: "Win"},
		FramesPerSecond: 60,
		Snapshots: []farm.MediaSnapshot{{
			Frame: 60, Objective: "Reach Pewter", Location: farm.MediaLocation{Name: "Pewter City"},
		}},
	}.Normalized()

	encoded, err := RenderFrame(input.Bytes(), "live-run", 1, timeline)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Size(); got.X != OutputWidth || got.Y != OutputHeight {
		t.Fatalf("frame size=%v want %dx%d", got, OutputWidth, OutputHeight)
	}
}

func TestBroadcastPlanEventCardsHaveDeterministicTimingAndLanes(t *testing.T) {
	timeline := farm.MediaTimeline{
		Run:             farm.MediaRunSummary{RunID: "run-events"},
		EndFrame:        600,
		FramesPerSecond: 60,
		Events: []farm.MediaEvent{
			{Type: "badge_acquired", Frame: 60, Summary: "Boulder Badge"},
			{Type: "checkpoint", Frame: 120, Summary: "Pewter checkpoint"},
			{Type: "blackout", Frame: 180, Summary: "Party wiped"},
			{Type: "recovery", Frame: 360, Summary: "Recovered"},
		},
	}.Normalized()
	plan := buildBroadcastPlan("run-events", 1, timeline)
	if len(plan.Events) != 4 {
		t.Fatalf("events=%d", len(plan.Events))
	}
	if got := plan.Events[0]; got.StartMS != 1000 || got.EndMS != 5000 || got.Lane != 0 || got.Kind != "BADGE ACQUIRED" {
		t.Fatalf("event0=%+v", got)
	}
	if plan.Events[1].Lane != 1 || plan.Events[2].Lane != 2 || plan.Events[3].Lane != 0 {
		t.Fatalf("lanes=%d,%d,%d", plan.Events[1].Lane, plan.Events[2].Lane, plan.Events[3].Lane)
	}
}
