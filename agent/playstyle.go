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

const (
	PlayStyleSpeedrun      = "speedrun"
	PlayStyleAdventure     = "adventure"
	PlayStyleCompletionist = "completionist"
	PlayStyleTeamBuilder   = "team_builder"
)

// PlayStyleProfile is data-only policy over the shared planner. Besides drive
// weights it controls how tolerant the mode is of detours and how strongly the
// common natural-play layer values exploration/optional content versus party
// development. Adding a profile should be a new value here, not a new planner.
type PlayStyleProfile struct {
	Name             string
	Weights          map[Drive]float64
	DetourPenalty    float64
	NaturalPlayScale float64
	ExplorationScale float64
	PartyScale       float64
}

var speedrunProfile = PlayStyleProfile{
	Name:             PlayStyleSpeedrun,
	DetourPenalty:    1.00,
	NaturalPlayScale: 0,
	ExplorationScale: 0.25,
	PartyScale:       0.40,
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
	Name:             PlayStyleAdventure,
	DetourPenalty:    0.70,
	NaturalPlayScale: 1.00,
	ExplorationScale: 1.00,
	PartyScale:       1.00,
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

var completionistProfile = PlayStyleProfile{
	Name:             PlayStyleCompletionist,
	DetourPenalty:    0.42,
	NaturalPlayScale: 1.20,
	ExplorationScale: 1.45,
	PartyScale:       0.90,
	Weights: map[Drive]float64{
		DriveProgression: 0.72,
		DriveExploration: 1.00,
		DriveParty:       0.68,
		DriveResources:   0.52,
		DriveInteraction: 0.95,
		DriveItems:       1.00,
		DriveTraining:    0.58,
		DrivePreparation: 0.72,
		DriveSafety:      0.82,
	},
}

var teamBuilderProfile = PlayStyleProfile{
	Name:             PlayStyleTeamBuilder,
	DetourPenalty:    0.65,
	NaturalPlayScale: 1.05,
	ExplorationScale: 0.55,
	PartyScale:       1.55,
	Weights: map[Drive]float64{
		DriveProgression: 0.82,
		DriveExploration: 0.32,
		DriveParty:       1.00,
		DriveResources:   0.55,
		DriveInteraction: 0.25,
		DriveItems:       0.72,
		DriveTraining:    1.00,
		DrivePreparation: 1.00,
		DriveSafety:      0.85,
	},
}

// NormalizePlayStyle returns the stable wire name. Empty and unknown values
// deliberately resolve to Speedrun so old serialized specs retain the exact
// pre-play-style behavior. Product surfaces may choose Adventure as their
// default by writing it explicitly.
func NormalizePlayStyle(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case PlayStyleAdventure:
		return PlayStyleAdventure
	case PlayStyleCompletionist:
		return PlayStyleCompletionist
	case PlayStyleTeamBuilder, "team-builder", "teambuilder":
		return PlayStyleTeamBuilder
	case "", PlayStyleSpeedrun:
		fallthrough
	default:
		return PlayStyleSpeedrun
	}
}

// PlayStyle returns a cloned built-in profile so callers cannot mutate global
// policy. Unknown/empty values retain the backwards-compatible Speedrun mode.
func PlayStyle(name string) PlayStyleProfile {
	switch NormalizePlayStyle(name) {
	case PlayStyleAdventure:
		return clonePlayStyle(adventureProfile)
	case PlayStyleCompletionist:
		return clonePlayStyle(completionistProfile)
	case PlayStyleTeamBuilder:
		return clonePlayStyle(teamBuilderProfile)
	default:
		return clonePlayStyle(speedrunProfile)
	}
}

func clonePlayStyle(in PlayStyleProfile) PlayStyleProfile {
	out := in
	out.Weights = make(map[Drive]float64, len(in.Weights))
	for k, v := range in.Weights {
		out.Weights[k] = v
	}
	return out
}

// DriveScore is safe decision telemetry: it explains the explicit objective
// features, configured weights, natural-play context and opportunity cost used
// by the runtime, not model reasoning. Value is the pre-cost value after the
// natural-play adjustment; Total is the net value after Cost.Total.
type DriveScore struct {
	Value         float64
	Total         float64
	Contributions map[Drive]float64
	Urgency       map[Drive]float64
	Weighted      map[Drive]float64
	Natural       NaturalPlaySignal
	Cost          OpportunityCost
}

// ScoreObjective evaluates one already-legal objective against a play style.
// The numbers are intentionally coarse. They are planner hints, not another
// progression engine, and therefore never gate an objective.
func ScoreObjective(obs Observation, o Objective, profile PlayStyleProfile) DriveScore {
	contrib := objectiveDriveContributions(o)
	urgency := driveUrgency(obs)
	weighted := make(map[Drive]float64, len(contrib))
	value := 0.0
	for drive, contribution := range contrib {
		w := profile.Weights[drive]
		u := urgency[drive]
		if u == 0 {
			u = 1
		}
		part := contribution * w * u
		weighted[drive] = part
		value += part
	}
	natural := naturalPlaySignal(obs, o, profile)
	natural = mergeNaturalPlaySignal(natural, teamBuilderRosterSignal(obs, o, profile))
	value += natural.Net()
	cost := opportunityCost(obs, o, profile)
	return DriveScore{
		Value:         value,
		Total:         value - cost.Total,
		Contributions: contrib,
		Urgency:       urgency,
		Weighted:      weighted,
		Natural:       natural,
		Cost:          cost,
	}
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
		if highImpactItem(o.Item) {
			add(DriveItems, 0.45)
			add(DrivePreparation, 0.55)
		}
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
	// useful. This is intentionally a mild nudge; the bounded stalled-
	// progression fallback owns the stronger escalation/reset rules.
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
// already sees. Speedrun is an exact no-op so old runs stay byte-for-byte
// compatible until a non-Speedrun profile is explicitly selected.
func AnnotatePlayStyle(obs Observation, offered []Objective, profile PlayStyleProfile) []Objective {
	out := filterRepelForPlayStyle(obs, offered, profile)
	if profile.Name == "" || profile.Name == PlayStyleSpeedrun {
		return out
	}
	for i := range out {
		score := ScoreObjective(obs, out[i], profile)
		if len(score.Weighted) == 0 {
			continue
		}
		parts := []string{dominantDrives(score, 3), opportunityCostSummary(score.Cost)}
		if natural := naturalPlaySummary(score.Natural); natural != "" {
			parts = append(parts, natural)
		}
		annotation := fmt.Sprintf("[%s %.2f: %s]", profile.Name, score.Total, strings.Join(parts, "; "))
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

// StyledLLMPlanner is the small standalone seam used by local/tests. Farm runs
// use the same AnnotatePlayStyle function in statsPlanner so failover and
// telemetry stay outside the policy layer.
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
	return NewStyledLLMPlanner(inner, PlayStyleAdventure)
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
