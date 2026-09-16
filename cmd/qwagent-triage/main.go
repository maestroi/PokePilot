package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/maestroi/pokepilot/deploy"
)

var errNothing = errors.New("no actionable unclaimed triage group")

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if errors.Is(err, errNothing) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: qwagent-triage pick [--claimed title] [--repaired key] [--regressed key]...")
	}
	switch args[0] {
	case "pick":
		return pickCmd(args[1:], stdin, stdout)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func pickCmd(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("pick", flag.ContinueOnError)
	var claimed repeatFlags
	var repaired repeatFlags
	var regressed repeatFlags
	fs.Var(&claimed, "claimed", "open PR title already claiming a triage key")
	fs.Var(&repaired, "repaired", "triage key with a merged repair not present in the representative failing revision")
	fs.Var(&regressed, "regressed", "triage key recurring on a revision that contains its merged repair")
	if err := fs.Parse(args); err != nil {
		return err
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}
	var groups []deploy.TriageGroup
	if err := json.Unmarshal(raw, &groups); err != nil {
		return err
	}
	g, ok := deploy.PickWithLocalState(groups, claimed, repaired, regressed)
	if !ok {
		return errNothing
	}
	out := map[string]any{
		"key":         g.Key,
		"run_id":      g.RunID(),
		"example":     g.Example,
		"count":       g.Count,
		"fingerprint": g.Fingerprint,
	}
	if g.Issue != nil && g.Issue.IssueNumber > 0 {
		out["issue_number"] = g.Issue.IssueNumber
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

type repeatFlags []string

func (r *repeatFlags) String() string { return strings.Join(*r, ", ") }

func (r *repeatFlags) Set(v string) error {
	*r = append(*r, v)
	return nil
}
