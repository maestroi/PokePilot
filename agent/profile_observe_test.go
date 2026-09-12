package agent

import (
	"strings"
	"testing"
)

func TestObserveCheckedRejectsUnknownROMRevisionBeforeReadingMemory(t *testing.T) {
	rom := make([]byte, 0x150)
	copy(rom[0x134:0x144], []byte("POKEMON RED"))

	_, err := ObserveChecked(nil, rom)
	if err == nil {
		t.Fatal("expected unsupported ROM error")
	}
	for _, want := range []string{"unsupported ROM", "POKEMON RED", "sha1=", "sha256="} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
}
