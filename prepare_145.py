from pathlib import Path

p = Path("implement_145.py")
s = p.read_text()
old = r'''replace_once("agent/failure_identity.go",
''' + "'''" + r'''	if errors.Is(err, ErrObjectiveBoundaryDirty) {
		return "objective_boundary_dirty", nil
	}
''' + "'''" + r''',
''' + "''' ''')" + "\n"
if old not in s:
    raise SystemExit("missing duplicate-boundary removal block")
p.write_text(s.replace(old, "", 1))
