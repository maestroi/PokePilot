from pathlib import Path
p = Path('implement_145.py')
s = p.read_text()
old = '''replace_once("agent/failure_identity.go",\n'''\\tif errors.Is(err, ErrObjectiveBoundaryDirty) {\n\\t\\treturn "objective_boundary_dirty", nil\n\\t}\n''',\n''' ''')\n'''
if old not in s:
    raise SystemExit('missing duplicate-boundary removal block')
p.write_text(s.replace(old, '', 1))
