package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/game"
)

// mediaBattleSpan is one trainer battle as the battle observer saw it: from
// the first decided turn to the reported result.
type mediaBattleSpan struct {
	StartFrame uint64
	EndFrame   uint64
	Won        bool
}

var mediaTimelineTelemetry = struct {
	sync.Mutex
	result            *agent.Result
	recordingRunID    string
	recordingStart    uint64
	hasRecordingStart bool
	battles           []mediaBattleSpan
	openBattle        *mediaBattleSpan
}{}

// captureMediaBattleTurn opens a trainer-battle span on its first turn. Wild
// battles are not footage worth keeping on their own.
func captureMediaBattleTurn(frame uint64, context game.BattleContext) {
	mediaTimelineTelemetry.Lock()
	defer mediaTimelineTelemetry.Unlock()
	if context != game.BattleContextTrainer {
		mediaTimelineTelemetry.openBattle = nil
		return
	}
	if mediaTimelineTelemetry.openBattle == nil {
		mediaTimelineTelemetry.openBattle = &mediaBattleSpan{StartFrame: frame}
	}
}

func captureMediaBattleResult(frame uint64, result game.BattleResult) {
	mediaTimelineTelemetry.Lock()
	defer mediaTimelineTelemetry.Unlock()
	span := mediaTimelineTelemetry.openBattle
	mediaTimelineTelemetry.openBattle = nil
	if span == nil || frame < span.StartFrame {
		return
	}
	span.EndFrame, span.Won = frame, result == game.BattleWon
	mediaTimelineTelemetry.battles = append(mediaTimelineTelemetry.battles, *span)
}

func captureMediaTimelineResult(res agent.Result) {
	copyRes := res
	copyRes.Outcomes = append([]agent.ObjectiveResult(nil), res.Outcomes...)
	copyRes.OutcomeTimings = append([]agent.ObjectiveTiming(nil), res.OutcomeTimings...)
	mediaTimelineTelemetry.Lock()
	mediaTimelineTelemetry.result = &copyRes
	mediaTimelineTelemetry.Unlock()
}

func captureMediaTimelineRecordingStart(runID string, frame uint64) {
	mediaTimelineTelemetry.Lock()
	mediaTimelineTelemetry.recordingRunID = runID
	mediaTimelineTelemetry.recordingStart = frame
	mediaTimelineTelemetry.hasRecordingStart = true
	mediaTimelineTelemetry.Unlock()
}

func resetMediaTimelineTelemetry() {
	mediaTimelineTelemetry.Lock()
	mediaTimelineTelemetry.result = nil
	mediaTimelineTelemetry.recordingRunID = ""
	mediaTimelineTelemetry.recordingStart = 0
	mediaTimelineTelemetry.hasRecordingStart = false
	mediaTimelineTelemetry.battles = nil
	mediaTimelineTelemetry.openBattle = nil
	mediaTimelineTelemetry.Unlock()
}

func drainMediaTimelineArtifact(spec farm.Spec, reason string, endFrame uint64, artifacts []farm.Artifact) (farm.Artifact, error) {
	mediaTimelineTelemetry.Lock()
	res := mediaTimelineTelemetry.result
	recordingStart := mediaTimelineTelemetry.recordingStart
	hasRecordingStart := mediaTimelineTelemetry.hasRecordingStart && mediaTimelineTelemetry.recordingRunID == spec.RunID
	battles := mediaTimelineTelemetry.battles
	mediaTimelineTelemetry.battles = nil
	mediaTimelineTelemetry.openBattle = nil
	mediaTimelineTelemetry.result = nil
	mediaTimelineTelemetry.recordingRunID = ""
	mediaTimelineTelemetry.recordingStart = 0
	mediaTimelineTelemetry.hasRecordingStart = false
	mediaTimelineTelemetry.Unlock()

	if res == nil {
		return farm.Artifact{}, nil
	}
	origin := res.StartFrame
	if hasRecordingStart {
		origin = recordingStart
	}
	if endFrame < origin {
		endFrame = origin
	}
	timeline := farm.MediaTimeline{
		Version:          farm.MediaTimelineVersion,
		Run:              farm.MediaRunSummary{RunID: spec.RunID, Status: reason, Goal: spec.Goal.String(), Planner: spec.Planner},
		Attempt:          spec.Attempt,
		SourceStartFrame: origin,
		EndFrame:         relativeMediaFrame(endFrame, origin),
		FramesPerSecond:  farm.GameBoyFramesPerSecond,
	}

	if hasMediaObservation(res.Initial) {
		timeline.Snapshots = append(timeline.Snapshots, mediaSnapshotFromObservation(
			relativeMediaFrame(res.StartFrame, origin), 0, "", "", res.Initial,
		))
	}

	previous := res.Initial
	consecutiveFailures := 0
	for i, result := range res.Outcomes {
		if i >= len(res.OutcomeTimings) {
			break
		}
		timing := res.OutcomeTimings[i]
		frame := relativeMediaFrame(timing.Frame, origin)
		objective := result.Objective.String()
		timeline.Snapshots = append(timeline.Snapshots, mediaSnapshotFromObservation(
			frame, timing.Round, objective, string(result.Outcome), result.Final,
		))

		baseType := "objective_failed"
		switch result.Outcome {
		case agent.OutcomeCompleted:
			baseType = "objective_completed"
			consecutiveFailures = 0
		case agent.OutcomeBlocked, agent.OutcomePostconditionFailed:
			baseType = "objective_blocked"
			consecutiveFailures++
		default:
			consecutiveFailures++
		}
		timeline.Events = append(timeline.Events, farm.MediaEvent{
			Type: baseType, Frame: frame, Round: timing.Round, Objective: objective,
			Summary: result.Summary, Evidence: fmt.Sprintf("outcome:%d:%s", i, baseType),
			Metadata: objectiveEventMetadata(result),
		})
		if consecutiveFailures >= 2 {
			timeline.Events = append(timeline.Events, farm.MediaEvent{
				Type: "repeated_failure", Frame: frame, Round: timing.Round, Objective: objective,
				Summary: result.Summary, Evidence: fmt.Sprintf("outcome:%d:repeated_failure", i),
			})
		}
		if result.Recovered {
			timeline.Events = append(timeline.Events, farm.MediaEvent{
				Type: "failure_recovered", Frame: frame, Round: timing.Round, Objective: objective,
				Summary: result.Summary, Evidence: fmt.Sprintf("outcome:%d:recovered", i),
			})
		}
		if result.Terminal {
			timeline.Events = append(timeline.Events, farm.MediaEvent{
				Type: "failure_terminal", Frame: frame, Round: timing.Round, Objective: objective,
				Summary: result.Summary, Evidence: fmt.Sprintf("outcome:%d:terminal", i),
			})
		}
		if objectiveBlackedOut(result) {
			timeline.Events = append(timeline.Events, farm.MediaEvent{
				Type: "blackout", Frame: frame, Round: timing.Round, Objective: objective,
				Summary: result.Summary, Evidence: fmt.Sprintf("outcome:%d:blackout", i),
			})
		}
		if result.Objective.Kind == agent.KindGym {
			timeline.Events = append(timeline.Events, farm.MediaEvent{
				Type: "gym_battle", StartFrame: relativeMediaFrame(timing.StartFrame, origin), Frame: frame,
				Round: timing.Round, Objective: objective,
				Summary: result.Summary, Evidence: fmt.Sprintf("outcome:%d:gym", i),
			})
		}
		for _, badge := range newlyAddedStrings(previous.Badges, result.Final.Badges) {
			timeline.Events = append(timeline.Events, farm.MediaEvent{
				Type: "badge_acquired", Frame: frame, Round: timing.Round, Objective: objective,
				Summary: "acquired " + badge, Evidence: fmt.Sprintf("outcome:%d:badge:%s", i, badge),
				Metadata: map[string]string{"badge": badge},
			})
		}
		if result.Objective.Kind == agent.KindTrain && result.Outcome == agent.OutcomeCompleted {
			for slot, change := range evolvedPartySlots(previous, result.Final) {
				timeline.Events = append(timeline.Events, farm.MediaEvent{
					Type: "evolution", Frame: frame, Round: timing.Round, Objective: objective,
					Summary: change, Evidence: fmt.Sprintf("outcome:%d:evolution:%d", i, slot),
					Metadata: map[string]string{"slot": strconv.Itoa(slot)},
				})
			}
		}
		previous = result.Final
	}

	appendBattleMediaEvents(&timeline, origin, res, battles)

	finalObs := res.Final
	if !hasMediaObservation(finalObs) && len(res.Outcomes) > 0 {
		finalObs = res.Outcomes[len(res.Outcomes)-1].Final
	}
	finalFrame := res.FinalFrame
	if finalFrame == 0 {
		finalFrame = endFrame
	}
	if hasMediaObservation(finalObs) {
		timeline.Snapshots = append(timeline.Snapshots, mediaSnapshotFromObservation(
			relativeMediaFrame(finalFrame, origin), res.Rounds, "", "", finalObs,
		))
	}

	appendCheckpointMediaEvents(&timeline, origin, artifacts)
	end := timeline.EndFrame
	if reason == "stuck" {
		timeline.Events = append(timeline.Events, farm.MediaEvent{
			Type: "stalled", Frame: end, Summary: "run stopped after stagnation", Evidence: "run:stuck",
		})
	}
	finishType := "run_finished"
	if reason == "failed" || reason == "error" || reason == "stuck" {
		finishType = "run_failed"
	}
	timeline.Events = append(timeline.Events, farm.MediaEvent{
		Type: finishType, Frame: end, Summary: reason, Evidence: "run:" + reason,
		Metadata: map[string]string{"reason": reason},
	})
	return farm.NewMediaTimelineArtifact(timeline)
}

func mediaSnapshotFromObservation(frame uint64, round int, objective, outcome string, obs agent.Observation) farm.MediaSnapshot {
	return farm.MediaSnapshot{
		Frame:     frame,
		Round:     round,
		Objective: objective,
		Outcome:   outcome,
		Location: farm.MediaLocation{
			Map: obs.Map, Name: obs.MapName, Place: string(obs.Location), X: obs.X, Y: obs.Y,
		},
		Player:  mediaPlayerFromObservation(obs),
		Planner: farm.MediaPlannerState{Intent: obs.Intent},
		Events:  append([]string(nil), obs.Events...),
	}
}

func mediaPlayerFromObservation(obs agent.Observation) *farm.Player {
	player := &farm.Player{
		Money:    obs.Money,
		Badges:   append([]string(nil), obs.Badges...),
		Party:    make([]farm.PartyMon, 0, len(obs.Party)),
		Bag:      make([]farm.BagItem, 0, len(obs.Bag)),
		DexOwned: len(obs.PokedexOwned),
		DexSeen:  len(obs.PokedexSeen),
	}
	for _, mon := range obs.Party {
		player.Party = append(player.Party, farm.PartyMon{
			Name: string(mon.Species), Level: mon.Level, HP: mon.HP, MaxHP: mon.MaxHP, Status: mon.Status,
		})
	}
	for _, item := range obs.Bag {
		player.Bag = append(player.Bag, farm.BagItem{Name: item.Name, Quantity: item.Quantity})
	}
	player.BagUsed = len(player.Bag)
	return player
}

func hasMediaObservation(obs agent.Observation) bool {
	return obs.Location != "" || obs.MapName != "" || len(obs.Party) > 0 || len(obs.Badges) > 0 || len(obs.Events) > 0 || obs.Money != 0
}

func relativeMediaFrame(frame, origin uint64) uint64 {
	if frame <= origin {
		return 0
	}
	return frame - origin
}

func objectiveBlackedOut(result agent.ObjectiveResult) bool {
	return result.Final.BlackedOut ||
		(result.Travel != nil && result.Travel.BlackedOut) ||
		(result.Train != nil && result.Train.BlackedOut)
}

func objectiveEventMetadata(result agent.ObjectiveResult) map[string]string {
	metadata := map[string]string{"outcome": string(result.Outcome)}
	if result.Cause != "" {
		metadata["cause"] = string(result.Cause)
	}
	if result.Failure != nil && result.Failure.Class != "" {
		metadata["failure_class"] = string(result.Failure.Class)
	}
	return metadata
}

func newlyAddedStrings(before, after []string) []string {
	seen := make(map[string]struct{}, len(before))
	for _, value := range before {
		seen[value] = struct{}{}
	}
	var out []string
	for _, value := range after {
		if _, exists := seen[value]; !exists {
			out = append(out, value)
		}
	}
	return out
}

func evolvedPartySlots(before, after agent.Observation) map[int]string {
	out := map[int]string{}
	limit := len(before.Party)
	if len(after.Party) < limit {
		limit = len(after.Party)
	}
	for i := 0; i < limit; i++ {
		from, to := string(before.Party[i].Species), string(after.Party[i].Species)
		if from != "" && to != "" && from != to {
			out[i] = from + " evolved into " + to
		}
	}
	return out
}

func appendCheckpointMediaEvents(timeline *farm.MediaTimeline, origin uint64, artifacts []farm.Artifact) {
	for _, artifact := range artifacts {
		name := filepath.Base(artifact.Name)
		if !strings.HasSuffix(name, ".state") || !strings.Contains(name, "frame-") {
			continue
		}
		frame, ok := checkpointFrameFromName(name)
		if !ok || frame < origin {
			continue
		}
		round, _ := checkpointRoundFromName(name)
		timeline.Events = append(timeline.Events, farm.MediaEvent{
			Type: "checkpoint", Frame: relativeMediaFrame(frame, origin), Round: round,
			Summary: name, Evidence: "checkpoint:" + name,
		})
	}
}

func checkpointFrameFromName(name string) (uint64, bool) {
	idx := strings.Index(name, "frame-")
	if idx < 0 {
		return 0, false
	}
	rest := name[idx+len("frame-"):]
	end := strings.IndexByte(rest, '-')
	if end >= 0 {
		rest = rest[:end]
	} else if dot := strings.IndexByte(rest, '.'); dot >= 0 {
		rest = rest[:dot]
	}
	frame, err := strconv.ParseUint(rest, 10, 64)
	return frame, err == nil
}

func checkpointRoundFromName(name string) (int, bool) {
	idx := strings.Index(name, "round-")
	if idx < 0 {
		return 0, false
	}
	rest := name[idx+len("round-"):]
	end := strings.IndexByte(rest, '-')
	if end >= 0 {
		rest = rest[:end]
	}
	round, err := strconv.Atoi(rest)
	return round, err == nil
}

// appendBattleMediaEvents turns observed trainer-battle spans into media
// events, classified only by the generic kind of the objective they ran in.
// Gym objectives already carry a gym_battle span. Progress objectives are the
// adapter's required story fights, so their battles are boss_battle; anything
// else is an ordinary trainer_battle.
func appendBattleMediaEvents(timeline *farm.MediaTimeline, origin uint64, res *agent.Result, battles []mediaBattleSpan) {
	for bi, span := range battles {
		eventType, objective, round := "trainer_battle", "", 0
		for i, timing := range res.OutcomeTimings {
			if i >= len(res.Outcomes) || span.EndFrame < timing.StartFrame || span.EndFrame > timing.Frame {
				continue
			}
			objective, round = res.Outcomes[i].Objective.String(), timing.Round
			switch res.Outcomes[i].Objective.Kind {
			case agent.KindGym:
				eventType = ""
			case agent.KindProgress:
				eventType = "boss_battle"
			}
			break
		}
		if eventType == "" {
			continue
		}
		result := "lost"
		if span.Won {
			result = "won"
		}
		timeline.Events = append(timeline.Events, farm.MediaEvent{
			Type: eventType, StartFrame: relativeMediaFrame(span.StartFrame, origin), Frame: relativeMediaFrame(span.EndFrame, origin),
			Round: round, Objective: objective, Summary: result,
			Evidence: fmt.Sprintf("battle:%d:%s", bi, result), Metadata: map[string]string{"result": result},
		})
	}
}
