package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestReplayOutputTailIsBounded(t *testing.T) {
	w := &replayOutputTail{}
	prefix := strings.Repeat("a", maxReplayProcessLogBytes)
	if _, err := w.Write([]byte(prefix)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("TAIL")); err != nil {
		t.Fatal(err)
	}
	got := w.String()
	if len(got) != maxReplayProcessLogBytes {
		t.Fatalf("tail bytes=%d, want %d", len(got), maxReplayProcessLogBytes)
	}
	if !strings.HasSuffix(got, "TAIL") {
		t.Fatal("bounded output did not retain newest bytes")
	}
}

func TestReplayRenderSlotHonorsCancellation(t *testing.T) {
	release, err := acquireReplayRender(context.Background())
	if err != nil {
		t.Fatalf("first slot: %v", err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	second, err := acquireReplayRender(ctx)
	if err == nil {
		second()
		t.Fatal("second render acquired slot while first was held")
	}
	if ctx.Err() == nil {
		t.Fatalf("second render err=%v, want context cancellation", err)
	}
}
