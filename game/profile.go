package game

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// GameID is the stable identity of a supported game family. Revision is kept
// separate because RAM/ROM layouts may differ between releases with the same
// title.
type GameID string

type RevisionID string

// ROMInfo is the immutable identity used for profile selection. Hashes cover
// the complete ROM; header fields are retained for diagnostics only. Profiles
// should prefer an exact hash when they know one.
type ROMInfo struct {
	Title          string `json:"title"`
	SHA1           string `json:"sha1"`
	SHA256         string `json:"sha256"`
	Size           int    `json:"size"`
	CGBFlag        byte   `json:"cgb_flag"`
	CartridgeType  byte   `json:"cartridge_type"`
	ROMSizeCode    byte   `json:"rom_size_code"`
	RAMSizeCode    byte   `json:"ram_size_code"`
	Version        byte   `json:"version"`
	HeaderChecksum byte   `json:"header_checksum"`
}

// InspectROM fingerprints a ROM without interpreting any game-specific data.
func InspectROM(rom []byte) ROMInfo {
	h1 := sha1.Sum(rom)
	h256 := sha256.Sum256(rom)
	info := ROMInfo{
		SHA1:   hex.EncodeToString(h1[:]),
		SHA256: hex.EncodeToString(h256[:]),
		Size:   len(rom),
	}
	if len(rom) < 0x150 {
		return info
	}

	// DMG titles occupy 0x134..0x143. On CGB cartridges byte 0x143 is the
	// CGB flag, so exclude it from the title when the high bit is set.
	titleEnd := 0x144
	if rom[0x143]&0x80 != 0 {
		titleEnd = 0x143
	}
	title := rom[0x134:titleEnd]
	if i := bytes.IndexByte(title, 0); i >= 0 {
		title = title[:i]
	}
	info.Title = strings.TrimSpace(string(title))
	info.CGBFlag = rom[0x143]
	info.CartridgeType = rom[0x147]
	info.ROMSizeCode = rom[0x148]
	info.RAMSizeCode = rom[0x149]
	info.Version = rom[0x14c]
	info.HeaderChecksum = rom[0x14d]
	return info
}

// MemoryReader is the side-effect-free memory surface profiles may observe.
// emu.Emu satisfies it; tests can use a tiny fake without importing an emulator.
type MemoryReader interface {
	Peek8(addr uint16) byte
	PeekInto(addr uint16, dst []byte)
}

// MemorySymbol is a semantic address owned by a concrete game/revision.
// Bank is zero for fixed/DMG memory and may be non-zero for banked profiles.
type MemorySymbol struct {
	Name    string `json:"name"`
	Address uint16 `json:"address"`
	Bank    int    `json:"bank"`
	Width   int    `json:"width,omitempty"`
}

type SymbolTable map[string]MemorySymbol

func (s SymbolTable) Lookup(name string) (MemorySymbol, bool) {
	v, ok := s[name]
	return v, ok
}

// ProfileFeature describes an optional game/profile capability. Generic code
// must test a feature rather than branch on a game name.
type ProfileFeature string

const (
	FeatureMapParsing       ProfileFeature = "map_parsing"
	FeatureInventory        ProfileFeature = "inventory"
	FeatureStoryProgress    ProfileFeature = "story_progress"
	FeatureBattles          ProfileFeature = "battles"
	FeatureFieldMoves       ProfileFeature = "field_moves"
	FeatureTrainerFlags     ProfileFeature = "trainer_flags"
	FeatureBankedMemory     ProfileFeature = "banked_memory"
	FeatureSemanticSpecies  ProfileFeature = "semantic_species"
)

type ProfileFeatures map[ProfileFeature]bool

func (f ProfileFeatures) Has(feature ProfileFeature) bool { return f[feature] }

// ProfilePartyMon is the common party representation exposed by profiles.
type ProfilePartyMon struct {
	Species    SpeciesID `json:"species"`
	Level      uint8     `json:"level"`
	Experience uint32    `json:"experience"`
	HP         uint16    `json:"hp"`
	MaxHP      uint16    `json:"max_hp"`
	Status     string    `json:"status,omitempty"`
}

// ProfileObservation is the game-agnostic state every runtime may rely on.
// NativeMapID exists only as an opaque adapter handle for the current
// incremental migration; it is intentionally excluded from planner JSON.
type ProfileObservation struct {
	NativeMapID uint16 `json:"-"`
	Location    PlaceID `json:"location"`
	MapName     string  `json:"map_name"`
	X, Y        uint8
	Facing      string

	Controllable bool
	InBattle     bool
	Party        []ProfilePartyMon
	Badges       []string
	Money        uint32
	RespawnPlace PlaceID
	Events       []string
	Story        ProgressState
	BlackedOut   bool
}

// ROMParser exposes only semantic ROM lookups that are meaningful to generic
// code. Rich game-specific parsers may expose additional methods on their
// concrete type; callers discover those through feature-specific interfaces.
type ROMParser interface {
	MapName(rawMapID uint16) (string, bool)
	Species(rawSpecies uint16) (SpeciesID, bool)
}

// GameProfile owns all game/revision-specific layout knowledge required to
// turn emulator/ROM bytes into semantic state.
type GameProfile interface {
	ID() GameID
	Revision() RevisionID
	Detect(ROMInfo) bool
	Symbols() SymbolTable
	Features() ProfileFeatures
	ROMParser() ROMParser
	DecodeObservation(MemoryReader, []byte) (ProfileObservation, error)
}

var requiredProfileSymbols = []string{
	"player.map",
	"player.x",
	"player.y",
	"player.direction",
	"party.count",
	"party.members",
	"battle.mode",
	"badges",
	"bag",
	"money",
}

// ValidateProfileContract is reusable by every concrete profile test. It
// checks the invariants generic runtime code assumes without knowing a game.
func ValidateProfileContract(p GameProfile) error {
	if p == nil {
		return errors.New("game: nil profile")
	}
	if strings.TrimSpace(string(p.ID())) == "" {
		return errors.New("game: profile has empty id")
	}
	if strings.TrimSpace(string(p.Revision())) == "" {
		return fmt.Errorf("game: profile %q has empty revision", p.ID())
	}
	if p.ROMParser() == nil {
		return fmt.Errorf("game: profile %s@%s has nil ROM parser", p.ID(), p.Revision())
	}
	symbols := p.Symbols()
	for _, name := range requiredProfileSymbols {
		if _, ok := symbols[name]; !ok {
			return fmt.Errorf("game: profile %s@%s missing required symbol %q", p.ID(), p.Revision(), name)
		}
	}
	return nil
}

// Registry selects exactly one profile for a ROM. Registration order is not
// selection precedence: ambiguous matches are rejected so adding a profile
// cannot silently change which adapter owns an existing ROM.
type Registry struct {
	profiles []GameProfile
}

func NewRegistry(profiles ...GameProfile) (*Registry, error) {
	r := &Registry{}
	for _, p := range profiles {
		if err := r.Register(p); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Registry) Register(p GameProfile) error {
	if err := ValidateProfileContract(p); err != nil {
		return err
	}
	for _, existing := range r.profiles {
		if existing.ID() == p.ID() && existing.Revision() == p.Revision() {
			return fmt.Errorf("game: duplicate profile %s@%s", p.ID(), p.Revision())
		}
	}
	r.profiles = append(r.profiles, p)
	return nil
}

func (r *Registry) Profiles() []GameProfile {
	out := append([]GameProfile(nil), r.profiles...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID() == out[j].ID() {
			return out[i].Revision() < out[j].Revision()
		}
		return out[i].ID() < out[j].ID()
	})
	return out
}

func (r *Registry) DetectROM(rom []byte) (GameProfile, ROMInfo, error) {
	info := InspectROM(rom)
	var matches []GameProfile
	for _, p := range r.profiles {
		if p.Detect(info) {
			matches = append(matches, p)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], info, nil
	case 0:
		return nil, info, UnsupportedROMError{Info: info}
	default:
		keys := make([]string, 0, len(matches))
		for _, p := range matches {
			keys = append(keys, fmt.Sprintf("%s@%s", p.ID(), p.Revision()))
		}
		sort.Strings(keys)
		return nil, info, fmt.Errorf("game: ambiguous ROM profile match for title=%q sha256=%s: %s", info.Title, info.SHA256, strings.Join(keys, ", "))
	}
}

type UnsupportedROMError struct {
	Info ROMInfo
}

func (e UnsupportedROMError) Error() string {
	return fmt.Sprintf("game: unsupported ROM title=%q sha1=%s sha256=%s version=0x%02x size=%d", e.Info.Title, e.Info.SHA1, e.Info.SHA256, e.Info.Version, e.Info.Size)
}
