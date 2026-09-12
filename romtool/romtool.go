// Package romtool contains game-agnostic memory snapshot and diff helpers used
// while reverse-engineering a Pokémon ROM profile.
package romtool

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

const SchemaVersion = 1

// Region is an inclusive, non-wrapping range in the Game Boy address space.
type Region struct {
	Name  string `json:"name"`
	Start uint16 `json:"start"`
	End   uint16 `json:"end"`
}

// DefaultRegions covers ordinary work RAM and high RAM without including I/O.
var DefaultRegions = []Region{
	{Name: "wram", Start: 0xC000, End: 0xDFFF},
	{Name: "hram", Start: 0xFF80, End: 0xFFFE},
}

func (r Region) length() int { return int(r.End-r.Start) + 1 }

// MemoryReader is the side-effect-free portion of the GomeBoy introspection API.
type MemoryReader interface {
	Peek8(addr uint16) byte
	PeekInto(addr uint16, dst []byte)
}

// ROMIdentity is enough to keep findings tied to one exact ROM revision.
type ROMIdentity struct {
	Title  string `json:"title,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	Model  string `json:"model,omitempty"`
}

// BankContext records the bank-select registers visible at capture time. The
// effective WRAM bank follows CGB SVBK semantics, where zero selects bank 1.
type BankContext struct {
	SVBKRaw  byte `json:"svbk_raw"`
	WRAMBank byte `json:"wram_bank"`
	VBKRaw   byte `json:"vbk_raw"`
	VRAMBank byte `json:"vram_bank"`
}

// CaptureMeta is metadata owned by the emulator/session rather than memory.
type CaptureMeta struct {
	ROM   ROMIdentity
	Frame uint64
	Cycle uint64
}

// RegionSnapshot stores one named captured memory range. Data is encoded as
// base64 by encoding/json, keeping snapshot files compact and machine-readable.
type RegionSnapshot struct {
	Name  string `json:"name"`
	Start uint16 `json:"start"`
	End   uint16 `json:"end"`
	Data  []byte `json:"data"`
}

// Snapshot is a named, side-effect-free memory observation.
type Snapshot struct {
	SchemaVersion int              `json:"schema_version"`
	Name          string           `json:"name"`
	ROM           ROMIdentity      `json:"rom"`
	Frame         uint64           `json:"frame"`
	Cycle         uint64           `json:"cycle"`
	Banks         BankContext      `json:"banks"`
	Regions       []RegionSnapshot `json:"regions"`
}

// Capture copies the requested regions from reader without mutating emulator state.
func Capture(reader MemoryReader, name string, meta CaptureMeta, regions []Region) (Snapshot, error) {
	if reader == nil {
		return Snapshot{}, errors.New("romtool: memory reader is nil")
	}
	if strings.TrimSpace(name) == "" {
		return Snapshot{}, errors.New("romtool: snapshot name is required")
	}
	if len(regions) == 0 {
		regions = DefaultRegions
	}
	if err := validateRegions(regions); err != nil {
		return Snapshot{}, err
	}

	svbk := reader.Peek8(0xFF70)
	wramBank := svbk & 0x07
	if wramBank == 0 {
		wramBank = 1
	}
	vbk := reader.Peek8(0xFF4F)
	snap := Snapshot{
		SchemaVersion: SchemaVersion,
		Name:          name,
		ROM:           meta.ROM,
		Frame:         meta.Frame,
		Cycle:         meta.Cycle,
		Banks: BankContext{
			SVBKRaw:  svbk,
			WRAMBank: wramBank,
			VBKRaw:   vbk,
			VRAMBank: vbk & 0x01,
		},
		Regions: make([]RegionSnapshot, 0, len(regions)),
	}
	for _, region := range regions {
		data := make([]byte, region.length())
		reader.PeekInto(region.Start, data)
		snap.Regions = append(snap.Regions, RegionSnapshot{
			Name: region.Name, Start: region.Start, End: region.End, Data: data,
		})
	}
	return snap, nil
}

func validateRegions(regions []Region) error {
	seen := map[string]struct{}{}
	for i, region := range regions {
		if strings.TrimSpace(region.Name) == "" {
			return fmt.Errorf("romtool: region %d has an empty name", i)
		}
		if region.End < region.Start {
			return fmt.Errorf("romtool: region %q ends before it starts", region.Name)
		}
		key := strings.ToLower(region.Name)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("romtool: duplicate region name %q", region.Name)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// WriteSnapshot serializes a snapshot as indented JSON.
func WriteSnapshot(w io.Writer, snap Snapshot) error {
	if err := ValidateSnapshot(snap); err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(snap)
}

// ReadSnapshot decodes and validates a snapshot.
func ReadSnapshot(r io.Reader) (Snapshot, error) {
	var snap Snapshot
	dec := json.NewDecoder(r)
	if err := dec.Decode(&snap); err != nil {
		return Snapshot{}, fmt.Errorf("romtool: decode snapshot: %w", err)
	}
	if err := ValidateSnapshot(snap); err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

// ValidateSnapshot checks the portable snapshot format independent of a ROM.
func ValidateSnapshot(s Snapshot) error {
	if s.SchemaVersion != SchemaVersion {
		return fmt.Errorf("romtool: unsupported snapshot schema %d (want %d)", s.SchemaVersion, SchemaVersion)
	}
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("romtool: snapshot name is required")
	}
	if len(s.Regions) == 0 {
		return errors.New("romtool: snapshot has no regions")
	}
	regions := make([]Region, 0, len(s.Regions))
	for _, region := range s.Regions {
		regions = append(regions, Region{Name: region.Name, Start: region.Start, End: region.End})
		if got, want := len(region.Data), int(region.End-region.Start)+1; region.End < region.Start || got != want {
			return fmt.Errorf("romtool: region %q has %d data bytes for %#04x..%#04x", region.Name, got, region.Start, region.End)
		}
	}
	return validateRegions(regions)
}

// DiffFilter narrows byte changes. Nil value predicates are ignored.
type DiffFilter struct {
	Region string
	Start  *uint16
	End    *uint16
	Before *byte
	After  *byte
	Delta  *int16
}

// Change is one changed byte between two snapshots.
type Change struct {
	Region     string `json:"region"`
	Address    uint16 `json:"address"`
	AddressHex string `json:"address_hex"`
	Bank       *int   `json:"bank,omitempty"`
	Before     byte   `json:"before"`
	After      byte   `json:"after"`
	Delta      int16  `json:"delta"`
}

// Diff compares snapshots of the same ROM and returns matching changes in
// region/address order.
func Diff(before, after Snapshot, filter DiffFilter) ([]Change, error) {
	if err := ValidateSnapshot(before); err != nil {
		return nil, fmt.Errorf("before: %w", err)
	}
	if err := ValidateSnapshot(after); err != nil {
		return nil, fmt.Errorf("after: %w", err)
	}
	if before.ROM.SHA256 != "" && after.ROM.SHA256 != "" && !strings.EqualFold(before.ROM.SHA256, after.ROM.SHA256) {
		return nil, fmt.Errorf("romtool: snapshots are from different ROMs (%s != %s)", before.ROM.SHA256, after.ROM.SHA256)
	}
	if filter.Start != nil && filter.End != nil && *filter.End < *filter.Start {
		return nil, errors.New("romtool: filter end is before start")
	}

	afterByName := make(map[string]RegionSnapshot, len(after.Regions))
	for _, region := range after.Regions {
		afterByName[strings.ToLower(region.Name)] = region
	}
	changes := make([]Change, 0)
	for _, br := range before.Regions {
		ar, ok := afterByName[strings.ToLower(br.Name)]
		if !ok || ar.Start != br.Start || ar.End != br.End {
			return nil, fmt.Errorf("romtool: region %q does not match between snapshots", br.Name)
		}
		if filter.Region != "" && !strings.EqualFold(filter.Region, br.Name) {
			continue
		}
		for i, bv := range br.Data {
			av := ar.Data[i]
			if bv == av {
				continue
			}
			addr := br.Start + uint16(i)
			delta := int16(av) - int16(bv)
			if filter.Start != nil && addr < *filter.Start || filter.End != nil && addr > *filter.End {
				continue
			}
			if filter.Before != nil && bv != *filter.Before || filter.After != nil && av != *filter.After || filter.Delta != nil && delta != *filter.Delta {
				continue
			}
			changes = append(changes, Change{
				Region: br.Name, Address: addr, AddressHex: fmt.Sprintf("0x%04X", addr),
				Bank: BankForAddress(addr, after.Banks), Before: bv, After: av, Delta: delta,
			})
		}
	}
	return changes, nil
}

// BankForAddress reports the active bank for memory ranges whose bank is meaningful.
func BankForAddress(addr uint16, banks BankContext) *int {
	var bank int
	switch {
	case addr >= 0xC000 && addr <= 0xCFFF:
		bank = 0
	case addr >= 0xD000 && addr <= 0xDFFF:
		bank = int(banks.WRAMBank)
	case addr >= 0x8000 && addr <= 0x9FFF:
		bank = int(banks.VRAMBank)
	case addr >= 0xFF80 && addr <= 0xFFFE:
		bank = 0
	default:
		return nil
	}
	return &bank
}

// Experiment is one named transition diff used for repeated comparison.
type Experiment struct {
	Name    string   `json:"name"`
	Changes []Change `json:"changes"`
}

// Candidate aggregates how one address behaved across repeated experiments.
type Candidate struct {
	Region          string       `json:"region"`
	Address         uint16       `json:"address"`
	AddressHex      string       `json:"address_hex"`
	Bank            *int         `json:"bank,omitempty"`
	Experiments     int          `json:"experiments"`
	Changed         int          `json:"changed"`
	ChangeRate      float64      `json:"change_rate"`
	ConsistentDelta bool         `json:"consistent_delta"`
	StableDelta     *int16       `json:"stable_delta,omitempty"`
	Score           float64      `json:"score"`
	Transitions     []Transition `json:"transitions"`
}

// Transition preserves the observed value pattern for one experiment.
type Transition struct {
	Experiment string `json:"experiment"`
	Before     byte   `json:"before"`
	After      byte   `json:"after"`
	Delta      int16  `json:"delta"`
}

// Rank aggregates repeated experiment diffs. exclude keys are addresses known
// to change in a control/no-input experiment and are omitted as noise.
func Rank(experiments []Experiment, exclude map[string]struct{}) []Candidate {
	type aggregate struct {
		candidate  Candidate
		firstDelta int16
		haveDelta  bool
	}
	byKey := map[string]*aggregate{}
	total := len(experiments)
	for _, exp := range experiments {
		seen := map[string]struct{}{}
		for _, change := range exp.Changes {
			key := ChangeKey(change.Region, change.Address)
			if _, drop := exclude[key]; drop {
				continue
			}
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			a := byKey[key]
			if a == nil {
				a = &aggregate{candidate: Candidate{
					Region: change.Region, Address: change.Address, AddressHex: change.AddressHex,
					Bank: change.Bank, Experiments: total, ConsistentDelta: true,
				}}
				byKey[key] = a
			}
			a.candidate.Changed++
			a.candidate.Transitions = append(a.candidate.Transitions, Transition{
				Experiment: exp.Name, Before: change.Before, After: change.After, Delta: change.Delta,
			})
			if !a.haveDelta {
				a.firstDelta, a.haveDelta = change.Delta, true
			} else if change.Delta != a.firstDelta {
				a.candidate.ConsistentDelta = false
			}
		}
	}

	out := make([]Candidate, 0, len(byKey))
	for _, a := range byKey {
		c := a.candidate
		if total > 0 {
			c.ChangeRate = float64(c.Changed) / float64(total)
		}
		if c.ConsistentDelta && a.haveDelta {
			delta := a.firstDelta
			c.StableDelta = &delta
		}
		c.Score = c.ChangeRate
		if c.ConsistentDelta && c.Changed > 1 {
			c.Score += 0.25
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Changed != out[j].Changed {
			return out[i].Changed > out[j].Changed
		}
		if out[i].Region != out[j].Region {
			return out[i].Region < out[j].Region
		}
		return out[i].Address < out[j].Address
	})
	return out
}

// ChangeKey is stable across diff/compare commands and suitable for control-noise sets.
func ChangeKey(region string, address uint16) string {
	return fmt.Sprintf("%s:%04x", strings.ToLower(region), address)
}

// Label assigns profile semantics to a candidate address.
type Label struct {
	Region  string `json:"region,omitempty"`
	Address uint16 `json:"address"`
	Name    string `json:"name"`
	Note    string `json:"note,omitempty"`
}

// Symbol is a profile-friendly semantic memory symbol.
type Symbol struct {
	Name       string `json:"name"`
	Region     string `json:"region"`
	Address    uint16 `json:"address"`
	AddressHex string `json:"address_hex"`
	Bank       *int   `json:"bank,omitempty"`
	Note       string `json:"note,omitempty"`
}

// SymbolFile is intentionally independent of the not-yet-landed GameProfile
// interface: a profile implementation can consume or translate this stable data.
type SymbolFile struct {
	SchemaVersion int         `json:"schema_version"`
	ROM           ROMIdentity `json:"rom"`
	Symbols       []Symbol    `json:"symbols"`
}

// ExportSymbols turns confirmed labels into a deterministic symbol file.
func ExportSymbols(rom ROMIdentity, candidates []Candidate, labels []Label) (SymbolFile, error) {
	lookup := make(map[string]Candidate, len(candidates))
	byAddress := make(map[uint16][]Candidate)
	for _, c := range candidates {
		lookup[ChangeKey(c.Region, c.Address)] = c
		byAddress[c.Address] = append(byAddress[c.Address], c)
	}
	symbols := make([]Symbol, 0, len(labels))
	names := map[string]struct{}{}
	for _, label := range labels {
		if strings.TrimSpace(label.Name) == "" {
			return SymbolFile{}, fmt.Errorf("romtool: empty symbol name at 0x%04X", label.Address)
		}
		if _, exists := names[label.Name]; exists {
			return SymbolFile{}, fmt.Errorf("romtool: duplicate symbol name %q", label.Name)
		}
		names[label.Name] = struct{}{}
		var candidate Candidate
		var ok bool
		if label.Region != "" {
			candidate, ok = lookup[ChangeKey(label.Region, label.Address)]
		} else {
			matches := byAddress[label.Address]
			if len(matches) == 1 {
				candidate, ok = matches[0], true
			} else if len(matches) > 1 {
				return SymbolFile{}, fmt.Errorf("romtool: address 0x%04X occurs in multiple regions; label must name a region", label.Address)
			}
		}
		if !ok {
			return SymbolFile{}, fmt.Errorf("romtool: label %q points to 0x%04X which is not a candidate", label.Name, label.Address)
		}
		symbols = append(symbols, Symbol{
			Name: label.Name, Region: candidate.Region, Address: candidate.Address,
			AddressHex: candidate.AddressHex, Bank: candidate.Bank, Note: label.Note,
		})
	}
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].Name != symbols[j].Name {
			return symbols[i].Name < symbols[j].Name
		}
		return symbols[i].Address < symbols[j].Address
	})
	return SymbolFile{SchemaVersion: SchemaVersion, ROM: rom, Symbols: symbols}, nil
}
