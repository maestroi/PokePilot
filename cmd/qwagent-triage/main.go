package main

import (
	"bytes"
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
		return fmt.Errorf("usage: qwagent-triage pick|pick-own-pr|classify-repairs|fetch-triage|fetch-debug|investigate ...")
	}
	switch args[0] {
	case "pick":
		return pickCmd(args[1:], stdin, stdout)
	case "pick-own-pr":
		return pickOwnPRCmd(args[1:], stdin, stdout)
	case "classify-repairs":
		return classifyRepairsCmd(args[1:], stdin, stdout)
	case "fetch-triage":
		return fetchTriageCmd(args[1:], stdout)
	case "fetch-debug":
		return fetchDebugCmd(args[1:], stdout)
	case "investigate":
		return investigateCmd(args[1:], stdout)
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
	groups, err := deploy.DecodeTriageGroups(raw)
	if err != nil {
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

func pickOwnPRCmd(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("pick-own-pr", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}
	prs, err := deploy.DecodePullRequests(raw)
	if err != nil {
		return err
	}
	pr, ok := deploy.PickOwnPRFailure(prs)
	if !ok {
		return errNothing
	}
	out := map[string]any{
		"mode":           "repair_pr",
		"key":            deploy.TriageKeyFromTitle(pr.Title),
		"pr_number":      pr.Number,
		"head_ref":       pr.HeadRef,
		"pr_url":         pr.URL,
		"failing_checks": pr.FailingChecks(),
		"example":        "CI failed: " + pr.FailingChecks(),
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func classifyRepairsCmd(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("classify-repairs", flag.ContinueOnError)
	repo := fs.String("repo", "", "git repository used for ancestry checks")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*repo) == "" {
		return fmt.Errorf("usage: qwagent-triage classify-repairs --repo PATH")
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}
	var rows []deploy.RepairObservation
	if err := json.Unmarshal(raw, &rows); err != nil {
		return err
	}
	repaired, regressed := deploy.ClassifyRepairs(*repo, rows)
	enc := json.NewEncoder(stdout)
	return enc.Encode(map[string]any{
		"repaired":  repaired,
		"regressed": regressed,
	})
}

func fetchTriageCmd(args []string, stdout io.Writer) error {
	endpoint, token, err := parseMCPFlags("fetch-triage", args)
	if err != nil {
		return err
	}
	raw, err := deploy.CallMCPTool(nil, endpoint, token, "pokepilot_get_triage", map[string]any{
		"include_resolved": true,
	})
	if err != nil {
		return err
	}
	groups, err := deploy.DecodeTriageGroups(raw)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(stdout)
	return enc.Encode(groups)
}

func fetchDebugCmd(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("fetch-debug", flag.ContinueOnError)
	runID := fs.String("run-id", "", "run id whose debug bundle to fetch")
	endpoint, token, err := parseMCPFlagsWith(fs, args)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(*runID)
	if id == "" && fs.NArg() > 0 {
		id = strings.TrimSpace(fs.Arg(0))
	}
	if id == "" {
		return fmt.Errorf("usage: qwagent-triage fetch-debug --run-id ID")
	}
	raw, err := deploy.CallMCPTool(nil, endpoint, token, "pokepilot_get_run_debug", map[string]any{
		"run_id": id,
	})
	if err != nil {
		return err
	}
	_, err = stdout.Write(append(bytes.TrimSpace(raw), '\n'))
	return err
}

func investigateCmd(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("investigate", flag.ContinueOnError)
	keyFlag := fs.String("key", "", "triage key to claim")
	endpoint, token, err := parseMCPFlagsWith(fs, args)
	if err != nil {
		return err
	}
	key := strings.TrimSpace(*keyFlag)
	if key == "" && fs.NArg() > 0 {
		key = strings.TrimSpace(fs.Arg(0))
	}
	if key == "" {
		return fmt.Errorf("usage: qwagent-triage investigate --key KEY")
	}
	raw, err := deploy.CallMCPTool(nil, endpoint, token, "pokepilot_investigate_failure", map[string]any{
		"key": key,
	})
	if err != nil {
		return err
	}
	_, err = stdout.Write(append(bytes.TrimSpace(raw), '\n'))
	return err
}

func parseMCPFlags(name string, args []string) (endpoint, token string, err error) {
	return parseMCPFlagsWith(flag.NewFlagSet(name, flag.ContinueOnError), args)
}

func parseMCPFlagsWith(fs *flag.FlagSet, args []string) (endpoint, token string, err error) {
	endpointFlag := fs.String("endpoint", strings.TrimSpace(os.Getenv("POKEPILOT_MCP_URL")), "MCP URL")
	tokenFlag := fs.String("token", strings.TrimSpace(os.Getenv("POKEPILOT_MCP_TOKEN")), "MCP bearer token")
	if err := fs.Parse(args); err != nil {
		return "", "", err
	}
	endpoint = strings.TrimSpace(*endpointFlag)
	if endpoint == "" {
		endpoint = deploy.DefaultMCPEndpoint()
	}
	token = strings.TrimSpace(*tokenFlag)
	if token == "" {
		return "", "", fmt.Errorf("POKEPILOT_MCP_TOKEN is required")
	}
	return endpoint, token, nil
}

type repeatFlags []string

func (r *repeatFlags) String() string { return strings.Join(*r, ", ") }

func (r *repeatFlags) Set(v string) error {
	*r = append(*r, v)
	return nil
}
