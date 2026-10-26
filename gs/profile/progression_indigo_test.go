package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/gs/sym"
)

// Only the Indigo flypoint (SPAWN_INDIGO) proves the plateau was reached;
// eight badges or a neighbouring Kanto/Johto flypoint must not.
func TestGSIndigoPlateauReachedIsTheIndigoFlypoint(t *testing.T) {
	for _, p := range []*Profile{NewGold(), NewSilver()} {
		mem := fakeGSReader{}
		mem[sym.JohtoBadges] = 0xff
		// SPAWN_CINNABAR (12) and SPAWN_NEW_BARK (14) straddle SPAWN_INDIGO.
		mem[sym.VisitedSpawns+1] = 1<<4 | 1<<6
		if p.DecodeFirstBadgeProgress(mem).Has(ProgressIndigoPlateauReached) {
			t.Fatal("neighbouring flypoints or eight badges manufactured Indigo Plateau")
		}
		mem[sym.VisitedSpawns+1] |= 1 << 5
		if !p.DecodeFirstBadgeProgress(mem).Has(ProgressIndigoPlateauReached) {
			t.Fatal("ENGINE_FLYPOINT_INDIGO_PLATEAU not decoded")
		}
	}
}
