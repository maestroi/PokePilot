package state

import (
	"testing"

	"github.com/maestroi/pokepilot/red/sym"
)

func TestDecodeCoinsBCD(t *testing.T) {
	var mem Mem
	mem[sym.PlayerCoins] = 0x99
	mem[sym.PlayerCoins+1] = 0x99
	if got := DecodeCoins(&mem); got != 9999 {
		t.Fatalf("DecodeCoins = %d, want 9999", got)
	}
	mem[sym.PlayerCoins] = 0x28
	mem[sym.PlayerCoins+1] = 0x50
	if got := DecodeCoins(&mem); got != 2850 {
		t.Fatalf("DecodeCoins = %d, want 2850", got)
	}
}
