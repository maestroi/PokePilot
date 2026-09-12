package game

import (
	"errors"
	"strings"
	"testing"
)

type testProfile struct {
	id       GameID
	revision RevisionID
	title    string
}

func (p testProfile) ID() GameID          { return p.id }
func (p testProfile) Revision() RevisionID { return p.revision }
func (p testProfile) Detect(info ROMInfo) bool { return info.Title == p.title }
func (p testProfile) Symbols() SymbolTable {
	out := SymbolTable{}
	for i, name := range requiredProfileSymbols {
		out[name] = MemorySymbol{Name: name, Address: uint16(0xc000 + i), Width: 1}
	}
	return out
}
func (p testProfile) Features() ProfileFeatures { return ProfileFeatures{} }
func (p testProfile) ROMParser() ROMParser      { return testParser{} }
func (p testProfile) DecodeObservation(MemoryReader, []byte) (ProfileObservation, error) {
	return ProfileObservation{}, nil
}

type testParser struct{}

func (testParser) MapName(uint16) (string, bool)   { return "test", true }
func (testParser) Species(uint16) (SpeciesID, bool) { return "testmon", true }

func testROM(title string) []byte {
	rom := make([]byte, 0x150)
	copy(rom[0x134:0x144], []byte(title))
	rom[0x147] = 0x13
	rom[0x148] = 0x05
	rom[0x149] = 0x03
	return rom
}

func TestInspectROM(t *testing.T) {
	rom := testROM("TEST GAME")
	info := InspectROM(rom)
	if info.Title != "TEST GAME" {
		t.Fatalf("title = %q", info.Title)
	}
	if len(info.SHA1) != 40 || len(info.SHA256) != 64 {
		t.Fatalf("unexpected hashes sha1=%q sha256=%q", info.SHA1, info.SHA256)
	}
	if info.CartridgeType != 0x13 || info.ROMSizeCode != 0x05 || info.RAMSizeCode != 0x03 {
		t.Fatalf("header = %#v", info)
	}
}

func TestRegistryDetectROM(t *testing.T) {
	p := testProfile{id: "test", revision: "rev0", title: "TEST GAME"}
	r, err := NewRegistry(p)
	if err != nil {
		t.Fatal(err)
	}
	got, info, err := r.DetectROM(testROM("TEST GAME"))
	if err != nil {
		t.Fatal(err)
	}
	if got.ID() != p.ID() || info.Title != "TEST GAME" {
		t.Fatalf("got profile=%s info=%#v", got.ID(), info)
	}
}

func TestRegistryUnsupportedROMDiagnostic(t *testing.T) {
	p := testProfile{id: "test", revision: "rev0", title: "TEST GAME"}
	r, err := NewRegistry(p)
	if err != nil {
		t.Fatal(err)
	}
	_, info, err := r.DetectROM(testROM("OTHER GAME"))
	if err == nil {
		t.Fatal("expected unsupported ROM error")
	}
	var unsupported UnsupportedROMError
	if !errors.As(err, &unsupported) {
		t.Fatalf("error type = %T: %v", err, err)
	}
	for _, want := range []string{"OTHER GAME", info.SHA1, info.SHA256} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
}

func TestRegistryRejectsAmbiguousMatches(t *testing.T) {
	one := testProfile{id: "one", revision: "rev0", title: "TEST GAME"}
	two := testProfile{id: "two", revision: "rev0", title: "TEST GAME"}
	r, err := NewRegistry(one, two)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.DetectROM(testROM("TEST GAME")); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous match error, got %v", err)
	}
}

func TestValidateProfileContractRequiresSemanticSymbols(t *testing.T) {
	p := testProfile{id: "test", revision: "rev0", title: "TEST"}
	if err := ValidateProfileContract(p); err != nil {
		t.Fatal(err)
	}

	broken := profileWithoutSymbols{GameProfile: p}
	if err := ValidateProfileContract(broken); err == nil {
		t.Fatal("expected missing symbol error")
	}
}

type profileWithoutSymbols struct{ GameProfile }

func (profileWithoutSymbols) Symbols() SymbolTable { return SymbolTable{} }
