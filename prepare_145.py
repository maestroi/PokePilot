from pathlib import Path

p = Path("implement_145.py")
lines = p.read_text().splitlines(keepends=True)
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
p.write_text("".join(lines))
