from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    s = p.read_text()
    if old not in s:
        raise SystemExit(f"missing anchor in {path}: {old[:120]!r}")
    if s.count(old) != 1:
        raise SystemExit(f"non-unique anchor in {path}: {s.count(old)}")
    p.write_text(s.replace(old, new, 1))


# The failure escalation key now contains the semantic world-state fingerprint.
# Do not forget it merely because some unrelated objective returned success:
# only a relevant state change should make the same fault a fresh attempt.
replace_once(
    "agent/run.go",
    '''\t\tconsecFailures = 0\n\t\tfailureEscalated = map[string]bool{}\n\t\tlastFailKey = ""\n''',
    '''\t\tconsecFailures = 0\n\t\t// failureEscalated intentionally survives successful objectives. Its\n\t\t// keys include the semantic world-state fingerprint, so a material\n\t\t// change naturally creates a fresh key while fail -> harmless success\n\t\t// -> same fail cannot reset the strategic recovery budget forever.\n\t\tlastFailKey = ""\n''',
)

# Preserve a replayable checkpoint reference for both production fault classes
# fixed here. Public CI cannot ship the private save states, but the structured
# occurrence must deterministically point at the pre-objective checkpoint that
# farm/pokequal can replay on a ROM-backed runner.
p = Path("cmd/pokepilot/failure_telemetry_test.go")
s = p.read_text()
s = s.replace('import (\n\t"testing"\n', 'import (\n\t"os"\n\t"path/filepath"\n\t"testing"\n', 1)
s += r'''

func TestRecoverableProductionFaultsKeepReplayCheckpoints(t *testing.T) {
    for _, tc := range []struct {
        name      string
        cause     agent.FailureCauseID
        objective agent.Objective
    }{
        {
            name:      "navigation stall",
            cause:     "navigation_stalled",
            objective: agent.Objective{Kind: agent.KindGoTo, Place: "mt moon b1f", Flee: true},
        },
        {
            name:      "shop timeout",
            cause:     "shop_menu_timeout",
            objective: agent.Objective{Kind: agent.KindBuy, Item: "pokeball", Qty: 3},
        },
    } {
        t.Run(tc.name, func(t *testing.T) {
            resetObjectiveFailureTelemetry()
            t.Cleanup(resetObjectiveFailureTelemetry)

            dir := t.TempDir()
            checkpoint := "round-001-frame-0000001234-recovery.state"
            if err := os.WriteFile(filepath.Join(dir, checkpoint), []byte("replay-state-placeholder"), 0o600); err != nil {
                t.Fatal(err)
            }

            failure := structuredFailureResult(tc.cause, 10)
            failure.Objective = tc.objective
            failure.Recovered = true
            captureObjectiveFailureTelemetry(agent.Result{Outcomes: []agent.ObjectiveResult{failure}})

            got, terminal := drainObjectiveFailureTelemetry("budget", "build-a", dir)
            if terminal != nil {
                t.Fatalf("recoverable fault produced terminal occurrence: %+v", terminal)
            }
            if len(got) != 1 {
                t.Fatalf("failures = %+v, want one", got)
            }
            if got[0].Checkpoint != checkpoint {
                t.Fatalf("checkpoint = %q, want %q", got[0].Checkpoint, checkpoint)
            }
            if got[0].RecoveredCount != 1 || got[0].TerminalCount != 0 {
                t.Fatalf("impact = recovered %d terminal %d", got[0].RecoveredCount, got[0].TerminalCount)
            }
        })
    }
}
'''
p.write_text(s)
