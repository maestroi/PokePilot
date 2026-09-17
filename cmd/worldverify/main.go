// Command worldverify audits the ROM-derived world model without playing a
// run. The verifier is game-agnostic; this command wires the currently
// supported Pokemon Red adapter into it. Additional game adapters can be added
// here without changing the verification algorithm.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	redrom "github.com/maestroi/pokepilot/red/rom"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/world"
	verifier "github.com/maestroi/pokepilot/worldverify"
)

func main() {
	romPath := flag.String("rom", os.Getenv("POKEMON_RED_ROM"), "path to ROM (defaults to POKEMON_RED_ROM)")
	game := flag.String("game", "red", "game adapter (currently: red)")
	jsonOutput := flag.Bool("json", false, "emit JSON report")
	strictWarnings := flag.Bool("strict-warnings", false, "exit non-zero when warnings are present")
	maxCaps := flag.Int("max-exhaustive-capabilities", 16, "maximum capabilities to enumerate exhaustively (16 = 65,536 states)")
	flag.Parse()

	if *game != "red" {
		fmt.Fprintf(os.Stderr, "worldverify: unsupported game adapter %q (currently: red)\n", *game)
		os.Exit(2)
	}
	if *romPath == "" {
		fmt.Fprintln(os.Stderr, "worldverify: -rom or POKEMON_RED_ROM is required")
		os.Exit(2)
	}
	romData, err := os.ReadFile(*romPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "worldverify: read ROM: %v\n", err)
		os.Exit(1)
	}
	if err := redrom.Verify(romData); err != nil {
		fmt.Fprintf(os.Stderr, "worldverify: verify ROM: %v\n", err)
		os.Exit(1)
	}

	graph, err := world.BuildGraph(romData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "worldverify: build graph: %v\n", err)
		os.Exit(1)
	}
	transitions := skill.RedRouteTransitionsForValidation(graph)
	// Pokemon Red's normal controllable overworld begins in Pallet Town (map
	// 0x00). The native id stays in this adapter wiring; worldverify itself sees
	// only the opaque portable map id emitted by ValidationSnapshot.
	snapshot := world.ValidationSnapshot(graph, transitions, 0x00)
	applyRedReachabilityManifest(&snapshot)
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
