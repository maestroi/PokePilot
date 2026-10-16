package segment

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/media/editplan"
)

type fakeEditRunner struct{ Calls [][]string }

func (f *fakeEditRunner) Run(_ context.Context, command string, args ...string) (string, error) {
	f.Calls = append(f.Calls, append([]string{command}, args...))
	output := args[len(args)-1]
	if strings.HasSuffix(output, ".mp4") {
		if err := os.WriteFile(output, []byte("test video"), 0600); err != nil {
			return "", err
		}
	}
	return "", nil
}
func TestRenderEditFromExistingVideo(t *testing.T) {
	dir := t.TempDir()
	runner := &fakeEditRunner{}
	plan := editplan.Plan{
		RenderVersion: editplan.RenderVersion, Profile: editplan.Episode,
		Sources: []editplan.Source{{Attempt: 1, DurationMS: 300000}},
		Segments: []editplan.Segment{
			{Action: editplan.Card, DurationMS: 1000, Overlay: "POKEPILOT"},
			{Action: editplan.Cut, Attempt: 1, StartMS: 0, EndMS: 10000},
			{Action: editplan.Keep, Attempt: 1, StartMS: 10000, EndMS: 40000, Speed: 8},
			{Action: editplan.Keep, Attempt: 1, StartMS: 40000, EndMS: 75000, Speed: 1,
				Caption: "Gym showdown", Overlay: "Challenge", NarrationCue: "Commentary cue",
				Chapter: "Gym battle", Transition: "fade"},
			{Action: editplan.Pad, DurationMS: 500},
		},
	}
	result, err := RenderEdit(context.Background(), "recorded-replay.mp4", plan, dir, filepath.Join(dir, "edited.mp4"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.Calls) != 5 {
		t.Fatalf("commands: %d", len(runner.Calls))
	}
	if result.DurationMS != 40250 {
		t.Fatalf("duration=%d", result.DurationMS)
	}
	if len(result.Chapters) != 1 || result.Chapters[0].StartMS != 4750 {
		t.Fatalf("chapters=%+v", result.Chapters)
	}
	if !strings.Contains(strings.Join(runner.Calls[1], " "), "/8") {
		t.Fatal("missing speed filter")
	}
	if !strings.Contains(strings.Join(runner.Calls[2], " "), "drawtext=textfile=") {
		t.Fatal("missing overlay filter")
	}
}
func TestRenderEditPortraitAndInvalidSpeed(t *testing.T) {
	dir := t.TempDir()
	runner := &fakeEditRunner{}
	plan := editplan.Plan{RenderVersion: editplan.RenderVersion, Profile: editplan.Short,
		Sources:  []editplan.Source{{Attempt: 1, DurationMS: 30000}},
		Segments: []editplan.Segment{{Action: editplan.Keep, Attempt: 1, EndMS: 12000, Speed: 2}},
	}
	_, err := RenderEdit(context.Background(), "recorded-replay.mp4", plan, dir, filepath.Join(dir, "short.mp4"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(runner.Calls[0], " "), "scale=1080:1920") {
		t.Fatal("wrong output geometry")
	}
	plan.Segments[0].Speed = 3
	if _, err := RenderEdit(context.Background(), "recorded-replay.mp4", plan, dir, filepath.Join(dir, "invalid.mp4"), runner); err == nil {
		t.Fatal("invalid speed accepted")
	}
}
