package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/emu"
)

// TestDebugProbe explains which debugger-level instruction first changes a
// watched byte in a saved state. It is deliberately separate from TestProbe:
// TestProbe answers map/topology questions, while this probe advances the CPU
// instruction by instruction and is therefore only for failure diagnosis.
//
// A label is resolved from the vendored pokered.sym, so the common failure
// probes do not require anyone to remember raw WRAM addresses:
//
//	PROBE_STATE=../failure-frame-00000184223-goto.state \
//	PROBE_WATCH=wJoyIgnore PROBE_CPU_STEPS=100000 \
//	go test ./skill -run TestDebugProbe -v
//
// PROBE_WATCH also accepts decimal, 0x-prefixed, or $-prefixed addresses.
// Checked forensic .state files and legacy/raw states are both accepted by
// emu.LoadState.
func TestDebugProbe(t *testing.T) {
	statePath := strings.TrimSpace(os.Getenv("PROBE_STATE"))
	watchSpec := strings.TrimSpace(os.Getenv("PROBE_WATCH"))
	if statePath == "" || watchSpec == "" {
		t.Skip("set PROBE_STATE and PROBE_WATCH to run the instruction-level debug probe")
	}
	romPath := strings.TrimSpace(os.Getenv("POKEMON_RED_ROM"))
	if romPath == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}

	stateBytes, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("PROBE_STATE %s: %v", statePath, err)
	}
	m, err := emu.Open(romPath)
	if err != nil {
		t.Fatalf("open ROM %s: %v", romPath, err)
	}
	defer m.Close()
	if err := m.LoadState(stateBytes); err != nil {
		t.Fatalf("PROBE_STATE %s: LoadState: %v", statePath, err)
	}

	addr, label := debugProbeAddress(t, watchSpec)
	maxSteps := debugProbeMaxSteps(t)
	cpu := m.CPUState()
	ppu := m.PPUState()
	stateHash, hashErr := m.StateHashHex()
	if hashErr != nil {
		stateHash = "<error: " + hashErr.Error() + ">"
	}
	t.Logf("loaded %s", filepath.Clean(statePath))
	t.Logf("start: frame=%d cycle=%d rom_sha256=%s state_hash=%s", m.FrameCount(), m.Cycle(), m.ROMSHA256Hex(), stateHash)
	t.Logf("watch: %s=%#04x value=%#02x max_steps=%d", label, addr, m.Peek8(addr), maxSteps)
	t.Logf("cpu: PC=%04X SP=%04X A=%02X F=%02X B=%02X C=%02X D=%02X E=%02X H=%02X L=%02X IME=%v IE=%02X IF=%02X halted=%v double_speed=%v halt_bug=%v",
		cpu.PC, cpu.SP, cpu.A, cpu.F, cpu.B, cpu.C, cpu.D, cpu.E, cpu.H, cpu.L, cpu.IME, cpu.IE, cpu.IF, cpu.Halted, cpu.DoubleSpeed, cpu.HaltBug)
	t.Logf("ppu: mode=%d LY=%d LX=%d STAT=%02X lcd=%v bg=%v window=%v obj=%v", ppu.Mode, ppu.LY, ppu.LX, ppu.STAT, ppu.LCDEnabled, ppu.BGEnabled, ppu.WinEnabled, ppu.ObjEnabled)

	hit, err := m.WatchMemoryChange(addr, maxSteps)
	if err != nil {
		t.Fatalf("watch %s (%#04x): %v; stopped PC=%04X value=%#02x", label, addr, err, hit.CPU.PC, hit.After)
	}
	t.Logf("watch hit: %s (%#04x) %#02x -> %#02x after %d CPU step(s)", label, addr, hit.Before, hit.After, hit.Steps)
	t.Logf("instruction: PC=%04X -> %04X opcode=%02X executed=%v interrupt=%v cycles=%d frames=%d",
		hit.Step.PCBefore, hit.Step.PCAfter, hit.Step.Opcode, hit.Step.Executed, hit.Step.Interrupt, hit.Step.Cycles, hit.Step.Frames)
	t.Logf("after cpu: PC=%04X SP=%04X A=%02X F=%02X B=%02X C=%02X D=%02X E=%02X H=%02X L=%02X IME=%v IE=%02X IF=%02X halted=%v",
		hit.CPU.PC, hit.CPU.SP, hit.CPU.A, hit.CPU.F, hit.CPU.B, hit.CPU.C, hit.CPU.D, hit.CPU.E, hit.CPU.H, hit.CPU.L, hit.CPU.IME, hit.CPU.IE, hit.CPU.IF, hit.CPU.Halted)
	t.Logf("after ppu: mode=%d LY=%d LX=%d STAT=%02X", hit.PPU.Mode, hit.PPU.LY, hit.PPU.LX, hit.PPU.STAT)
}

func debugProbeMaxSteps(t *testing.T) uint64 {
	t.Helper()
	const defaultSteps = 100_000
	s := strings.TrimSpace(os.Getenv("PROBE_CPU_STEPS"))
	if s == "" {
		return defaultSteps
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil || n == 0 {
		t.Fatalf("PROBE_CPU_STEPS=%q: want a positive integer", s)
	}
	return n
}

func debugProbeAddress(t *testing.T, spec string) (uint16, string) {
	t.Helper()
	if strings.HasPrefix(spec, "0x") || strings.HasPrefix(spec, "0X") {
		n, err := strconv.ParseUint(spec[2:], 16, 16)
		if err != nil {
			t.Fatalf("PROBE_WATCH=%q: %v", spec, err)
		}
		return uint16(n), fmt.Sprintf("%#04x", n)
	}
	if strings.HasPrefix(spec, "$") {
		n, err := strconv.ParseUint(spec[1:], 16, 16)
		if err != nil {
			t.Fatalf("PROBE_WATCH=%q: %v", spec, err)
		}
		return uint16(n), fmt.Sprintf("%#04x", n)
	}
	if allDecimal(spec) {
		n, err := strconv.ParseUint(spec, 10, 16)
		if err != nil {
			t.Fatalf("PROBE_WATCH=%q: %v", spec, err)
		}
		return uint16(n), fmt.Sprintf("%#04x", n)
	}

	data, err := os.ReadFile(filepath.Join("..", "red", "sym", "testdata", "pokered.sym"))
	if err != nil {
		t.Fatalf("read vendored pokered.sym: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[1] != spec {
			continue
		}
		_, rawAddr, ok := strings.Cut(fields[0], ":")
		if !ok {
			continue
		}
		n, err := strconv.ParseUint(rawAddr, 16, 16)
		if err != nil {
			continue
		}
		return uint16(n), spec
	}
	t.Fatalf("PROBE_WATCH=%q: not an address or a label in red/sym/testdata/pokered.sym", spec)
	return 0, ""
}

func allDecimal(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
