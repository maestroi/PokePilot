package main

import (
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/farm"
)

// objectiveFailureTelemetry stores the just-finished agent result until the
// FinishReport is assembled. Identity is taken from ObjectiveResult's semantic
// objective/state/cause fields; agent log prose is no longer an input.
var objectiveFailureTelemetry = struct {
	sync.Mutex
	result *agent.Result
}{}

// observeAgentLogLine intentionally does nothing. agentTraceLog still calls it
// for compatibility with its line-buffering tests, but farm failure identity
// must never be reconstructed by parsing human diagnostics.
func observeAgentLogLine(string) {}

func captureObjectiveFailureTelemetry(res agent.Result) {
	copyRes := res
	copyRes.Outcomes = append([]agent.ObjectiveResult(nil), res.Outcomes...)
	objectiveFailureTelemetry.Lock()
	objectiveFailureTelemetry.result = &copyRes
	objectiveFailureTelemetry.Unlock()
}

// drainObjectiveFailureTelemetry converts the completed run's structured
// ObjectiveResults into per-fingerprint occurrence groups and clears the
// process-local slot before the next lease. The returned terminal occurrence,
// when non-nil, is also used as the run's stable top-level triage identity.
func drainObjectiveFailureTelemetry(reason, build, checkpointDir string) ([]farm.ObjectiveFailure, *farm.FailureOccurrence) {
	objectiveFailureTelemetry.Lock()
	res := objectiveFailureTelemetry.result
	objectiveFailureTelemetry.result = nil
	objectiveFailureTelemetry.Unlock()
	if res == nil {
		return nil, nil
	}

	type grouped struct {
		failure        farm.ObjectiveFailure
		lastIdx        int
		lastOccurrence farm.FailureOccurrence
	}
	groups := map[string]*grouped{}
	observedAt := time.Now().UTC()
	var terminal *farm.FailureOccurrence
	lastFailureIdx := -1
	lastFailureFingerprint := ""

	for i, result := range res.Outcomes {
		if result.Outcome == agent.OutcomeCompleted || expectedStructuredGameOutcome(result) {
			continue
		}
		identity := farmIdentityFromAgent(result)
		checkpoint := checkpointForRound(checkpointDir, i+1)
		occurrence, err := farm.NewFailureOccurrence(identity, build, i+1, checkpoint, result.Summary, observedAt)
		if err != nil {
			continue
		}
		g := groups[occurrence.Fingerprint]
		if g == nil {
			id := occurrence.Identity
			g = &grouped{failure: farm.ObjectiveFailure{
				Objective:    result.Objective.String(),
				Error:        result.Summary,
				FirstRound:   i + 1,
				Map:          result.Final.Map,
				X:            result.Final.X,
				Y:            result.Final.Y,
				ObservedAt:   observedAt,
				Key:          occurrence.Key,
				Fingerprint:  occurrence.Fingerprint,
				Identity:     &id,
				Build:        occurrence.Build,
				Outcome:      string(result.Outcome),
				Cause:        string(result.Cause),
				CauseContext: append([]string(nil), result.CauseContext...),
				Checkpoint:   occurrence.Checkpoint,
			}}
			groups[occurrence.Fingerprint] = g
		}
		g.failure.Count++
		if result.Recovered {
			g.failure.RecoveredCount++
		}
		if result.Terminal {
			g.failure.TerminalCount++
			occ := occurrence
			terminal = &occ
		}
		g.failure.LastRound = i + 1
		g.failure.Map = result.Final.Map
		g.failure.X = result.Final.X
		g.failure.Y = result.Final.Y
		g.failure.Error = result.Summary
		g.failure.Checkpoint = occurrence.Checkpoint
		g.lastIdx = i
		g.lastOccurrence = occurrence
		lastFailureIdx = i
		lastFailureFingerprint = occurrence.Fingerprint
	}

	// Backward compatibility for Results produced before per-occurrence impact
	// flags existed. New Run paths mark every non-completed result explicitly.
	failureDrivenStop := reason == "failed" || reason == "stuck" || reason == "error"
	if terminal == nil && failureDrivenStop && lastFailureIdx >= 0 {
		if g := groups[lastFailureFingerprint]; g != nil && g.failure.TerminalCount == 0 {
			g.failure.TerminalCount++
			occ := g.lastOccurrence
			terminal = &occ
		}
	}

	out := make([]farm.ObjectiveFailure, 0, len(groups))
	for _, g := range groups {
		unknown := g.failure.Count - g.failure.RecoveredCount - g.failure.TerminalCount
		if unknown > 0 && (reason == "done" || majorProgressAfter(res.Outcomes, g.lastIdx)) {
			g.failure.RecoveredCount += unknown
		}
		g.failure.Recovered = g.failure.RecoveredCount > 0 && g.failure.TerminalCount == 0
		g.failure.Blocking = g.failure.TerminalCount > 0 && g.failure.Count >= 2 && failureDrivenStop
		out = append(out, g.failure)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Blocking != out[j].Blocking {
			return out[i].Blocking
		}
		if out[i].TerminalCount != out[j].TerminalCount {
			return out[i].TerminalCount > out[j].TerminalCount
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].LastRound != out[j].LastRound {
			return out[i].LastRound > out[j].LastRound
		}
		return out[i].Fingerprint < out[j].Fingerprint
	})
	return out, terminal
}

func farmIdentityFromAgent(result agent.ObjectiveResult) farm.FailureIdentity {
	initial := agent.FailureState{}
	if result.Initial != nil {
		initial = *result.Initial
	}
	return farm.FailureIdentity{
		Version:      farm.FailureIdentityVersion,
		Game:         "pokemon",
		Adapter:      "pokemon-red",
		Objective:    farmObjectiveFromAgent(agent.FailureObjectiveFor(result.Objective)),
		Outcome:      string(result.Outcome),
		Cause:        string(result.Cause),
		CauseContext: append([]string(nil), result.CauseContext...),
		Initial:      farmStateFromAgent(initial),
		Final:        farmStateFromAgent(agent.FailureStateFor(result.Final)),
	}
}

func farmObjectiveFromAgent(o agent.FailureObjective) farm.FailureObjective {
	return farm.FailureObjective{
		Kind:     o.Kind,
		Place:    string(o.Place),
		X:        o.X,
		Y:        o.Y,
		Starter:  o.Starter,
		Progress: string(o.Progress),
		Level:    o.Level,
		Species:  string(o.Species),
		Item:     string(o.Item),
		Slot:     o.Slot,
		Qty:      o.Qty,
		Flee:     o.Flee,
	}
}

func farmStateFromAgent(s agent.FailureState) farm.FailureState {
	out := farm.FailureState{
		Location:     string(s.Location),
		X:            s.X,
		Y:            s.Y,
		Controllable: s.Controllable,
		InBattle:     s.InBattle,
		Money:        s.Money,
		Party:        make([]farm.FailurePartyMember, 0, len(s.Party)),
		Inventory:    make([]farm.FailureInventoryItem, 0, len(s.Inventory)),
		Badges:       append([]string(nil), s.Badges...),
		Capabilities: make([]farm.FailureCapability, 0, len(s.Capabilities)),
		Progress:     make([]farm.FailureProgressFact, 0, len(s.Progress)),
	}
	for _, mon := range s.Party {
		out.Party = append(out.Party, farm.FailurePartyMember{
			Species: string(mon.Species), Level: mon.Level, HP: mon.HP, MaxHP: mon.MaxHP, Status: mon.Status,
		})
	}
	for _, item := range s.Inventory {
		out.Inventory = append(out.Inventory, farm.FailureInventoryItem{ID: string(item.ID), Quantity: item.Quantity})
	}
	for _, cap := range s.Capabilities {
		out.Capabilities = append(out.Capabilities, farm.FailureCapability{
			ID: string(cap.ID), BadgeOwned: cap.BadgeOwned, HMOwned: cap.HMOwned, Learned: cap.Learned, Usable: cap.Usable,
		})
	}
	for _, fact := range s.Progress {
		out.Progress = append(out.Progress, farm.FailureProgressFact{ID: string(fact.ID), Complete: fact.Complete, Value: fact.Value})
	}
	return out
}

func expectedStructuredGameOutcome(result agent.ObjectiveResult) bool {
	switch result.Cause {
	case "blacked_out", "catch_blackout", "train_retreat":
		return true
	default:
		return false
	}
}

// majorProgressAfter approximates Run's monotonic watchdog from the semantic
// state retained in results: badge/progression growth, party growth, or a new
// maximum level proves the earlier failure was not the run's final frontier.
func majorProgressAfter(outcomes []agent.ObjectiveResult, idx int) bool {
	if idx < 0 || idx >= len(outcomes) {
		return false
	}
	base := majorFailureMark(agent.FailureStateFor(outcomes[idx].Final))
	for _, later := range outcomes[idx+1:] {
		if base.improvedBy(majorFailureMark(agent.FailureStateFor(later.Final))) {
			return true
		}
	}
	return false
}

type failureMajorMark struct {
	badges        int
	party         int
	maxLevel      uint8
	completeFacts int
	progressValue int
}

func majorFailureMark(s agent.FailureState) failureMajorMark {
	m := failureMajorMark{badges: len(s.Badges), party: len(s.Party)}
	for _, mon := range s.Party {
		if mon.Level > m.maxLevel {
			m.maxLevel = mon.Level
		}
	}
	for _, fact := range s.Progress {
		if fact.Complete {
			m.completeFacts++
		}
		if fact.Value > 0 {
			m.progressValue += fact.Value
		}
	}
	return m
}

func (m failureMajorMark) improvedBy(n failureMajorMark) bool {
	return n.badges > m.badges || n.party > m.party || n.maxLevel > m.maxLevel ||
		n.completeFacts > m.completeFacts || n.progressValue > m.progressValue
}

func checkpointForRound(dir string, round int) string {
	if dir == "" || round <= 0 {
		return ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	prefix := "round-" + threeDigits(round) + "-"
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".state") {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return names[len(names)-1]
}

func threeDigits(n int) string {
	if n < 10 {
		return "00" + string(rune('0'+n))
	}
	if n < 100 {
		return "0" + intString(n)
	}
	return intString(n)
}

func intString(n int) string {
	// round numbers are bounded by Run's int budget and this helper is only
	// used for checkpoint filenames. Avoid fmt in the hot path.
	if n == 0 {
		return "0"
	}
	var buf [24]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func resetObjectiveFailureTelemetry() {
	objectiveFailureTelemetry.Lock()
	objectiveFailureTelemetry.result = nil
	objectiveFailureTelemetry.Unlock()
}
