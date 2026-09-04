package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func centerRecoveryMem(hp, maxHP uint16, status, pp uint8) state.Mem {
	var mem state.Mem
	mem[sym.PartyCount] = 1
	base := sym.PartyMon1
	mem[base+sym.MonSpecies] = 1
	mem[base+sym.MonHP] = byte(hp >> 8)
	mem[base+sym.MonHP+1] = byte(hp)
	mem[base+sym.MonStatus] = status
	mem[base+sym.MonMoves] = 1
	mem[base+sym.MonPP] = pp
	mem[base+sym.MonMaxHP] = byte(maxHP >> 8)
	mem[base+sym.MonMaxHP+1] = byte(maxHP)
	return mem
}

func TestAllPartyCenterRecoveredIncludesPPAndStatus(t *testing.T) {
	recovered := centerRecoveryMem(20, 20, 0, 5)
	if !allPartyCenterRecovered(&recovered) {
		t.Fatal("full HP, healthy status, positive PP should satisfy Center recovery")
	}

	noPP := centerRecoveryMem(20, 20, 0, 0)
	if allPartyCenterRecovered(&noPP) {
		t.Fatal("zero PP was accepted as fully Center-recovered")
	}

	status := centerRecoveryMem(20, 20, 1, 5)
	if allPartyCenterRecovered(&status) {
		t.Fatal("status ailment was accepted as fully Center-recovered")
	}

	hurt := centerRecoveryMem(10, 20, 0, 5)
	if allPartyCenterRecovered(&hurt) {
		t.Fatal("missing HP was accepted as fully Center-recovered")
	}
}
