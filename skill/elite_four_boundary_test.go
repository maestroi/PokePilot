package skill

import (
	"testing"

	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

func TestLeagueMemberBoundaryRejectsTransientControlWhileRoomScriptOwnsExecution(t *testing.T) {
	var mem state.Mem
	mem[sym.CurMapWidth] = 10
	mem[sym.CurMapHeight] = 8
	done := func(state.StoryFacts) bool { return true }

	mem[sym.LoreleisRoomCurScript] = 2 // LoreleiEndBattleScript
	if !state.Controllable(&mem) {
		t.Fatal("regression setup must expose the transient controllable frame")
	}
	if leagueMemberBoundaryReady(&mem, sym.LoreleisRoomCurScript, done) {
		t.Fatal("League boundary accepted control while Lorelei's end-battle script was still active")
	}

	mem[sym.LoreleisRoomCurScript] = 0
	if !leagueMemberBoundaryReady(&mem, sym.LoreleisRoomCurScript, done) {
		t.Fatal("League boundary did not accept the default room script on a controllable completed state")
	}
}

func TestLeagueRoomScriptAddressesMatchDeclaredRooms(t *testing.T) {
	cases := []struct {
		mapID uint8
		want  uint16
	}{
		{loreleiRoomMap, sym.LoreleisRoomCurScript},
		{brunoRoomMap, sym.BrunosRoomCurScript},
		{agathaRoomMap, sym.AgathasRoomCurScript},
		{lanceRoomMap, sym.LancesRoomCurScript},
	}
	for _, tc := range cases {
		got, ok := leagueRoomScriptAddress(tc.mapID)
		if !ok || got != tc.want {
			t.Fatalf("room %#02x script = %#04x, %v; want %#04x", tc.mapID, got, ok, tc.want)
		}
	}
}
