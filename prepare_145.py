from pathlib import Path

p = Path("implement_145.py")
lines = p.read_text().splitlines(keepends=True)

# Remove the duplicate failure_identity cleanup transform; the preceding
# transform already moved objective_boundary_dirty to the dominating slot.
starts = [i for i, line in enumerate(lines) if line.startswith('replace_once("agent/failure_identity.go",')]
if len(starts) < 2:
    raise SystemExit(f"expected two failure_identity transforms, found {len(starts)}")
start = starts[1]
end = None
for i in range(start + 1, len(lines)):
    if lines[i].strip() == "''' ''')":
        end = i
        break
if end is None:
    raise SystemExit("could not find end of duplicate failure_identity transform")
del lines[start:end + 1]

# Generated Go source needs actual tabs, not the two characters backslash+t.
s = "".join(lines)
s = s.replace("new_error_block = r'''", "new_error_block = '''", 1)
s = s.replace("r'''func drainObjectiveFailureTelemetry", "'''func drainObjectiveFailureTelemetry", 1)

# replace_between retains its end marker; do not also emit that next function
# header inside the replacement body.
needle = "func (r ObjectiveResult) HistoryText() string {\n''')"
pos = s.find(needle)
if pos < 0:
    raise SystemExit("missing duplicate HistoryText replacement header")
s = s[:pos] + "''')" + s[pos + len(needle):]
needle = "func farmIdentityFromAgent(result agent.ObjectiveResult) farm.FailureIdentity {\n''')"
pos = s.find(needle)
if pos < 0:
    raise SystemExit("missing duplicate farmIdentity replacement header")
s = s[:pos] + "''')" + s[pos + len(needle):]

p.write_text(s)
