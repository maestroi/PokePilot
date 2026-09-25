package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// TestWriteFileAtomicNeverExposesPartialState is the root-cause regression for
// the poisoned resume lineage. The farm checkpoint uploader polls the
// checkpoint directory on a timer and publishes whatever it reads, so a state
// must appear whole or not at all. os.WriteFile truncates the target first, and
// a reader inside that window sees zero or partial bytes: that is how a
// complete 321 KB pre-objective checkpoint was stored on the wall as a 0-byte
// resume state, whose high embedded frame then outranked every checkpoint the
// run produced afterwards and restarted it from a fresh cartridge forever.
func TestWriteFileAtomicNeverExposesPartialState(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "round-001-frame-0003842410-progress-secret-key-owned.state")
	payload := bytes.Repeat([]byte("emulator-state"), 321616/14)

	var partial, complete int64
	stop := make(chan struct{})
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			b, err := os.ReadFile(target)
			if err != nil {
				continue // no state visible yet: the rename has not landed
			}
			if len(b) != len(payload) {
				atomic.AddInt64(&partial, 1)
				continue
			}
			atomic.AddInt64(&complete, 1)
		}
	}()
	for i := 0; i < 250; i++ {
		if err := writeFileAtomic(target, ".state-*.tmp", payload); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	<-readerDone

	if partial != 0 {
		t.Fatalf("reader observed %d partial/empty states (%d complete reads)", partial, complete)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(target) {
		t.Fatalf("checkpoint dir holds %d entries, want only the state: %v", len(entries), entries)
	}
}
