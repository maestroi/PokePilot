package skill

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/red/sym"
)

// TestEliteFourLossRecoveryQualification proves the League's ordinary loss path
// from a private deterministic checkpoint. The checkpoint is intentionally
// underpowered enough to lose to Lorelei with StatAwareMove.
//
// The regression owns three boundaries:
//  1. the mandatory fight must return RequiredBattleError rather than a plain
//     story-specific error;
//  2. the post-loss state must be a real Indigo blackout/respawn with Lorelei
//     still incomplete;
//  3. after a save/close/reopen/load boundary, the normal League room traversal
//     must heal/recommit and return to Lorelei without a member-specific retry
//     branch.
//
// Generic agent tests separately pin RequiredBattleError -> combat_defeat and
// combat_loss -> combat_retry after material readiness progress.
func TestEliteFourLossRecoveryQualification(t *testing.T) {
	if testing.Short() {
		t.Skip("ROM-backed qualification")
	}
	romPath := os.Getenv("POKEMON_RED_ROM")
	corpus := os.Getenv("POKEPILOT_QUALIFICATION_CORPUS")
	if romPath == "" || corpus == "" {
		t.Skip("POKEMON_RED_ROM and POKEPILOT_QUALIFICATION_CORPUS are required")
	}

	checkpoint := filepath.Join(corpus, "elite-four-loss-recovery", "start.state")
	stateBytes, err := os.ReadFile(checkpoint)
	if err != nil {
		t.Fatalf("read checkpoint %s: %v", checkpoint, err)
	}
	m, err := emu.Open(romPath)
	if err != nil {
		t.Fatalf("open ROM: %v", err)
	}
	defer func() {
		if m != nil {
			_ = m.Close()
		}
	}()
	if err := m.LoadState(stateBytes); err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}

	var before state.Mem
	state.Snapshot(m, &before)
	if got := before.U8(sym.CurMap); got != indigoPlateauLobbyMap {
		t.Fatalf("checkpoint map = %#02x, want Indigo lobby %#02x", got, indigoPlateauLobbyMap)
	}
	beforeFacts := state.DecodeStoryFacts(&before, state.DecodeInventory(&before))
	if beforeFacts.MainStoryComplete || beforeFacts.LeagueLoreleiDefeated {
		t.Fatalf("loss checkpoint already completed League progression: %+v", beforeFacts)
	}

	policy := StatAwareMove(m.ROM())
	if err := LeagueStartChallenge(m, m.ROM(), policy); err != nil {
		t.Fatalf("start League challenge: %v", err)
	}
	err = LeagueDefeatLorelei(m, m.ROM(), policy)
	if err == nil {
		t.Fatal("Lorelei unexpectedly won; refresh elite-four-loss-recovery/start.state with a deterministic losing party")
	}
	var required *RequiredBattleError
	if !errors.As(err, &required) {
		t.Fatalf("Lorelei loss = %v, want RequiredBattleError", err)
	}
	if required.Outcome.Result != state.ResultLost || !required.Outcome.Trainer || required.Outcome.Encounter == "" {
		t.Fatalf("Lorelei structured loss = %+v, want trainer defeat with encounter identity", required.Outcome)
	}

	var afterLoss state.Mem
	state.Snapshot(m, &afterLoss)
	afterFacts := state.DecodeStoryFacts(&afterLoss, state.DecodeInventory(&afterLoss))
	if afterFacts.LeagueLoreleiDefeated || afterFacts.MainStoryComplete {
		t.Fatalf("loss incorrectly committed League progress: %+v", afterFacts)
	}
	if got := afterLoss.U8(sym.CurMap); got != indigoPlateauMap {
		t.Fatalf("Lorelei blackout map = %#02x, want Indigo exterior %#02x", got, indigoPlateauMap)
	}

	// Production recovery persists checkpoints between attempts. Exercise the
	// same process boundary instead of relying on one emulator instance.
	m = reopenEliteFourQualification(t, romPath, m)

	if err := leagueReachRoom(m, m.ROM(), StatAwareMove(m.ROM()), loreleiRoomMap); err != nil {
		t.Fatalf("resume League after blackout: %v", err)
	}
	var resumed state.Mem
	state.Snapshot(m, &resumed)
	resumedFacts := state.DecodeStoryFacts(&resumed, state.DecodeInventory(&resumed))
	if resumed.U8(sym.CurMap) != loreleiRoomMap || !resumedFacts.LeagueChallengeStarted {
		t.Fatalf("resume did not recommit Lorelei boundary: map=%#02x facts=%+v", resumed.U8(sym.CurMap), resumedFacts)
	}
	if resumedFacts.LeagueLoreleiDefeated {
		t.Fatalf("resume fabricated Lorelei victory: %+v", resumedFacts)
	}
	if !allPartyCenterRecovered(&resumed) {
		t.Fatal("League blackout recovery did not restore party HP/status/PP before recommitting")
	}
}
