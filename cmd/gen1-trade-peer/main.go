// Command gen1-trade-peer runs a lightweight Pokemon Red/Blue Cable Club peer
// against a GomeBoy network-link broker. It does not run a second emulator.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/maestroi/pokepilot/game"
	gen1trade "github.com/maestroi/pokepilot/gen1/trade"
	"github.com/maestroi/pokepilot/red/data"
	redtrade "github.com/maestroi/pokepilot/red/trade"
)

func main() {
	broker := flag.String("broker", "127.0.0.1:8765", "GomeBoy link broker address")
	httpAddr := flag.String("http", "", "optional HTTP control API address; when set, sessions are provisioned via /v1/sessions")
	session := flag.String("session", "", "broker session id shared with the emulator (one-shot mode)")
	peerID := flag.String("peer-id", "virtual-trader", "broker peer id prefix")
	gameID := flag.String("game", "pokemon-red", "game metadata: pokemon-red or pokemon-blue")
	runID := flag.String("run-id", "", "optional PokePilot run id for provenance metadata")
	policy := flag.String("policy", "scripted", "trade policy: scripted, tradeback, version-assisted, or pokedex")
	species := flag.String("species", "pidgey", "species offered in virtual slot 1")
	level := flag.Int("level", 20, "level of the generated offered Pokemon")
	trainerName := flag.String("trainer-name", "POKEPILOT", "virtual trainer name")
	romPath := flag.String("rom", "", "Red/Blue ROM used to derive species data; defaults to POKEPILOT_ROM/POKEMON_RED_ROM")
	timeout := flag.Duration("timeout", 10*time.Second, "broker dial/serial timeout")
	sessionTTL := flag.Duration("session-ttl", 15*time.Minute, "maximum lifetime of a managed virtual-trader session")
	flag.Parse()

	if *romPath == "" {
		*romPath = os.Getenv("POKEPILOT_ROM")
		if *romPath == "" {
			*romPath = os.Getenv("POKEMON_RED_ROM")
		}
	}
	if *romPath == "" {
		log.Fatal("-rom or POKEPILOT_ROM is required to generate a valid Gen-I party Pokemon")
	}
	romData, err := os.ReadFile(*romPath)
	if err != nil {
		log.Fatalf("read ROM: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if strings.TrimSpace(*httpAddr) != "" {
		service := newTradeService(tradeServiceConfig{
			Broker: *broker, ROMData: romData, PeerID: *peerID, TrainerName: *trainerName,
			Timeout: *timeout, TTL: *sessionTTL, Events: os.Stdout,
		})
		defer service.Close()
		server := &http.Server{Addr: *httpAddr, Handler: service, ReadHeaderTimeout: 5 * time.Second}
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownCtx)
		}()
		log.Printf("Gen-I virtual trader service ready: http=%s broker=%s", *httpAddr, *broker)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
		return
	}

	if *session == "" {
		log.Fatal("-session is required unless -http is set")
	}
	if err := validatePolicy(*policy); err != nil {
		log.Fatal(err)
	}
	if *level < 1 || *level > 100 {
		log.Fatalf("-level %d outside 1..100", *level)
	}

	semanticSpecies := game.SpeciesID(game.CanonicalID(*species))
	mon, err := redtrade.SyntheticMon(romData, semanticSpecies, uint8(*level), 0x504b)
	if err != nil {
		log.Fatalf("build offered Pokemon: %v", err)
	}

	events := json.NewEncoder(os.Stdout)
	machine, err := gen1trade.NewMachine(gen1trade.MachineConfig{
		Trainer:   gen1trade.NewTrainer(*trainerName, mon),
		Tradeback: strings.EqualFold(*policy, "tradeback"),
		OnEvent: func(event gen1trade.Event) {
			name, _ := data.SpeciesName(event.Species)
			_ = events.Encode(map[string]any{
				"type":        "trade_event",
				"event":       event.Kind,
				"remote_slot": event.RemoteSlot,
				"local_slot":  event.LocalSlot,
				"species":     name,
				"species_id":  fmt.Sprintf("0x%02x", event.Species),
				"policy":      strings.ToLower(strings.TrimSpace(*policy)),
			})
		},
	})
	if err != nil {
		log.Fatalf("create trade machine: %v", err)
	}

	metadata := map[string]string{
		"game":              *gameID,
		"trade_policy":      strings.ToLower(strings.TrimSpace(*policy)),
		"requested_species": string(semanticSpecies),
		"status":            "ready_to_trade",
	}
	if *runID != "" {
		metadata["run_id"] = *runID
	}

	log.Printf("Gen-I virtual trader ready: broker=%s session=%s policy=%s species=%s", *broker, *session, *policy, semanticSpecies)
	err = gen1trade.RunBroker(ctx, gen1trade.BrokerConfig{
		Address: *broker, Session: *session, PeerID: *peerID, Metadata: metadata, Timeout: *timeout,
	}, machine)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func validatePolicy(policy string) error {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "scripted", "tradeback", "version-assisted", "pokedex":
		return nil
	case "sandbox":
		return errors.New("sandbox/Mew injection is intentionally not part of the v1.2.0 virtual-trader slice")
	case "strict":
		return errors.New("strict policy requires a real second game peer, not a synthetic virtual trader")
	default:
		return fmt.Errorf("unknown trade policy %q", policy)
	}
}
