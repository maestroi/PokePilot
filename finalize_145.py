from pathlib import Path

p = Path("agent/objective_result_test.go")
s = p.read_text()
replacements = {
    "{OutcomePostconditionFailed, actionStop},": "{OutcomePostconditionFailed, actionReplan},",
    "if got := classifyObjectiveOutcome(o, replan, clean); got != OutcomeControllerUncertain {\n\t\tt.Fatalf(\"replan exhaustion = %q, want controller_uncertain\", got)\n\t}": "if got := classifyObjectiveOutcome(o, replan, clean); got != OutcomeBlocked {\n\t\tt.Fatalf(\"replan exhaustion = %q, want blocked\", got)\n\t}",
    "if got := classifyObjectiveOutcome(o, joinedController, clean); got != OutcomeControllerUncertain {\n\t\tt.Fatalf(\"controller + dirty boundary = %q, want controller_uncertain\", got)\n\t}": "if got := classifyObjectiveOutcome(o, joinedController, clean); got != OutcomeStabilizationFailed {\n\t\tt.Fatalf(\"controller + dirty boundary = %q, want stabilization_failed\", got)\n\t}",
    "if got := classifyObjectiveOutcome(o, skill.ErrMenuStuck, clean); got != OutcomeControllerUncertain {\n\t\tt.Fatalf(\"menu stuck = %q, want controller_uncertain\", got)\n\t}": "if got := classifyObjectiveOutcome(o, skill.ErrMenuStuck, clean); got != OutcomeBlocked {\n\t\tt.Fatalf(\"menu stuck = %q, want blocked\", got)\n\t}",
    "if got := classifyObjectiveOutcome(o, emu.ErrFrameDeadline, clean); got != OutcomeControllerUncertain {\n\t\tt.Fatalf(\"frame deadline = %q, want controller_uncertain\", got)\n\t}": "if got := classifyObjectiveOutcome(o, emu.ErrFrameDeadline, clean); got != OutcomeBlocked {\n\t\tt.Fatalf(\"frame deadline = %q, want blocked\", got)\n\t}",
}
for old, new in replacements.items():
    if old not in s:
        raise SystemExit(f"missing objective_result_test anchor: {old[:100]!r}")
    s = s.replace(old, new, 1)
p.write_text(s)
