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
	"sort"
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
	StartMS     int64 // optional source-timeline window start, inclusive
	EndMS       int64 // optional source-timeline window end, exclusive
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

func windowBroadcastPlan(plan broadcastPlan, startMS, endMS int64) broadcastPlan {
	if startMS <= 0 && endMS <= 0 {
		return plan
	}
	if startMS < 0 {
		startMS = 0
	}
	if endMS <= startMS {
		return broadcastPlan{}
	}
	duration := endMS - startMS
	out := broadcastPlan{}

	active := -1
	for i := range plan.States {
		if plan.States[i].StartMS <= startMS {
			active = i
			continue
		}
		break
	}
	if active >= 0 {
		state := plan.States[active]
		state.StartMS = 0
		state.EndMS = duration
		out.States = append(out.States, state)
	}
	for i := range plan.States {
		state := plan.States[i]
		if state.StartMS <= startMS || state.StartMS >= endMS {
			continue
		}
		state.StartMS -= startMS
		if state.EndMS == 0 || state.EndMS > endMS {
			state.EndMS = duration
		} else {
			state.EndMS -= startMS
		}
		out.States = append(out.States, state)
	}
	for i := 0; i+1 < len(out.States); i++ {
		out.States[i].EndMS = out.States[i+1].StartMS
	}
	if len(out.States) > 0 {
		out.States[len(out.States)-1].EndMS = duration
	}

	for _, event := range plan.Events {
		if event.EndMS <= startMS || event.StartMS >= endMS {
			continue
		}
		event.StartMS -= startMS
		event.EndMS -= startMS
		if event.StartMS < 0 {
			event.StartMS = 0
		}
		if event.EndMS > duration {
			event.EndMS = duration
		}
		if event.EndMS > event.StartMS {
			out.Events = append(out.Events, event)
		}
	}
	return out
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
	plan = windowBroadcastPlan(plan, scene.StartMS, scene.EndMS)
	overlayTimeline, err := writeBroadcastOverlayTimeline(dir, plan)
	if err != nil {
		return err
	}

	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	if scene.VAAPI {
		device := strings.TrimSpace(scene.VAAPIDevice)
		if device == "" {
			device = defaultVAAPIDevice
		}
		args = append(args, "-init_hw_device", "vaapi=va:"+device, "-filter_hw_device", "va")
	}
	args = append(args, "-i", scene.RawVideo, "-f", "concat", "-safe", "0", "-i", overlayTimeline)

	var filters strings.Builder
	filters.WriteString("[0:v]scale=704:634:flags=neighbor,pad=1280:720:24:43:color=0x071018[base0];")
	filters.WriteString("[base0][1:v]overlay=0:0:eof_action=repeat:shortest=0[composed];")
	if scene.VAAPI {
		filters.WriteString("[composed]format=nv12,hwupload[outv]")
	} else {
		filters.WriteString("[composed]format=yuv420p[outv]")
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

// writeBroadcastOverlayTimeline composes the active state and event cards at
// each semantic transition. FFmpeg reads this as one sparse video input rather
// than keeping one endlessly looped decoder/filter per snapshot and event.
// Memory therefore stays bounded as long recordings accumulate telemetry.
func writeBroadcastOverlayTimeline(dir string, plan broadcastPlan) (string, error) {
	boundaries := []int64{0}
	for _, state := range plan.States {
		if state.StartMS > 0 {
			boundaries = append(boundaries, state.StartMS)
		}
	}
	for _, event := range plan.Events {
		if event.StartMS > 0 {
			boundaries = append(boundaries, event.StartMS)
		}
		if event.EndMS > 0 {
			boundaries = append(boundaries, event.EndMS)
		}
	}
	sort.Slice(boundaries, func(i, j int) bool { return boundaries[i] < boundaries[j] })
	unique := boundaries[:0]
	for _, at := range boundaries {
		if len(unique) == 0 || at != unique[len(unique)-1] {
			unique = append(unique, at)
		}
	}

	var concat strings.Builder
	concat.WriteString("ffconcat version 1.0\n")
	stateIndex := 0
	for i, at := range unique {
		for stateIndex+1 < len(plan.States) && plan.States[stateIndex+1].StartMS <= at {
			stateIndex++
		}
		img := image.NewRGBA(image.Rect(0, 0, broadcastSceneWidth, broadcastSceneHeight))
		if len(plan.States) > 0 {
			drawBroadcastStateOverlay(img, plan.States[stateIndex])
		}
		for _, event := range plan.Events {
			if event.StartMS <= at && at < event.EndMS {
				drawBroadcastEventOverlay(img, event)
			}
		}
		name := fmt.Sprintf("overlay-%04d.png", i)
		if err := writeScaledPNG(filepath.Join(dir, name), img); err != nil {
			return "", err
		}
		fmt.Fprintf(&concat, "file %s\n", name)
		if i+1 < len(unique) {
			fmt.Fprintf(&concat, "duration %.3f\n", float64(unique[i+1]-at)/1000)
		} else {
			// The concat demuxer needs a final file entry to present the last
			// image before overlay repeats it through the end of the raw video.
			fmt.Fprintf(&concat, "duration 0.001\nfile %s\n", name)
		}
	}
	filename := filepath.Join(dir, "overlay.ffconcat")
	if err := os.WriteFile(filename, []byte(concat.String()), 0o600); err != nil {
		return "", fmt.Errorf("write broadcast overlay timeline: %w", err)
	}
	return filename, nil
}

func drawBroadcastStateOverlay(img stddraw.Image, state broadcastState) {
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
}

func drawBroadcastEventOverlay(img stddraw.Image, event broadcastEventCard) {
	y := 238 + event.Lane*34
	card := image.Rect(20, y, 354, y+30)
	fillRect(img, card, color.RGBA{R: 14, G: 26, B: 34, A: 242})
	fillRect(img, image.Rect(card.Min.X, card.Min.Y, card.Min.X+4, card.Max.Y), color.RGBA{R: 232, G: 178, B: 79, A: 255})
	drawText(img, 30, y+12, color.RGBA{R: 242, G: 197, B: 108, A: 255}, clipBroadcastText(event.Kind, 22))
	drawText(img, 30, y+25, color.RGBA{R: 232, G: 237, B: 239, A: 255}, clipBroadcastText(event.Summary, 45))
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
