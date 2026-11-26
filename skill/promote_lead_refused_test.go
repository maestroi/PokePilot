package skill

import (
	"errors"
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
)

// Pewter Center's Jigglypuff puts Yellow's starter Pikachu to sleep, and the
// party menu then refuses to select it ("There isn't any response...").
// PromoteToLead must report that as ErrPartySelectionRefused with the order
// unchanged and the overworld controllable, not leave the START menu open
// (run-19hh6kl2rokmj175snrpc9rkj1, triage:247b8865e0eb7ab9). The .state is
// never committed; set YELLOW_SLEEPING_PIKACHU_CHECKPOINT to materialize it.
func TestPromoteToLeadReportsRefusedSleepingPikachu(t *testing.T) {
	rom := os.Getenv("POKEMON_YELLOW_ROM")
	checkpoint := os.Getenv("YELLOW_SLEEPING_PIKACHU_CHECKPOINT")
	if rom == "" || checkpoint == "" {
		t.Skip("set POKEMON_YELLOW_ROM and YELLOW_SLEEPING_PIKACHU_CHECKPOINT to replay the captured refusal")
	}
	stateBytes, err := os.ReadFile(checkpoint)
	if err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}
	m, err := emu.Open(rom)
	if err != nil {
		t.Fatalf("open ROM: %v", err)
	}
	defer m.Close()
	if err := m.LoadState(stateBytes); err != nil {
		t.Fatalf("load state: %v", err)
	}
	var mem state.Mem
	state.Snapshot(m, &mem)
	lead := state.DecodeParty(&mem).Mons[0].Species

	err = SetLead(m, 3)
	if !errors.Is(err, ErrPartySelectionRefused) {
		t.Fatalf("SetLead = %v, want ErrPartySelectionRefused", err)
	}
	state.Snapshot(m, &mem)
	if got := state.DecodeParty(&mem).Mons[0].Species; got != lead {
		t.Fatalf("lead = %#02x, want unchanged %#02x", got, lead)
	}
	if !state.Controllable(&mem) || state.DecodeInteraction(&mem).Kind != state.InteractionNone {
		t.Fatalf("boundary dirty: controllable=%v interaction=%s", state.Controllable(&mem), state.DecodeInteraction(&mem).Kind)
	}
}
