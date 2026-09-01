package emu_test

import (
	"crypto/sha256"
	"os"
	"testing"

	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/skill/fixture"
)

// TestRestoredStateReplaysDeterministically answers one question: given the
// same save state and the same button inputs, does this emulator produce the
// same result every time?
//
// It reaches a known, controllable state through the existing fixture helper
// (no new boot path), snapshots it with SaveState, then replays a fixed input
// script from a fresh LoadState three times and a DIFFERENT control script
// once. After each run it hashes WRAM and the PPU display memory into one
// digest. The three replays must hash identically; the control must hash
// differently, so the test can actually fail (a digest that ignores the
// emulator state would pass the three-way check but fail the control).
//
// The main script walks the player into Route 1's tall grass to trigger a
// wild battle — the RNG-consuming event — then fights a few turns. The RNG is
// seeded from rDIV (the hardware divider), which advances every frame, so a
// battle exercises the exact reseeding path the rest of the project depends
// on.
func TestRestoredStateReplaysDeterministically(t *testing.T) {
	if os.Getenv("POKEMON_RED_ROM") == "" {
		t.Skip("POKEMON_RED_ROM not set; cannot load ROM or fixture")
	}

	// Reach the known state via the existing fixture helper.
	base, err := fixture.LoadState("route1")
	if err != nil {
		t.Fatalf("fixture.LoadState(route1): %v", err)
	}
	snapshot, err := base.SaveState()
	if err != nil {
		base.Close()
		t.Fatalf("SaveState: %v", err)
	}
	base.Close()

	rom := os.Getenv("POKEMON_RED_ROM")
	run := func(script func(*emu.Emu)) [32]byte {
		m, err := emu.Open(rom)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer m.Close()
		if err := m.LoadState(snapshot); err != nil {
			t.Fatalf("LoadState: %v", err)
		}
		script(m)
		return hashState(t, m)
	}

	d0 := run(walkEastAndFight)
	d1 := run(walkEastAndFight)
	d2 := run(walkEastAndFight)

	if d0 != d1 || d0 != d2 {
		t.Fatalf("same state + same input script produced different digests:\n d0=%x\n d1=%x\n d2=%x", d0, d1, d2)
	}

	// Control: a different input script must produce a different digest.
	// Without this, the three-way equality above would pass even if the hash
	// ignored the emulator state entirely.
	dc := run(walkWest)
	if dc == d0 {
		t.Fatalf("control script (different inputs) produced the same digest %x as the main script; the hash does not reflect the emulator state", d0)
	}
}

// hashState hashes WRAM and the PPU display memory (the screen's backing
// store) into one digest.
//
// The rendered RGB frame (gomeboy's e.Frame()) is not exposed by emu.Emu —
// it sits behind the unexported e field — and this task forbids adding a
// production API. The PPU display memory is hashed instead: it is what the
// screen is drawn from, and being plain memory it is scanline-stable, so it
// avoids the "hashing the PPU mid-frame" artifact a rendered-frame hash would
// be subject to. VRAM holds the tile patterns and both tile maps; OAM/BGP/OBP
// hold the sprite and palette state.
func hashState(t *testing.T, m *emu.Emu) [32]byte {
	t.Helper()
	h := sha256.New()
	buf := make([]byte, 0x2000)

	m.PeekInto(0xC000, buf[:0x1000]) // WRAM
	h.Write(buf[:0x1000])
	m.PeekInto(0x8000, buf)          // VRAM: tile patterns + both tile maps
	h.Write(buf)
	m.PeekInto(0xFE00, buf[:0x100])  // OAM + BGP + OBP
	h.Write(buf[:0x100])

	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return digest
}

// walkEastAndFight is the main script. The route1 fixture stands the player at
// Route 1 (5,14) facing left. Walking east 18 taps crosses into the tall grass
// (verified: a wild battle starts at (14,14)), then A/B taps and frame
// stepping play out a few battle turns. Both button taps and plain frame
// stepping are present, and the total is well over several hundred frames.
func walkEastAndFight(m *emu.Emu) {
	for i := 0; i < 18; i++ {
		m.Tap(emu.Right, 3, 7)
	}
	m.StepFrames(300) // battle intro
	for i := 0; i < 6; i++ {
		m.Tap(emu.A, 3, 7)
	}
	m.StepFrames(600) // battle turns: move effectiveness, crit, accuracy, damage
	for i := 0; i < 4; i++ {
		m.Tap(emu.B, 3, 7)
	}
	m.StepFrames(300)
}

// walkWest is the control script: the same shape but walking the opposite
// direction. West of (5,14) is the path and then a wall (no grass), so no
// battle starts and the player ends in a different place — a clearly
// different final state, and therefore a different digest.
func walkWest(m *emu.Emu) {
	for i := 0; i < 18; i++ {
		m.Tap(emu.Left, 3, 7)
	}
	m.StepFrames(300)
	for i := 0; i < 6; i++ {
		m.Tap(emu.A, 3, 7)
	}
	m.StepFrames(600)
	for i := 0; i < 4; i++ {
		m.Tap(emu.B, 3, 7)
	}
	m.StepFrames(300)
}
