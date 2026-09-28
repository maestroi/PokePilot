// Command boxxlebench runs the deterministic Boxxle (Sokoban) solver over
// the checked-in fixture set and prints the benchmark report.
//
// It is ROM-free: the fixtures are embedded in boxxle/solver, so the
// benchmark runs without a Boxxle cartridge. Model planners plug in through
// the bench.Proposer seam; this command establishes the solver reference
// baseline that their reports are compared against.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/maestroi/pokepilot/boxxle/bench"
	"github.com/maestroi/pokepilot/boxxle/solver"
)

func main() {
	jsonOut := flag.Bool("json", false, "print the report as JSON")
	flag.Parse()

	puzzles, err := solver.Fixtures()
	if err != nil {
		fmt.Fprintln(os.Stderr, "boxxlebench:", err)
		os.Exit(1)
	}

	var runs []bench.Run
	reference := map[string]int{}
	for _, p := range puzzles {
		run := bench.ReferenceRun(p)
		runs = append(runs, run)
		if run.Solved {
			reference[p.Name] = run.Pushes
		}
	}

	report := bench.Aggregate("solver", runs, reference)
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintln(os.Stderr, "boxxlebench:", err)
			os.Exit(1)
		}
		return
	}
	fmt.Print(bench.CompareText(report, report))
}
