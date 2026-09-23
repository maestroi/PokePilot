package skill

import (
	"errors"
	"os"
	"testing"

	gameruntime "github.com/maestroi/pokepilot/game"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
	"github.com/maestroi/pokepilot/world"
)

// TestLeagueRoomExitIsOneWayWhileChallengeIsRunning pins the shared invariant
// behind run-jxh8lk19wv6on's stall: inside the Elite Four gauntlet the south
// door of Lorelei's, Bruno's and Agatha's rooms reads as an ordinary two-way
// warp in the ROM table and the static collision grid, but the room script
// refuses that walk and pushes the player back north. The router must not
// price a retreat the cartridge will not perform.
//
// This runs on ROM bytes alone — no emulator, no fixture — so it is cheap
// enough to keep. Fixtures are replayed rather than committed, and the failure
// itself only appears after the whole campaign reaches the League.
func TestLeagueRoomExitIsOneWayWhileChallengeIsRunning(t *testing.T) {
	romData := romBytesForLeagueGate(t)

	blocked := leagueRoutePrereqs(t, romData, leagueMem(t, false))
	for _, from := range []uint8{loreleiRoomMap, brunoRoomMap, agathaRoomMap} {
		_, err := findRoutePlanForDestination(blocked.graph, from, 4, 9, indigoLobbyCenter(), nil, blocked.prereqs)
		var routeBlocked *world.RouteBlockedError
		if !errors.As(err, &routeBlocked) {
			t.Fatalf("map %#02x -> lobby err = %v, want an actionable route blockage", from, err)
		}
		if !routeBlockageNames(routeBlocked, "red:league_room_exit", capCanLeaveLeague) {
			t.Fatalf("map %#02x -> lobby blockage = %+v, want transition red:league_room_exit missing %s",
				from, routeBlocked.Blockages, capCanLeaveLeague)
		}
	}
}

// TestLeagueRoomExitOpensAfterHallOfFame is the other half of the gate: the
// durable completion bit has to reopen the door, or a finished campaign would
// be permanently sealed in the League it just won.
func TestLeagueRoomExitOpensAfterHallOfFame(t *testing.T) {
	romData := romBytesForLeagueGate(t)

	open := leagueRoutePrereqs(t, romData, leagueMem(t, true))
	_, err := findRoutePlanForDestination(open.graph, loreleiRoomMap, 4, 9, indigoLobbyCenter(), nil, open.prereqs)
	if err != nil {
		t.Fatalf("completed campaign could not route out of Lorelei's room: %v", err)
	}
}

// TestLeagueLobbyEntryStaysOpen: the gate is directional. Walking INTO the
// gauntlet from the lobby is how the challenge starts, so over-gating that
// edge would make the League unreachable.
func TestLeagueLobbyEntryStaysOpen(t *testing.T) {
	romData := romBytesForLeagueGate(t)

	round := leagueRoutePrereqs(t, romData, leagueMem(t, false))
	lorelei := Destination{Map: loreleiRoomMap, X: 4, Y: 9}
	if _, err := findRoutePlanForDestination(round.graph, indigoPlateauLobbyMap, 7, 6, lorelei, nil, round.prereqs); err != nil {
		t.Fatalf("league entry from the lobby was blocked: %v", err)
	}
}

// TestLeagueBlackoutRecoveryStaysOpen: losing inside the gauntlet places the
// player on the Indigo Plateau exterior with the challenge still running.
// Recovery has to walk back to the lobby nurse from there, so that approach
// must not be caught by the room-exit gate.
func TestLeagueBlackoutRecoveryStaysOpen(t *testing.T) {
	romData := romBytesForLeagueGate(t)

	recovery := leagueRoutePrereqs(t, romData, leagueMem(t, false))
	if _, err := findRoutePlanForDestination(recovery.graph, indigoPlateauMap, 11, 14, indigoLobbyCenter(), nil, recovery.prereqs); err != nil {
		t.Fatalf("blackout recovery from the Indigo exterior was blocked: %v", err)
	}
}

type leagueRoute struct {
	graph   *world.Graph
	prereqs world.RoutePrerequisites
}

func indigoLobbyCenter() Destination {
	dest, ok := Place("indigo plateau pokemon center")
	if !ok {
		panic("indigo plateau pokemon center is not registered")
	}
	return dest
}

// leagueMem builds the minimal RAM projection the router reads: an in-progress
// Elite Four challenge, optionally with the durable Hall-of-Fame bit set.
func leagueMem(t *testing.T, mainStoryComplete bool) *state.Mem {
	t.Helper()
	var mem state.Mem
	// EVENT_AUTOWALKED_INTO_LORELEIS_ROOM / EVENT_BEAT_LORELEIS_ROOM_TRAINER_0
	// live in the Indigo event range, which only exists once the gauntlet has
	// been entered.
	setEvent(&mem, leagueEventAutowalkedIntoLoreleisRoom)
	if mainStoryComplete {
		mem[elite4FlagsAddrRAM] |= elite4CompletedBit
	}
	facts := state.DecodeStoryFacts(&mem, state.DecodeInventory(&mem))
	if !facts.LeagueChallengeStarted {
		t.Fatal("test setup did not project a started League challenge")
	}
	if facts.MainStoryComplete != mainStoryComplete {
		t.Fatalf("test setup main story complete = %v, want %v", facts.MainStoryComplete, mainStoryComplete)
	}
	return &mem
}

func setEvent(mem *state.Mem, event state.Event) {
	addr := sym.EventFlags + uint16(event)/8
	mem[addr] |= 1 << (uint16(event) % 8)
}

func leagueRoutePrereqs(t *testing.T, romData []byte, mem *state.Mem) leagueRoute {
	t.Helper()
	graph, err := world.BuildGraph(romData)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	return leagueRoute{graph: graph, prereqs: redRoutePrerequisites(graph, romData, mem)}
}

func routeBlockageNames(blocked *world.RouteBlockedError, transitionID string, missing gameruntime.CapabilityID) bool {
	for _, blockage := range blocked.Blockages {
		if blockage.Transition.ID != transitionID {
			continue
		}
		for _, id := range blockage.Missing {
			if id == missing {
				return true
			}
		}
	}
	return false
}

// Red adapter facts the route gate reads. They mirror red/state's private
// constants; story_test.go is what keeps the event index honest.
const (
	leagueEventAutowalkedIntoLoreleisRoom state.Event = 0x8E6
	elite4FlagsAddrRAM                    uint16      = 0xD734
	elite4CompletedBit                    uint8       = 1 << 0
)

func romBytesForLeagueGate(t *testing.T) []byte {
	t.Helper()
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM required for ROM-backed route topology")
	}
	romData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	return romData
}
