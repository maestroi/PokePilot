from pathlib import Path


def replace(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    if old not in text:
        raise SystemExit(f"expected text not found in {path}: {old[:160]!r}")
    p.write_text(text.replace(old, new, 1))


replace(
    "cmd/pokewall/main.go",
    '\tstateFile := flag.String("state", "", "if set, persist the tile map and queue here so a wall restart does not forget active runs")\n\tartifactRetention :=',
    '\tstateFile := flag.String("state", "", "if set, persist live tiles, queue, issue links and outbox here so a wall restart does not forget active runs")\n\tcatalogPath := flag.String("catalog", "", "if set, persist the queryable run history in SQLite; finished tiles then leave RAM")\n\tartifactRetention :=',
)

replace(
    "cmd/pokewall/main.go",
    '\tif client != nil {\n',
    '\tif *catalogPath != "" {\n\t\tif err := wall.SetCatalogPath(*catalogPath); err != nil {\n\t\t\tlog.Fatalf("pokewall: open catalog %s: %v", *catalogPath, err)\n\t\t}\n\t\tdefer wall.CloseCatalog() //nolint:errcheck // process exit closes the file descriptor too\n\t\tgo wall.RunCatalogSettlementSweep(5 * time.Second)\n\t}\n\tif client != nil {\n',
)

replace(
    "cmd/pokewall/main.go",
    '\t\tHandler:           runtimeOperatorHTTPHandler(wall),\n',
    '\t\tHandler:           wall.catalogHTTPHandler(runtimeOperatorHTTPHandler(wall)),\n',
)

replace(
    "cmd/pokeui/main.go",
    '\tmux.HandleFunc("GET /v1/stats", statsHandler(wallBase))\n',
    '\tmux.HandleFunc("GET /v1/stats", outcomesStatsHandler(wallBase))\n',
)

replace(
    "cmd/pokewall/snapshot_perf.go",
    '''func (w *Wall) snapshotRun(runID string) (tileRow, bool) {
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
}
''',
    '''func (w *Wall) snapshotRun(runID string) (tileRow, bool) {
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
}
''',
)

replace(
    "cmd/pokewall/artifact_retention.go",
    '''\tif stateChanged {
\t\tw.saveState()
\t}
\treturn errors.Join(errs...)
}
''',
    '''\tif stateChanged {
\t\tw.saveState()
\t}
\tif err := w.expireCatalogArtifacts(now, maxAge); err != nil {
\t\terrs = append(errs, err)
\t}
\treturn errors.Join(errs...)
}
''',
)

replace(
    "deploy/farm.yml",
    '''      - -state
      - /var/lib/pokewall/state.json
      - -issues-api
''',
    '''      - -state
      - /var/lib/pokewall/state.json
      - -catalog
      - /var/lib/pokewall/catalog.db
      - -issues-api
''',
)

replace(
    "go.mod",
    '''\tgithub.com/modelcontextprotocol/go-sdk v1.7.0
)''',
    '''\tgithub.com/modelcontextprotocol/go-sdk v1.7.0
\tmodernc.org/sqlite v1.58.0
)''',
)

replace(
    "cmd/pokewall/catalog.go",
    r'_, _ = io.WriteString(res, `{\"runs\":[`)',
    '_, _ = io.WriteString(res, `{"runs":[`)',
)

p = Path("docs/RUN_INSPECTOR.md")
text = p.read_text()
note = '''\n## Run catalog\n\nProduction PokéWall keeps finished run metadata in `/var/lib/pokewall/catalog.db`. SQLite is the query index; finish dumps/checkpoints remain artifacts with their normal retention policy. RAM and `state.json` keep only live runs plus any finished resume ancestors still required by an active child.\n'''
if "## Run catalog" not in text:
    p.write_text(text.rstrip() + note + "\n")
