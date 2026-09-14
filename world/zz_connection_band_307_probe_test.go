package world

import "testing"

func TestProbeConnectionBands307(t *testing.T) {
	g := loadGraph(t)
	for _, e := range g.Edges[0x03] {
		if e.Kind != EdgeConnection || e.To != 0x0f {
			continue
		}
		start, end, scoped := ConnectionBand(e)
		t.Logf("Cerulean->Route4 band=%d..%d scoped=%t exitRaw=%v entryRaw=%v entryExpanded=%v", start, end, scoped, g.exitPortComps(e), g.entryPortComps(e), g.entryComps[e])
	}
}
