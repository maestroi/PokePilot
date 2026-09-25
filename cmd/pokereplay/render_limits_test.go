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
	for i := 0; i < cap(replayRenderSlots); i++ {
		release, err := acquireReplayRender(context.Background())
		if err != nil {
			t.Fatalf("slot %d: %v", i, err)
		}
		defer release()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	second, err := acquireReplayRender(ctx)
	if err == nil {
		second()
		t.Fatal("render acquired a slot while all were held")
	}
	if ctx.Err() == nil {
		t.Fatalf("second render err=%v, want context cancellation", err)
	}
}
