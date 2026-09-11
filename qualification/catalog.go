// Package qualification defines the private ROM-backed validation plan.
//
// The catalog is data, not gameplay policy. Public CI remains ROM-free; a
// local/self-hosted runner selects cases from this catalog after verifying the
// supplied ROM. Red-specific mechanics are executed by cmd/pokequal at the
// validation boundary, never by the generic agent/runtime layers.
package qualification

import (
	"fmt"
	"sort"
	"strings"
)

// Layer is one level of the qualification pyramid from docs/ARCHITECTURE.md.
type Layer string

const (
	LayerSkill     Layer = "skill"
	LayerMilestone Layer = "milestone"
	LayerFull      Layer = "full"
)

// Runner tells pokequal how a case is executed.
type Runner string

const (
	RunnerGoTest   Runner = "go-test"
	RunnerRedSkill Runner = "red-skill"
	RunnerFullRun  Runner = "full-run"
)

// Expectation is a positive semantic postcondition checked after a direct
// checkpoint scenario. Go-test cases own their assertions inside the test.
type Expectation struct {
	Kind  string `json:"kind,omitempty"`
	Value string `json:"value,omitempty"`
}

// Case is one qualification barrier. Checkpoint is either a generated fixture
// label ("fixture:<name>") or a path relative to the private corpus root. The
// commercial ROM itself is never a checkpoint or artifact.
type Case struct {
	ID          string      `json:"id"`
	Description string      `json:"description"`
	Layer       Layer       `json:"layer"`
	Runner      Runner      `json:"runner"`
	Package     string      `json:"package,omitempty"`
	Test        string      `json:"test,omitempty"`
	Short       bool        `json:"short,omitempty"`
	Checkpoint  string      `json:"checkpoint,omitempty"`
	Action      string      `json:"action,omitempty"`
	Expect      Expectation `json:"expect,omitempty"`
	Available   bool        `json:"available"`
	BlockedBy   int         `json:"blocked_by_issue,omitempty"`
}

// Catalog returns the complete Red qualification roadmap. Cases whose
// progression slice has not landed stay visible but unavailable; that makes the
// qualification matrix grow by flipping the case live when its slice lands,
// rather than inventing a second roadmap in CI.
func Catalog() []Case {
	return []Case{
		{
			ID:          "rom-short",
			Description: "focused ROM-backed short suite across the repository",
			Layer:       LayerSkill,
			Runner:      RunnerGoTest,
			Package:     "./...",
			Short:       true,
			Available:   true,
		},
		{
			ID:          "opening-brock",
			Description: "north Viridian Forest checkpoint through Brock and the Boulder Badge",
			Layer:       LayerMilestone,
			Runner:      RunnerGoTest,
			Package:     "./skill",
			Test:        "^TestGymBoulderBadge$",
			Checkpoint:  "fixture:forest_north_gate",
			Available:   true,
		},
		{
			ID:          "mt-moon-cerulean",
			Description: "Mt. Moon B2F checkpoint through the Super Nerd's fossil gate to Cerulean",
			Layer:       LayerMilestone,
			Runner:      RunnerGoTest,
			Package:     "./skill",
			Test:        "^TestMtMoonFossilOpensTheEasternExit$",
			Checkpoint:  "fixture:mt_moon_b2f",
			Available:   true,
		},
		{
			ID:          "misty",
			Description: "post-Brock checkpoint through Route 3/4, Mt. Moon and the Cascade Badge",
			Layer:       LayerMilestone,
			Runner:      RunnerGoTest,
			Package:     "./skill",
			Test:        "^TestGymCascadeBadge$",
			Checkpoint:  "fixture:post_boulder",
			Available:   true,
		},
		{
			ID:          "rocket-hideout",
			Description: "preserved Celadon checkpoint through Giovanni and Silph Scope acquisition",
			Layer:       LayerMilestone,
			Runner:      RunnerRedSkill,
			Checkpoint:  "rocket-hideout/start.state",
			Action:      "rocket-hideout",
			Expect:      Expectation{Kind: "item", Value: "silph scope"},
			Available:   true,
		},
		{
			ID:          "pokemon-tower",
			Description: "preserved post-Hideout checkpoint through Mr. Fuji and Poké Flute acquisition",
			Layer:       LayerMilestone,
			Runner:      RunnerRedSkill,
			Checkpoint:  "pokemon-tower/start.state",
			Action:      "pokemon-tower",
			Expect:      Expectation{Kind: "item", Value: "poke flute"},
			Available:   true,
		},
		{
			ID:          "fuchsia-koga-surf-strength",
			Description: "Poké Flute checkpoint through Fuchsia, Koga, Surf and Strength",
			Layer:       LayerMilestone,
			Runner:      RunnerRedSkill,
			Checkpoint:  "fuchsia-koga-surf-strength/start.state",
			Available:   false,
			BlockedBy:   33,
		},
		{
			ID:          "silph-sabrina",
			Description: "post-Fuchsia checkpoint through Saffron, Silph Co and Sabrina",
			Layer:       LayerMilestone,
			Runner:      RunnerRedSkill,
			Checkpoint:  "silph-sabrina/start.state",
			Available:   false,
			BlockedBy:   34,
		},
		{
			ID:          "cinnabar-blaine",
			Description: "post-Saffron checkpoint through Cinnabar, Secret Key and Blaine",
			Layer:       LayerMilestone,
			Runner:      RunnerRedSkill,
			Checkpoint:  "cinnabar-blaine/start.state",
			Available:   false,
			BlockedBy:   35,
		},
		{
			ID:          "viridian-giovanni",
			Description: "post-Cinnabar checkpoint through Viridian Gym and Giovanni",
			Layer:       LayerMilestone,
			Runner:      RunnerRedSkill,
			Checkpoint:  "viridian-giovanni/start.state",
			Available:   false,
			BlockedBy:   36,
		},
		{
			ID:          "victory-road-indigo",
			Description: "eight-badge checkpoint through Route 23, Victory Road and Indigo Plateau",
			Layer:       LayerMilestone,
			Runner:      RunnerRedSkill,
			Checkpoint:  "victory-road-indigo/start.state",
			Available:   false,
			BlockedBy:   37,
		},
		{
			ID:          "elite-four-champion",
			Description: "Indigo Plateau checkpoint through Elite Four, Champion and Hall of Fame completion",
			Layer:       LayerMilestone,
			Runner:      RunnerGoTest,
			Package:     "./skill",
			Test:        "^TestEliteFourProgressionQualification$",
			Checkpoint:  "elite-four-champion/start.state",
			Available:   true,
		},
		{
			ID:          "fresh-hall-of-fame",
			Description: "fresh-save unattended eight-badge campaign through Hall of Fame",
			Layer:       LayerFull,
			Runner:      RunnerFullRun,
			Available:   true,
		},
	}
}

// Select chooses runnable cases for profile. A specifically requested case is
// validated even if it is not part of the profile, which keeps local replay
// commands terse. Pending milestones fail loudly instead of being reported as
// green skips.
func Select(profile, only string) ([]Case, error) {
	profile = strings.ToLower(strings.TrimSpace(profile))
	if profile == "" {
		profile = "milestones"
	}
	if only != "" {
		for _, c := range Catalog() {
			if c.ID != only {
				continue
			}
			if !c.Available {
				return nil, fmt.Errorf("qualification: case %q is not available yet (blocked by #%d)", c.ID, c.BlockedBy)
			}
			return []Case{c}, nil
		}
		return nil, fmt.Errorf("qualification: unknown case %q", only)
	}

	var layers map[Layer]bool
	switch profile {
	case "skills":
		layers = map[Layer]bool{LayerSkill: true}
	case "milestones":
		layers = map[Layer]bool{LayerMilestone: true}
	case "full":
		layers = map[Layer]bool{LayerFull: true}
	case "all":
		layers = map[Layer]bool{LayerSkill: true, LayerMilestone: true, LayerFull: true}
	default:
		return nil, fmt.Errorf("qualification: unknown profile %q (want skills, milestones, full or all)", profile)
	}

	var out []Case
	for _, c := range Catalog() {
		if c.Available && layers[c.Layer] {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("qualification: profile %q has no runnable cases", profile)
	}
	return out, nil
}

// IDs returns catalog IDs in stable sorted order for CLI/list output.
func IDs() []string {
	cases := Catalog()
	ids := make([]string, 0, len(cases))
	for _, c := range cases {
		ids = append(ids, c.ID)
	}
	sort.Strings(ids)
	return ids
}
