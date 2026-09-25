package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	stddraw "image/draw"
	"image/png"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/maestroi/pokepilot/farm"
	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const (
	broadcastRendererVersion = "broadcast-1280x720-v1"
	broadcastWidth           = 1280
	broadcastHeight          = 720
	broadcastSceneWidth      = 640
	broadcastSceneHeight     = 360
	broadcastEventDurationMS = int64(4000)
	broadcastEventLanes      = 3
)

type replayMode string

const (
	replayModeBroadcast replayMode = "broadcast"
	replayModeRaw       replayMode = "raw"
)

func parseReplayMode(value string) (replayMode, error) {
	switch replayMode(strings.ToLower(strings.TrimSpace(value))) {
	case "", replayModeBroadcast:
		return replayModeBroadcast, nil
	case replayModeRaw:
		return replayModeRaw, nil
	default:
		return "", fmt.Errorf("unsupported replay mode %q (want broadcast or raw)", value)
	}
}

type replayCompositor interface {
	Version() string
	Compose(context.Context, broadcastScene) error
}

type broadcastScene struct {
	RunID       string
	Attempt     int
	RawVideo    string
	Destination string
	Timeline    farm.MediaTimeline
	VAAPI       bool
	VAAPIDevice string
}

type ffmpegBroadcastCompositor struct {
	binary string
}

func (c *ffmpegBroadcastCompositor) Version() string { return broadcastRendererVersion }

type broadcastPlan struct {
	States []broadcastState
	Events []broadcastEventCard
}

type broadcastState struct {
	StartMS   int64
	EndMS     int64
	RunID     string
	Attempt   int
	Elapsed   string
	Objective string
	Location  string
	Badges    string
	Party     []string
	Planner   string
}

type broadcastEventCard struct {
	StartMS int64
	EndMS   int64
	Lane    int
	Kind    string
	Summary string
}

func (s *replayServer) replayCacheKeyForMode(runID string, recordings []replayRecording, mode replayMode) string {
	version := broadcastRendererVersion
	if s.compositor != nil {
		version = s.compositor.Version()
	}
	return replaySetCacheKeyForMode(runID, recordings, mode, version)
}

func replaySetCacheKeyForMode(runID string, recordings []replayRecording, mode replayMode, rendererVersion string) string {
	raw := replaySetCacheKey(runID, recordings)
	if mode == replayModeRaw {
		return raw
	}
	version := sanitizeKeySegment(rendererVersion)
	if version == "" {
		version = "broadcast"
	}
	ext := path.Ext(raw)
	return strings.TrimSuffix(raw, ext) + "-" + version + ext
}

func buildBroadcastPlan(runID string, attempt int, timeline farm.MediaTimeline) broadcastPlan {
	timeline = timeline.Normalized()
	plan := broadcastPlan{
		States: []broadcastState{broadcastStateAt(runID, attempt, timeline, nil, 0)},
	}
	for i := range timeline.Snapshots {
		snapshot := timeline.Snapshots[i]
		state := broadcastStateAt(runID, attempt, timeline, &snapshot, snapshot.TimestampMS)
		if len(plan.States) > 0 && plan.States[len(plan.States)-1].StartMS == state.StartMS {
			plan.States[len(plan.States)-1] = state
		} else {
			plan.States = append(plan.States, state)
		}
	}
	for i := 0; i+1 < len(plan.States); i++ {
		plan.States[i].EndMS = plan.States[i+1].StartMS
	}

	laneUntil := make([]int64, broadcastEventLanes)
	for _, event := range timeline.Events {
		start := event.TimestampMS
		end := start + broadcastEventDurationMS
		lane := 0
		found := false
		for i := range laneUntil {
			if laneUntil[i] <= start {
				lane = i
				found = true
				break
			}
		}
		if !found {
			lane = 0
			for i := 1; i < len(laneUntil); i++ {
				if laneUntil[i] < laneUntil[lane] {
					lane = i
				}
			}
		}
		laneUntil[lane] = end
		summary := firstNonEmpty(event.Summary, event.Objective, humanizeEventType(event.Type))
		plan.Events = append(plan.Events, broadcastEventCard{
			StartMS: start,
			EndMS:   end,
			Lane:    lane,
			Kind:    humanizeEventType(event.Type),
			Summary: summary,
		})
	}
	return plan
}

func broadcastStateAt(runID string, attempt int, timeline farm.MediaTimeline, snapshot *farm.MediaSnapshot, timestampMS int64) broadcastState {
	state := broadcastState{
		StartMS:   timestampMS,
		RunID:     runID,
		Attempt:   attempt,
		Elapsed:   formatBroadcastElapsed(timestampMS),
		Objective: firstNonEmpty(timeline.Run.Goal, "Telemetry unavailable"),
		Location:  "Location unavailable",
		Badges:    "None recorded",
		Planner:   firstNonEmpty(timeline.Run.Planner, "No planner summary"),
	}
	if snapshot == nil {
		return state
	}
	state.Objective = firstNonEmpty(snapshot.Objective, timeline.Run.Goal, "No active objective")
	state.Location = broadcastLocation(snapshot.Location)
	state.Planner = firstNonEmpty(snapshot.Planner.Intent, snapshot.Planner.PlanGoal, snapshot.Planner.ReplanReason, timeline.Run.Planner, "No planner summary")
	if snapshot.Player == nil {
		return state
	}
	if len(snapshot.Player.Badges) > 0 {
		state.Badges = strings.Join(snapshot.Player.Badges, ", ")
	}
	for _, mon := range snapshot.Player.Party {
		status := ""
		if strings.TrimSpace(mon.Status) != "" {
			status = " " + strings.ToUpper(strings.TrimSpace(mon.Status))
		}
		state.Party = append(state.Party, fmt.Sprintf("%s  L%d  HP %d/%d%s", mon.Name, mon.Level, mon.HP, mon.MaxHP, status))
	}
	return state
}

func broadcastLocation(location farm.MediaLocation) string {
	name := firstNonEmpty(location.Place, location.Name)
	if name == "" {
		name = fmt.Sprintf("Map %d", location.Map)
	}
	return fmt.Sprintf("%s  (%d,%d)", name, location.X, location.Y)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func formatBroadcastElapsed(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	total := ms / 1000
	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

func humanizeEventType(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "_", " "))
	if value == "" {
		return "EVENT"
	}
	return strings.ToUpper(value)
}

func (c *ffmpegBroadcastCompositor) Compose(ctx context.Context, scene broadcastScene) error {
	binary := strings.TrimSpace(c.binary)
	if binary == "" {
		binary = "ffmpeg"
	}
	dir, err := os.MkdirTemp(filepath.Dir(scene.Destination), ".pokereplay-broadcast-*")
	if err != nil {
		return fmt.Errorf("create broadcast render dir: %w", err)
	}
	defer os.RemoveAll(dir)

	plan := buildBroadcastPlan(scene.RunID, scene.Attempt, scene.Timeline)
	inputs := make([]string, 0, len(plan.States)+len(plan.Events))
	for i, state := range plan.States {
		file := filepath.Join(dir, fmt.Sprintf("state-%04d.png", i))
		if err := writeBroadcastStatePNG(file, state); err != nil {
			return err
		}
		inputs = append(inputs, file)
	}
	for i, event := range plan.Events {
		file := filepath.Join(dir, fmt.Sprintf("event-%04d.png", i))
		if err := writeBroadcastEventPNG(file, event); err != nil {
			return err
		}
		inputs = append(inputs, file)
	}

	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	if scene.VAAPI {
		device := strings.TrimSpace(scene.VAAPIDevice)
		if device == "" {
			device = defaultVAAPIDevice
		}
		args = append(args, "-init_hw_device", "vaapi=va:"+device, "-filter_hw_device", "va")
	}
	args = append(args, "-i", scene.RawVideo)
	for _, input := range inputs {
		args = append(args, "-loop", "1", "-framerate", "1", "-i", input)
	}

	var filters strings.Builder
	filters.WriteString("[0:v]scale=704:634:flags=neighbor,pad=1280:720:24:43:color=0x071018[base0];")
	current := "base0"
	inputIndex := 1
	stage := 1
	for _, state := range plan.States {
		next := fmt.Sprintf("base%d", stage)
		fmt.Fprintf(&filters, "[%s][%d:v]overlay=0:0:shortest=1:enable='%s'[%s];", current, inputIndex, ffmpegEnable(state.StartMS, state.EndMS), next)
		current = next
		inputIndex++
		stage++
	}
	for _, event := range plan.Events {
		next := fmt.Sprintf("base%d", stage)
		fmt.Fprintf(&filters, "[%s][%d:v]overlay=0:0:shortest=1:enable='%s'[%s];", current, inputIndex, ffmpegEnable(event.StartMS, event.EndMS), next)
		current = next
		inputIndex++
		stage++
	}
	if scene.VAAPI {
		fmt.Fprintf(&filters, "[%s]format=nv12,hwupload[outv]", current)
	} else {
		fmt.Fprintf(&filters, "[%s]format=yuv420p[outv]", current)
	}
	filterPath := filepath.Join(dir, "filter.txt")
	if err := os.WriteFile(filterPath, []byte(filters.String()), 0o600); err != nil {
		return fmt.Errorf("write broadcast filter: %w", err)
	}
	args = append(args, "-filter_complex_script", filterPath, "-map", "[outv]", "-an")
	if scene.VAAPI {
		args = append(args, "-c:v", "h264_vaapi")
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "20")
	}
	args = append(args, "-movflags", "+faststart", "-shortest", scene.Destination)

	output := &replayOutputTail{}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("broadcast replay compose: %w: %s", err, strings.TrimSpace(output.String()))
	}
	return nil
}

func ffmpegEnable(startMS, endMS int64) string {
	start := float64(startMS) / 1000
	if endMS <= startMS {
		return fmt.Sprintf("gte(t,%.3f)", start)
	}
	end := float64(endMS) / 1000
	return fmt.Sprintf("between(t,%.3f,%.3f)", start, end)
}

func writeBroadcastStatePNG(filename string, state broadcastState) error {
	img := image.NewRGBA(image.Rect(0, 0, broadcastSceneWidth, broadcastSceneHeight))
	fillRect(img, image.Rect(12, 4, 628, 20), color.RGBA{R: 9, G: 24, B: 34, A: 238})
	fillRect(img, image.Rect(372, 24, 628, 348), color.RGBA{R: 8, G: 19, B: 28, A: 244})
	drawFrame(img, image.Rect(10, 20, 366, 341), color.RGBA{R: 58, G: 182, B: 206, A: 220})

	title := fmt.Sprintf("POKEPILOT  RUN %s  ATTEMPT %d  T+%s", shortBroadcastID(state.RunID), state.Attempt, state.Elapsed)
	drawText(img, 18, 17, color.RGBA{R: 196, G: 232, B: 239, A: 255}, clipBroadcastText(title, 80))

	y := 42
	y = drawSection(img, 384, y, "OBJECTIVE", state.Objective, 31)
	y = drawSection(img, 384, y+4, "LOCATION", state.Location, 31)
	y = drawSection(img, 384, y+4, "BADGES", state.Badges, 31)
	drawText(img, 384, y+6, color.RGBA{R: 83, G: 200, B: 219, A: 255}, "PARTY")
	y += 20
	if len(state.Party) == 0 {
		drawText(img, 384, y, color.RGBA{R: 142, G: 159, B: 171, A: 255}, "Party unavailable")
		y += 15
	} else {
		for _, line := range state.Party {
			drawText(img, 384, y, color.RGBA{R: 222, G: 232, B: 236, A: 255}, clipBroadcastText(line, 34))
			y += 15
		}
	}
	_ = drawSection(img, 384, y+4, "PLANNER", state.Planner, 31)
	return writeScaledPNG(filename, img)
}

func writeBroadcastEventPNG(filename string, event broadcastEventCard) error {
	img := image.NewRGBA(image.Rect(0, 0, broadcastSceneWidth, broadcastSceneHeight))
	y := 238 + event.Lane*34
	card := image.Rect(20, y, 354, y+30)
	fillRect(img, card, color.RGBA{R: 14, G: 26, B: 34, A: 242})
	fillRect(img, image.Rect(card.Min.X, card.Min.Y, card.Min.X+4, card.Max.Y), color.RGBA{R: 232, G: 178, B: 79, A: 255})
	drawText(img, 30, y+12, color.RGBA{R: 242, G: 197, B: 108, A: 255}, clipBroadcastText(event.Kind, 22))
	drawText(img, 30, y+25, color.RGBA{R: 232, G: 237, B: 239, A: 255}, clipBroadcastText(event.Summary, 45))
	return writeScaledPNG(filename, img)
}

func drawSection(img stddraw.Image, x, y int, label, value string, width int) int {
	drawText(img, x, y, color.RGBA{R: 83, G: 200, B: 219, A: 255}, label)
	y += 15
	for _, line := range wrapBroadcastText(value, width) {
		drawText(img, x, y, color.RGBA{R: 222, G: 232, B: 236, A: 255}, line)
		y += 15
	}
	return y
}

func drawText(dst stddraw.Image, x, baseline int, c color.Color, text string) {
	d := font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(c),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(x, baseline),
	}
	d.DrawString(text)
}

func drawFrame(dst stddraw.Image, rect image.Rectangle, c color.Color) {
	fillRect(dst, image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+1), c)
	fillRect(dst, image.Rect(rect.Min.X, rect.Max.Y-1, rect.Max.X, rect.Max.Y), c)
	fillRect(dst, image.Rect(rect.Min.X, rect.Min.Y, rect.Min.X+1, rect.Max.Y), c)
	fillRect(dst, image.Rect(rect.Max.X-1, rect.Min.Y, rect.Max.X, rect.Max.Y), c)
}

func fillRect(dst stddraw.Image, rect image.Rectangle, c color.Color) {
	stddraw.Draw(dst, rect, image.NewUniform(c), image.Point{}, stddraw.Src)
}

func writeScaledPNG(filename string, src image.Image) error {
	dst := image.NewRGBA(image.Rect(0, 0, broadcastWidth, broadcastHeight))
	draw.NearestNeighbor.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("create broadcast overlay: %w", err)
	}
	if err := png.Encode(file, dst); err != nil {
		_ = file.Close()
		return fmt.Errorf("encode broadcast overlay: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close broadcast overlay: %w", err)
	}
	return nil
}

func wrapBroadcastText(value string, width int) []string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return []string{"-"}
	}
	if width < 8 {
		width = 8
	}
	words := strings.Fields(value)
	lines := make([]string, 0, 2)
	var line string
	for _, word := range words {
		if len(word) > width {
			word = clipBroadcastText(word, width)
		}
		if line == "" {
			line = word
			continue
		}
		if len(line)+1+len(word) <= width {
			line += " " + word
			continue
		}
		lines = append(lines, line)
		line = word
		if len(lines) == 3 {
			break
		}
	}
	if line != "" && len(lines) < 3 {
		lines = append(lines, line)
	}
	if len(lines) == 3 && strings.Join(lines, " ") != value {
		lines[2] = clipBroadcastText(lines[2], width-1) + "..."
	}
	return lines
}

func clipBroadcastText(value string, width int) string {
	value = strings.TrimSpace(value)
	if width <= 0 || len(value) <= width {
		return value
	}
	if width == 1 {
		return "."
	}
	return strings.TrimSpace(value[:width-1]) + "..."
}

func shortBroadcastID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 18 {
		return value
	}
	return value[:18]
}
