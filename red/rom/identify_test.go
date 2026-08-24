package rom

import (
	"errors"
	"os"
	"testing"
)

func TestSHA1Hex(t *testing.T) {
	got := SHA1Hex([]byte("abc"))
	want := "a9993e364706816aba3e25717850c26c9cd0d89d"
	if got != want {
		t.Errorf("SHA1Hex(abc) = %s, want %s", got, want)
	}
}

func TestVerifyRejectsUnknown(t *testing.T) {
	err := Verify([]byte("not a rom"))
	var unknown *ErrUnknownROM
	if !errors.As(err, &unknown) {
		t.Fatalf("Verify(not a rom) = %v, want *ErrUnknownROM", err)
	}
}

func TestVerifyAcceptsRealROM(t *testing.T) {
	path := os.Getenv("POKEMON_RED_ROM")
	if path == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ROM: %v", err)
	}
	if err := Verify(rom); err != nil {
		t.Fatalf("Verify(real ROM) = %v, want nil", err)
	}
}
