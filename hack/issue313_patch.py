from pathlib import Path


def replace_once(text: str, old: str, new: str, label: str) -> str:
    if new in text:
        return text
    if old not in text:
        raise SystemExit(f"missing anchor for {label}")
    return text.replace(old, new, 1)


p = Path("cmd/pokewall/main.go")
text = p.read_text()
text = replace_once(
    text,
    'stateFile := flag.String("state", "", "if set, persist the tile map and queue here so a wall restart does not forget active runs")',
    'stateFile := flag.String("state", "", "if set, persist live tiles, queue, issue links and outbox here so a wall restart does not forget active runs")\n\tcatalogPath := flag.String("catalog", "", "if set, persist the queryable run history in SQLite; finished tiles then leave RAM")',
    "catalog flag",
)
text = replace_once(
    text,
    '\tif client != nil {',
    '\tif *catalogPath != "" {\n\t\tif err := wall.SetCatalogPath(*catalogPath); err != nil {\n\t\t\tlog.Fatalf("pokewall: open catalog %s: %v", *catalogPath, err)\n\t\t}\n\t\tdefer wall.CloseCatalog() //nolint:errcheck // process exit closes the file descriptor too\n\t\tgo wall.RunCatalogSettlementSweep(5 * time.Second)\n\t}\n\tif client != nil {',
    "catalog startup",
)
text = replace_once(
    text,
    'Handler:           runtimeOperatorHTTPHandler(wall),',
    'Handler:           wall.catalogHTTPHandler(runtimeOperatorHTTPHandler(wall)),',
    "catalog handler",
)
p.write_text(text)

p = Path("cmd/pokeui/main.go")
text = p.read_text()
text = replace_once(
    text,
    'mux.HandleFunc("GET /v1/stats", statsHandler(wallBase))',
    'mux.HandleFunc("GET /v1/stats", outcomesStatsHandler(wallBase))',
    "stats outcomes route",
)
p.write_text(text)

p = Path("cmd/pokewall/snapshot_perf.go")
text = p.read_text()
old = '''func (w *Wall) snapshotRun(runID string) (tileRow, bool) {
\trunID = strings.TrimSpace(runID)
\tif runID == "" {
\t\treturn tileRow{}, false
\t}
\tw.mu.Lock()
\tdefer w.mu.Unlock()
\tt := w.tiles[runID]
\tif t == nil {
\t\treturn tileRow{}, false
\t}
\treturn w.tileRowLocked(t), true
}'''
new = '''func (w *Wall) snapshotRun(runID string) (tileRow, bool) {
\trunID = strings.TrimSpace(runID)
\tif runID == "" {
\t\treturn tileRow{}, false
\t}
\tw.mu.Lock()
\tt := w.tiles[runID]
\tif t != nil {
\t\trow := w.tileRowLocked(t)
\t\tw.mu.Unlock()
\t\treturn row, true
\t}
\tw.mu.Unlock()
\treturn w.catalogSnapshotRun(runID)
}'''
text = replace_once(text, old, new, "snapshot catalog fallback")
p.write_text(text)

p = Path("cmd/pokewall/artifact_retention.go")
text = p.read_text()
old = '''\tif stateChanged {
\t\tw.saveState()
\t}
\treturn errors.Join(errs...)
}'''
new = '''\tif stateChanged {
\t\tw.saveState()
\t}
\tif err := w.expireCatalogArtifacts(now, maxAge); err != nil {
\t\terrs = append(errs, err)
\t}
\treturn errors.Join(errs...)
}'''
text = replace_once(text, old, new, "catalog artifact retention")
p.write_text(text)

p = Path("deploy/farm.yml")
text = p.read_text()
text = replace_once(
    text,
    '      - /var/lib/pokewall/state.json\n      - -issues-api',
    '      - /var/lib/pokewall/state.json\n      - -catalog\n      - /var/lib/pokewall/catalog.db\n      - -issues-api',
    "farm catalog flag",
)
p.write_text(text)

p = Path("go.mod")
text = p.read_text()
text = replace_once(
    text,
    '\tgithub.com/modelcontextprotocol/go-sdk v1.7.0\n)',
    '\tgithub.com/modelcontextprotocol/go-sdk v1.7.0\n\tmodernc.org/sqlite v1.58.0\n)',
    "sqlite module",
)
p.write_text(text)

p = Path("cmd/pokewall/catalog.go")
text = p.read_text()
text = replace_once(
    text,
    'io.WriteString(res, `\\{\\"runs\\":\\[`)',
    'io.WriteString(res, `{"runs":[`)',
    "outcomes JSON prefix",
)
p.write_text(text)

p = Path("docs/RUN_INSPECTOR.md")
text = p.read_text()
if "## Run catalog" not in text:
    text = text.rstrip() + '''

## Run catalog

Production PokéWall keeps finished run metadata in `/var/lib/pokewall/catalog.db`. SQLite is the query index; finish dumps/checkpoints remain artifacts with their normal retention policy. RAM and `state.json` keep only live runs plus any finished resume ancestors still required by an active child.
'''
    p.write_text(text)
