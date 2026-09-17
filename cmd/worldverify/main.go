// Command worldverify audits a ROM-derived world model without playing a run.
// The verifier is game-agnostic; this command detects a supported game profile
// and wires its world adapter into the portable verification algorithm.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	blueprofile "github.com/maestroi/pokepilot/blue/profile"
	"github.com/maestroi/pokepilot/profiles"
	redprofile "github.com/maestroi/pokepilot/red/profile"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
	verifier "github.com/maestroi/pokepilot/worldverify"
)

const pokemonYellowENUSRev0SHA1 = "cc7d03262ebfaf2f06772c1a480c7d9d5f4a38e1"

func main() {
	romPath := flag.String("rom", defaultROMPath(), "path to ROM (defaults to POKEMON_ROM, then POKEMON_RED_ROM)")
	game := flag.String("game", "auto", "game adapter (auto, red, blue; yellow fingerprint is recognized but its world adapter is not implemented yet)")
	jsonOutput := flag.Bool("json", false, "emit JSON report")
	strictWarnings := flag.Bool("strict-warnings", false, "exit non-zero when warnings are present")
	maxCaps := flag.Int("max-exhaustive-capabilities", 16, "maximum capabilities to enumerate exhaustively (16 = 65,536 states)")
	flag.Parse()

	requestedGame, err := normalizeGameFlag(*game)
	if err != nil {
		fmt.Fprintf(os.Stderr, "worldverify: %v\n", err)
		os.Exit(2)
	}
	if *romPath == "" {
		fmt.Fprintln(os.Stderr, "worldverify: -rom or POKEMON_ROM is required")
		os.Exit(2)
	}
	romData, err := os.ReadFile(*romPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "worldverify: read ROM: %v\n", err)
		os.Exit(1)
	}

	profile, info, err := profiles.Detect(romData)
	if err != nil {
		if info.SHA1 == pokemonYellowENUSRev0SHA1 {
			fmt.Fprintf(os.Stderr, "worldverify: recognized pokemon-yellow@en-us-rev0 ROM (sha1 %s), but the Yellow world adapter is not implemented yet\n", info.SHA1)
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "worldverify: detect ROM: %v\n", err)
		os.Exit(1)
	}
	if requestedGame != "auto" && requestedGame != string(profile.ID()) {
		fmt.Fprintf(os.Stderr, "worldverify: -game %q does not match detected ROM profile %q\n", *game, profile.ID())
		os.Exit(2)
	}

	graph, err := world.BuildGraph(romData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "worldverify: build graph: %v\n", err)
		os.Exit(1)
	}

	var transitions map[world.Edge]structTransition
	_ = transitions
	// Red and Blue share the English Gen-I map ids, ROM map-table format and
	// progression topology. Their profile contract already pins that shared
	// engine; verification therefore intentionally uses one Gen-I semantic
	// transition catalog while keeping the detected game identity in the
	// portable report.
	switch profile.ID() {
	case redprofile.GameID, blueprofile.GameID:
	default:
		fmt.Fprintf(os.Stderr, "worldverify: no world adapter for detected profile %q\n", profile.ID())
		os.Exit(2)
	}
	redBlueTransitions := skill.RedRouteTransitionsForValidation(graph)
	// English Red/Blue both begin normal controllable play in Pallet Town
	// (native map 0x00). Native ids stay in this adapter wiring.
	snapshot := world.ValidationSnapshot(graph, redBlueTransitions, 0x00)
	snapshot.Game = string(profile.ID())
	applyGen1ReachabilityManifest(&snapshot)
	report := verifier.Verify(snapshot, verifier.Options{MaxExhaustiveCapabilities: *maxCaps})

	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "worldverify: encode report: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("%s world verification: %d maps, %d edges, %d components, %d capabilities\n",
			report.Game, report.Stats.Maps, report.Stats.Edges, report.Stats.Components, report.Stats.Capabilities)
		if report.Stats.CapabilityStatesChecked > 0 {
			mode := "bounded"
			if report.Stats.ExhaustiveCapabilities {
				mode = "exhaustive"
			}
			fmt.Printf("capability exploration: %d states (%s), full-set reachability %d maps / %d components\n",
				report.Stats.CapabilityStatesChecked, mode, report.Stats.FullReachableMaps, report.Stats.FullReachableComponents)
			fmt.Printf("reachability audit: %d unreachable (%d required, %d optional, %d expected-unused, %d story-state, %d suspicious)\n",
				report.Stats.FullUnreachableMaps,
				report.Stats.RequiredUnreachableMaps,
				report.Stats.OptionalUnreachableMaps,
				report.Stats.ExpectedUnreachableMaps,
				report.Stats.StoryStateDependentUnreachableMaps,
				report.Stats.SuspiciousUnreachableMaps)
			for _, unreachable := range report.UnreachableMaps {
				label := unreachable.Label
				if label == "" {
					label = "(unnamed)"
				}
				fmt.Printf("unreachable %-22s map=%s %-32s %s\n",
					unreachable.Class, unreachable.Map, label, unreachable.Reason)
			}
		}
		if report.Stats.InactiveStaticEdges > 0 || report.Stats.SemanticDeadPortEdges > 0 {
			fmt.Printf("geometry audit: %d inactive static edges, %d semantic dead-port edges\n",
				report.Stats.InactiveStaticEdges, report.Stats.SemanticDeadPortEdges)
		}
		for _, finding := range report.Findings {
			where := ""
			if finding.Map != "" {
				where = " map=" + string(finding.Map)
			}
			if finding.Edge != "" {
				where += " edge=" + finding.Edge
			}
			fmt.Printf("%s %-28s%s %s\n", finding.Severity, finding.Code, where, finding.Message)
		}
		fmt.Printf("result: %d errors, %d warnings\n", report.ErrorCount(), report.WarningCount())
	}

	if report.HasErrors() || (*strictWarnings && report.WarningCount() > 0) {
		os.Exit(1)
	}
}

func defaultROMPath() string {
	if path := os.Getenv("POKEMON_ROM"); path != "" {
		return path
	}
	return os.Getenv("POKEMON_RED_ROM")
}

func normalizeGameFlag(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return "auto", nil
	case "red", "pokemon-red":
		return string(redprofile.GameID), nil
	case "blue", "pokemon-blue":
		return string(blueprofile.GameID), nil
	case "yellow", "pokemon-yellow":
		return "pokemon-yellow", nil
	default:
		return "", fmt.Errorf("unsupported -game %q (supported: auto, red, blue; yellow adapter pending)", value)
	}
}

// structTransition is intentionally never instantiated; this declaration is
// only here to keep game-neutral adapter selection visually separate from the
// concrete Gen-I transition catalog below.
type structTransition = struct{}
