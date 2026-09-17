package skill_test

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/maestroi/gomeboy/pkg/link"
	"github.com/maestroi/pokepilot/game"
	gen1trade "github.com/maestroi/pokepilot/gen1/trade"
	"github.com/maestroi/pokepilot/red/state"
	redtrade "github.com/maestroi/pokepilot/red/trade"
	"github.com/maestroi/pokepilot/skill"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestVirtualCableClubTrade is the full acceptance path for the lightweight
// peer: an unmodified Red ROM with the Pokédex walks to a Center, negotiates
// the Cable Club over GomeBoy's broker, receives a synthetic remote Pokemon
// through the real Trade Center code, saves, and returns to the overworld.
// It is opt-in because the commercial ROM is never stored in the repository.
func TestVirtualCableClubTrade(t *testing.T) {
	if testing.Short() {
		t.Skip("ROM-backed Cable Club integration")
	}
	if os.Getenv("POKEMON_RED_ROM") == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}

	e, err := fixture.LoadState("post_errand")
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	romData := e.ROM()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	brokerDone := make(chan error, 1)
	go func() { brokerDone <- link.NewBroker().Serve(listener) }()

	remote, err := redtrade.SyntheticMon(romData, game.SpeciesID("pidgey"), 8, 0x504b)
	if err != nil {
		t.Fatal(err)
	}
	machine, err := gen1trade.NewMachine(gen1trade.MachineConfig{Trainer: gen1trade.NewTrainer("POKEPILOT", remote)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	peerDone := make(chan error, 1)
	go func() {
		peerDone <- gen1trade.RunBroker(ctx, gen1trade.BrokerConfig{
			Address: listener.Addr().String(), Session: "rom-e2e", PeerID: "virtual",
			Timeout: time.Second, OnReady: func() { close(ready) },
		}, machine)
	}()
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("virtual peer did not register with broker")
	}

	networkLink, err := e.ConnectBrokerLink(listener.Addr().String(), "rom-e2e", "emulator", "emulator", nil, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer networkLink.Close()

	result, err := skill.VirtualTrade(e, romData, 0, false, skill.StatAwareMove(romData))
	if err != nil {
		t.Fatalf("VirtualTrade: %v (result=%+v)", err, result)
	}
	if result.Trades != 1 {
		t.Fatalf("trades = %d, want 1", result.Trades)
	}
	var mem state.Mem
	state.Snapshot(e, &mem)
	party := state.DecodeParty(&mem)
	if party.Count != 1 || len(party.Mons) != 1 || party.Mons[0].Species != remote.Species() {
		t.Fatalf("party after trade = %+v, want remote species %#02x", party, remote.Species())
	}
	if !state.Controllable(&mem) {
		t.Fatal("player is not controllable after leaving the Trade Center")
	}

	cancel()
	select {
	case <-peerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("virtual peer did not stop")
	}
}
