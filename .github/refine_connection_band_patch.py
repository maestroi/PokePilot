from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one match, found {count}: {old[:100]!r}")
    p.write_text(text.replace(old, new, 1))


replace_once(
    "world/graph.go",
    '''\tWarpX uint8 // EdgeWarp: tile X; EdgeConnection: encoded band start (see ConnectionBand)\n\tWarpY uint8 // EdgeWarp: tile Y; EdgeConnection: encoded band end\n\tDir   uint8 // EdgeConnection only: 0=north 1=south 2=west 3=east\n''',
    '''\tWarpX uint8 // EdgeWarp only: the warp tile on the source map\n\tWarpY uint8\n\tDir   uint8 // EdgeConnection only: 0=north 1=south 2=west 3=east\n\n\t// Connection edges may be scoped to one contiguous source-border band.\n\t// The index is X for north/south and Y for west/east. Zero-valued legacy\n\t// edges remain unscoped when BandScoped is false.\n\tBandStart  uint8\n\tBandEnd    uint8\n\tBandScoped bool\n''',
)

p = Path("world/connection_band.go")
text = p.read_text()
start = text.index("// ConnectionBand returns")
end = text.index("func connectionBandRange")
replacement = r'''// ConnectionBand returns the inclusive source-border index range carried by a
// component-scoped connection edge. North/south bands are X coordinates;
// west/east bands are Y coordinates. Unscoped/hand-built connection edges
// return ok=false and retain the historical whole-border behavior.
func ConnectionBand(e Edge) (start, end int, ok bool) {
	if e.Kind != EdgeConnection || !e.BandScoped {
		return 0, 0, false
	}
	start, end = int(e.BandStart), int(e.BandEnd)
	if end < start {
		return 0, 0, false
	}
	return start, end, true
}

func encodeConnectionBand(e Edge, start, end int) (Edge, bool) {
	if start < 0 || end < start || end > 255 {
		return Edge{}, false
	}
	e.BandStart = uint8(start)
	e.BandEnd = uint8(end)
	e.BandScoped = true
	return e, true
}

'''
p.write_text(text[:start] + replacement + text[end:])

replace_once(
    "world/connection_band.go",
    '''\tsrc, okSrc := g.tiles[from]\n\t_, okDst := g.tiles[c.MapID]\n\tif !okSrc || !okDst || g.comps[from] == nil || g.comps[c.MapID] == nil {\n''',
    '''\tsrc, okSrc := g.tiles[from]\n\tdst, okDst := g.tiles[c.MapID]\n\tif !okSrc || !okDst || g.comps[from] == nil || g.comps[c.MapID] == nil {\n''',
)

replace_once(
    "world/connection_band.go",
    '''\tfor i := 0; i < n; i++ {\n\t\tsx, sy, tx, ty := g.connectionSeamTile(base, c, i)\n\t\ta := standingComponentAt(g, from, sx, sy)\n\t\tb := standingComponentAt(g, c.MapID, tx, ty)\n\t\tif len(a) == 0 || len(b) == 0 {\n\t\t\tflush(i - 1)\n\t\t\tcontinue\n\t\t}\n\t\tpair := connectionComponentPair{exit: a[0], entry: b[0]}\n''',
    '''\tfor i := 0; i < n; i++ {\n\t\tsx, sy, tx, ty := g.connectionSeamTile(base, c, i)\n\t\t// Offset can leave part of the source edge outside the actual overlap;\n\t\t// that is not a physical connection band. A tile that is in-bounds but\n\t\t// non-walkable *is* retained with component 0 so semantic transitions\n\t\t// such as Surf can still own it while ordinary canExit rejects it.\n\t\tif sx < 0 || sy < 0 || sx >= src.w || sy >= src.h ||\n\t\t\ttx < 0 || ty < 0 || tx >= dst.w || ty >= dst.h {\n\t\t\tflush(i - 1)\n\t\t\tcontinue\n\t\t}\n\t\texitComp, entryComp := 0, 0\n\t\tif a := standingComponentAt(g, from, sx, sy); len(a) > 0 {\n\t\t\texitComp = a[0]\n\t\t}\n\t\tif b := standingComponentAt(g, c.MapID, tx, ty); len(b) > 0 {\n\t\t\tentryComp = b[0]\n\t\t}\n\t\tpair := connectionComponentPair{exit: exitComp, entry: entryComp}\n''',
)

replace_once(
    "world/connection_band_test.go",
    '''\tif len(edges) != 3 {\n\t\tt.Fatalf("connectionEdges produced %d edges, want 3: %+v", len(edges), edges)\n\t}\n\n\twant := [][2]int{{0, 1}, {2, 3}, {5, 5}}\n\twantEntry := []int{2, 3, 4}\n''',
    '''\tif len(edges) != 4 {\n\t\tt.Fatalf("connectionEdges produced %d edges, want 4: %+v", len(edges), edges)\n\t}\n\n\twant := [][2]int{{0, 1}, {2, 3}, {4, 4}, {5, 5}}\n\twantEntry := [][]int{{2}, {3}, nil, {4}}\n''',
)

replace_once(
    "world/connection_band_test.go",
    '''\t\tif got := g.connectionPortComps(e, true); len(got) != 1 || got[0] != wantEntry[i] {\n\t\t\tt.Errorf("edge %d entry components = %v, want [%d]", i, got, wantEntry[i])\n\t\t}\n''',
    '''\t\tgot := g.connectionPortComps(e, true)\n\t\twantComps := wantEntry[i]\n\t\tif len(got) != len(wantComps) || (len(got) == 1 && got[0] != wantComps[0]) {\n\t\t\tt.Errorf("edge %d entry components = %v, want %v", i, got, wantComps)\n\t\t}\n''',
)
