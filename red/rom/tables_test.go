package rom

import (
	"testing"

	"github.com/maestroi/pokepilot/gen1rom/symtest"
)

func TestRedTablesMatchDecompSymbols(t *testing.T) {
	symtest.Verify(t, "../../pokered/pokered.sym", symtest.Labels(redTables, "SuperRodData"))
}

func TestRedEventFlagRefIsNative(t *testing.T) {
	addr, mask, ok := EventFlagRef(nil, 0xD747, 9)
	if !ok || addr != 0xD748 || mask != 1<<1 {
		t.Fatalf("EventFlagRef(0xd747, 9) = %#04x,%#02x,%v want 0xd748,0x02,true", addr, mask, ok)
	}
}
