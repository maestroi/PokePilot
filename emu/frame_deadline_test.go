package emu

import (
	"errors"
	"testing"
)

func TestWithFrameDeadlineInterruptsBatchedStepFrames(t *testing.T) {
	m := openTestEmu(t)
	start := m.FrameCount()
	deadline := start + 7

	err := m.WithFrameDeadline(deadline, func() error {
		m.StepFrames(1000)
		return nil
	})
	if !errors.Is(err, ErrFrameDeadline) {
		t.Fatalf("WithFrameDeadline error = %v, want ErrFrameDeadline", err)
	}
	if got := m.FrameCount(); got != deadline {
		t.Fatalf("FrameCount() = %d, want exact deadline %d", got, deadline)
	}

	// The guard is scoped: normal stepping works immediately afterwards.
	m.StepFrames(2)
	if got := m.FrameCount(); got != deadline+2 {
		t.Fatalf("deadline leaked after return: frame=%d, want %d", got, deadline+2)
	}
}

func TestWithFrameDeadlineRethrowsUnrelatedPanics(t *testing.T) {
	m := openTestEmu(t)
	defer func() {
		if got := recover(); got != "boom" {
			t.Fatalf("recovered %v, want unrelated panic re-thrown", got)
		}
	}()
	_ = m.WithFrameDeadline(m.FrameCount()+10, func() error {
		panic("boom")
	})
}
