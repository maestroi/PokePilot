package emu

import (
	"errors"
	"os"
	"testing"
)

const testROM = "/home/maestro/Documents/projects/gomeboy/tests/roms/little-things-gb/firstwhite.gb"

func openTestEmu(t *testing.T) *Emu {
	t.Helper()
	if _, err := os.Stat(testROM); err != nil {
		t.Skipf("test ROM not present: %v", err)
	}
	m, err := Open(testROM)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { m.Close() })
	return m
}

func TestOpenAndStep(t *testing.T) {
	m := openTestEmu(t)
	m.StepFrames(10)
	if got := m.FrameCount(); got != 10 {
		t.Fatalf("FrameCount() = %d, want 10", got)
	}
}

func TestStepUntilImmediate(t *testing.T) {
	m := openTestEmu(t)
	n, err := m.StepUntil(100, func(*Emu) bool { return true })
	if err != nil {
		t.Fatalf("StepUntil: unexpected error %v", err)
	}
	if n != 0 {
		t.Fatalf("stepped %d frames, want 0", n)
	}
}

func TestStepUntilTimeout(t *testing.T) {
	m := openTestEmu(t)
	n, err := m.StepUntil(5, func(*Emu) bool { return false })
	if err == nil {
		t.Fatal("StepUntil: expected error, got nil")
	}
	var to *ErrTimeout
	if !errors.As(err, &to) {
		t.Fatalf("StepUntil: error is %T (%v), want *ErrTimeout", err, err)
	}
	if to.Frames != 5 {
		t.Fatalf("ErrTimeout.Frames = %d, want 5", to.Frames)
	}
	if n != 5 {
		t.Fatalf("stepped %d frames, want 5", n)
	}
}

func TestStepUntilSucceedsMidway(t *testing.T) {
	m := openTestEmu(t)
	start := m.FrameCount()
	n, err := m.StepUntil(100, func(e *Emu) bool { return e.FrameCount() >= start+3 })
	if err != nil {
		t.Fatalf("StepUntil: %v", err)
	}
	if n != 3 {
		t.Fatalf("stepped %d frames, want 3", n)
	}
}

func TestTapAndHoldDoNotPanic(t *testing.T) {
	m := openTestEmu(t)
	for _, b := range []Button{A, B, Start, Select, Up, Down, Left, Right} {
		m.Tap(b, 3, 7)
		m.Hold(b, 5)
	}
}
