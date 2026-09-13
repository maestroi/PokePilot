package agent

import (
	"fmt"
	"sort"
	"strings"
)

// Drive is one reusable reason an objective may be valuable. Drives are
// deliberately game-agnostic: a play style changes their weights, while the
// offered objective menu remains the source of truth for what is legal.
type Drive string

const (
	DriveProgression Drive = "progression"
	DriveExploration Drive = "exploration"
	DriveParty       Drive = "party"
	DriveResources   Drive = "resources"
	DriveInteraction Drive = "interaction"
	DriveItems       Drive = "items"
	DriveTraining    Drive = "training"
	DrivePreparation Drive = "preparation"
	DriveSafety      Drive = "safety"
)

var driveOrder = []Drive{
	DriveProgression,
	DriveExploration,
	DriveParty,
	DriveResources,
	DriveInteraction,
	DriveItems,
	DriveTraining,
	DrivePreparation,
	DriveSafety,
}

// PlayStyleProfile is policy over shared planner drives. It does not add,
// remove, or bypass objectives; deterministic offering/progression continues
// to own legality and prerequisites.
type PlayStyleProfile struct {
	Name    string
	Weights map[Drive]float64
}

var speedrunProfile = PlayStyleProfile{
	Name: "speedrun",
	Weights: map[Drive]float64{
		DriveProgression: 1.00,
		DriveExploration: 0.08,
		DriveParty:       0.20,
		DriveResources:   0.18,
		DriveInteraction: 0.08,
		DriveItems:       0.10,
		DriveTraining:    0.25,
		DrivePreparation: 0.35,
		DriveSafety:      0.70,
	},
}

var adventureProfile = PlayStyleProfile{
	Name: "adventure",
	Weights: map[Drive]float64{
		DriveProgression: 1.00,
		DriveExploration: 0.72,
		DriveParty:       0.62,
		DriveResources:   0.48,
		DriveInteraction: 0.55,
		DriveItems:       0.58,
		DriveTraining:    0.46,
		DrivePreparation: 0.68,
		DriveSafety:      0.82,
	},
}

// PlayStyle returns a built-in profile. Unknown names deliberately fall back
// to speedrun so adding the feature cannot silently alter existing runs.
func PlayStyle(name string) PlayStyleProfile {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "adventure":
		return clonePlayStyle(adventureProfile)
	case "", "speedrun":
		fallthrough
	default:
		return clonePlayStyle(speedrunProfile)
	}
}

func clonePlayStyle(in PlayStyleProfile) PlayStyleProfile {
	out := PlayStyleProfile{Name: in.Name, Weights: make(map[Drive]float64, len(in.Weights))}
	for k, v := range in.Weights {
		out.Weights[k] = v
	}
	return out
}

// DriveScore is safe decision telemetry: it explains the explicit objective
// features and configured weights used by the runtime, not model reasoning.
type DriveScore struct {
	Total         float64
	Contributions map[Drive]float64
	Urgency       map[Drive]float64
	Weighted      map[Drive]float64
}

// ScoreObjective evaluates one already-legal objective against a play style.
// The numbers are intentionally coarse. They are planner hints, not another
// progression engine, and therefore never gate an objective.
func ScoreObjective(obs Observation, o Objective, profile PlayStyleProfile) DriveScore {
	contrib := objectiveDriveContributions(o)
	urgency := driveUrgency(obs)
	weighted := make(map[Drive]float64, len(contrib))
	total := 0.0
	for drive, value := range contrib {
		w := profile.Weights[drive]
		u := urgency[drive]
		if u == 0 {
			u = 1
		}
		part := value * w * u
		weighted[drive] = part
		total += part
	}
	return DriveScore{Total: total, Contributions: contrib, Urgency: urgency, Weighted: weighted}
}

func objectiveDriveContributions(o Objective) map[Drive]float64 {
	out := map[Drive]float64{}
	add := func(d Drive, v float64) { out[d] += v }

	switch o.Kind {
	case KindProgress:
		add(DriveProgression, 1.00)
	case KindGym:
		add(DriveProgression, 0.95)
		add(DrivePreparation, 0.35)
	case KindGoTo:
		add(DriveProgression, 0.35)
		if strings.Contains(strings.ToLower(o.Note), "unvisited adjacent map") {
			add(DriveExploration, 0.85)
		} else {
			add(DriveExploration, 0.10)
		}
	case KindTalk:
		add(DriveInteraction, 1.00)
		add(DriveExploration, 0.20)
	case KindTrainer:
		add(DriveTraining, 0.55)
		add(DrivePreparation, 0.20)
		add(DriveProgression, 0.15)
	case KindStarter:
		add(DriveParty, 1.00)
		add(DriveProgression, 0.45)
	case KindTrain:
		add(DriveTraining, 0.90)
		add(DriveParty, 0.60)
		add(DrivePreparation, 0.45)
	case KindHeal:
		add(DriveSafety, 1.00)
		add(DriveResources, 0.20)
	case KindCatch:
		add(DriveParty, 0.85)
		add(DriveExploration, 0.30)
	case KindBuy:
		add(DriveResources, 0.90)
		add(DrivePreparation, 0.20)
	case KindPickup:
		add(DriveItems, 1.00)
		add(DriveExploration, 0.20)
	case KindUseItem:
		add(DriveSafety, 0.80)
		add(DriveResources, 0.20)
	}
	return out
}

func driveUrgency(obs Observation) map[Drive]float64 {
	u := map[Drive]float64{}
	for _, d := range driveOrder {
		u[d] = 1
	}

	// Recovery should win naturally when the party is in danger, independent
	// of play style. This mirrors the existing deterministic retreat/heal
	// mechanics instead of replacing them.
	if partyHurt(obs) || leadOutOfPP(obs) {
		u[DriveSafety] = 1.60
		u[DriveResources] = 1.20
	}

	// A small party makes catches/team work more relevant without turning
	// every catch into a requirement.
	if obs.PartyCount > 0 && obs.PartyCount < 4 {
		u[DriveParty] = 1 + float64(4-obs.PartyCount)*0.12
	}

	// Repeated failures are evidence that tunnel vision is becoming less
	// useful. This is intentionally a mild nudge; #275 owns the bounded
	// stalled-progression fallback and its stronger escalation/reset rules.
	failures := 0
	for _, f := range obs.Failures {
		failures += f.Times
	}
	if failures > 0 {
		if failures > 4 {
			failures = 4
		}
		u[DriveExploration] += float64(failures) * 0.10
		u[DriveInteraction] += float64(failures) * 0.08
	}
	return u
}

// AnnotatePlayStyle adds compact, inspectable drive hints to the lines the LLM
// already sees. Speedrun is an exact no-op so existing prompts/runs stay
// unchanged until Adventure is explicitly selected.
func AnnotatePlayStyle(obs Observation, offered []Objective, profile PlayStyleProfile) []Objective {
	out := append([]Objective(nil), offered...)
	if profile.Name == "" || profile.Name == "speedrun" {
		return out
	}
	for i := range out {
		score := ScoreObjective(obs, out[i], profile)
		if len(score.Weighted) == 0 {
			continue
		}
		annotation := fmt.Sprintf("[%s %.2f: %s]", profile.Name, score.Total, dominantDrives(score, 3))
		if out[i].Note == "" {
			out[i].Note = annotation
		} else {
			out[i].Note += " " + annotation
		}
	}
	return out
}

func dominantDrives(score DriveScore, limit int) string {
	type ranked struct {
		drive Drive
		value float64
	}
	parts := make([]ranked, 0, len(score.Weighted))
	for d, v := range score.Weighted {
		if v > 0 {
			parts = append(parts, ranked{drive: d, value: v})
		}
	}
	sort.Slice(parts, func(i, j int) bool {
		if parts[i].value == parts[j].value {
			return parts[i].drive < parts[j].drive
		}
		return parts[i].value > parts[j].value
	})
	if limit > len(parts) {
		limit = len(parts)
	}
	labels := make([]string, 0, limit)
	for _, p := range parts[:limit] {
		labels = append(labels, string(p.drive))
	}
	return strings.Join(labels, "+")
}

// StyledLLMPlanner is the first opt-in Adventure seam. It preserves the
// existing LLM planner, strategist, retry, goal, usage, and telemetry behavior
// via embedding and only decorates the offered menu before model calls.
type StyledLLMPlanner struct {
	*LLMPlanner
	Profile PlayStyleProfile
}

func NewStyledLLMPlanner(inner *LLMPlanner, style string) *StyledLLMPlanner {
	if inner == nil {
		inner = NewLLMPlanner()
	}
	return &StyledLLMPlanner{LLMPlanner: inner, Profile: PlayStyle(style)}
}

func NewAdventureLLMPlanner(inner *LLMPlanner) *StyledLLMPlanner {
	return NewStyledLLMPlanner(inner, "adventure")
}

func (p *StyledLLMPlanner) Next(obs Observation, offered []Objective) (Objective, error) {
	return p.LLMPlanner.Next(obs, AnnotatePlayStyle(obs, offered, p.Profile))
}

func (p *StyledLLMPlanner) NextRetry(obs Observation, offered []Objective, r Retry) (Objective, error) {
	return p.LLMPlanner.NextRetry(obs, AnnotatePlayStyle(obs, offered, p.Profile), r)
}

func (p *StyledLLMPlanner) Strategize(obs Observation, offered []Objective, reason string) (Plan, error) {
	return p.LLMPlanner.Strategize(obs, AnnotatePlayStyle(obs, offered, p.Profile), reason)
}

func (p *StyledLLMPlanner) StrategizeRetry(obs Observation, offered []Objective, reason string, r Retry) (Plan, error) {
	return p.LLMPlanner.StrategizeRetry(obs, AnnotatePlayStyle(obs, offered, p.Profile), reason, r)
}
