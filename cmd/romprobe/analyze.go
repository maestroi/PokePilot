package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/maestroi/pokepilot/romtool"
)

type commonAnalysisFlags struct {
	region        string
	start, end    optionalUint16
	before, after optionalByte
	delta         optionalInt16
	limit         int
	json          bool
	labels        stringList
	symbols       string
}

func addAnalysisFlags(fs *flag.FlagSet) *commonAnalysisFlags {
	f := &commonAnalysisFlags{}
	fs.StringVar(&f.region, "region", "", "only this named region")
	fs.Var(&f.start, "start", "first address (decimal or 0xHEX)")
	fs.Var(&f.end, "end", "last address, inclusive")
	fs.Var(&f.before, "before", "only changes from this byte value")
	fs.Var(&f.after, "after", "only changes to this byte value")
	fs.Var(&f.delta, "delta", "only changes with this signed delta")
	fs.IntVar(&f.limit, "limit", 128, "maximum rows to print; 0 means unlimited")
	fs.BoolVar(&f.json, "json", false, "emit machine-readable JSON")
	fs.Var(&f.labels, "label", "confirmed symbol: [REGION@]ADDRESS=NAME[:NOTE] (repeatable)")
	fs.StringVar(&f.symbols, "symbols", "", "write confirmed labels as a profile-friendly symbol JSON file")
	return f
}

func (f *commonAnalysisFlags) filter() romtool.DiffFilter {
	var start, end *uint16
	var before, after *byte
	var delta *int16
	if f.start.set {
		v := f.start.value
		start = &v
	}
	if f.end.set {
		v := f.end.value
		end = &v
	}
	if f.before.set {
		v := f.before.value
		before = &v
	}
	if f.after.set {
		v := f.after.value
		after = &v
	}
	if f.delta.set {
		v := f.delta.value
		delta = &v
	}
	return romtool.DiffFilter{Region: f.region, Start: start, End: end, Before: before, After: after, Delta: delta}
}
func (f *commonAnalysisFlags) validate() error {
	if f.limit < 0 {
		return errors.New("-limit must be >= 0")
	}
	return nil
}

type diffReport struct {
	ROM         romtool.ROMIdentity `json:"rom"`
	Before      string              `json:"before"`
	After       string              `json:"after"`
	BeforeBanks romtool.BankContext `json:"before_banks"`
	AfterBanks  romtool.BankContext `json:"after_banks"`
	Changes     []romtool.Change    `json:"changes"`
	Symbols     []romtool.Symbol    `json:"symbols,omitempty"`
}

func runDiff(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("romprobe diff", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	af := addAnalysisFlags(fs)
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("romprobe diff: %w", err)
	}
	if err := af.validate(); err != nil {
		return fmt.Errorf("romprobe diff: %w", err)
	}
	if fs.NArg() != 2 {
		return errors.New("usage: romprobe diff [filters] BEFORE.json AFTER.json")
	}
	before, err := readSnapshotFile(fs.Arg(0))
	if err != nil {
		return err
	}
	after, err := readSnapshotFile(fs.Arg(1))
	if err != nil {
		return err
	}
	changes, err := romtool.Diff(before, after, af.filter())
	if err != nil {
		return fmt.Errorf("romprobe diff: %w", err)
	}
	symbols, err := prepareSymbols(af.symbols, after.ROM, candidatesFromChanges(changes, before.Name+"->"+after.Name), af.labels)
	if err != nil {
		return err
	}
	report := diffReport{ROM: after.ROM, Before: before.Name, After: after.Name, BeforeBanks: before.Banks, AfterBanks: after.Banks, Changes: changes, Symbols: symbols}
	if af.json {
		return writeJSON(out, report)
	}
	fmt.Fprintf(out, "%s -> %s  ROM %s  WRAM bank %d -> %d\n", before.Name, after.Name, shortHash(after.ROM.SHA256), before.Banks.WRAMBank, after.Banks.WRAMBank)
	writeChanges(out, changes, af.limit)
	writeSymbols(out, symbols)
	return nil
}

type compareReport struct {
	ROM         romtool.ROMIdentity  `json:"rom"`
	Experiments []romtool.Experiment `json:"experiments"`
	Candidates  []romtool.Candidate  `json:"candidates"`
	Symbols     []romtool.Symbol     `json:"symbols,omitempty"`
}

func runCompare(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("romprobe compare", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	af := addAnalysisFlags(fs)
	var controlPairs intList
	fs.Var(&controlPairs, "control-pair", "1-based pair to treat as no-input noise (repeatable)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("romprobe compare: %w", err)
	}
	if err := af.validate(); err != nil {
		return fmt.Errorf("romprobe compare: %w", err)
	}
	if fs.NArg() < 4 || fs.NArg()%2 != 0 {
		return errors.New("usage: romprobe compare [filters] BEFORE1 AFTER1 BEFORE2 AFTER2 [...]")
	}

	var rom romtool.ROMIdentity
	experiments := make([]romtool.Experiment, 0, fs.NArg()/2)
	control := map[string]struct{}{}
	controlSet := map[int]struct{}{}
	for _, n := range controlPairs {
		if n < 1 || n > fs.NArg()/2 {
			return fmt.Errorf("romprobe compare: -control-pair %d is out of range", n)
		}
		controlSet[n-1] = struct{}{}
	}
	for i := 0; i < fs.NArg(); i += 2 {
		before, err := readSnapshotFile(fs.Arg(i))
		if err != nil {
			return err
		}
		after, err := readSnapshotFile(fs.Arg(i + 1))
		if err != nil {
			return err
		}
		changes, err := romtool.Diff(before, after, af.filter())
		if err != nil {
			return fmt.Errorf("romprobe compare pair %d: %w", i/2+1, err)
		}
		if rom.SHA256 == "" {
			rom = after.ROM
		} else if after.ROM.SHA256 != "" && !strings.EqualFold(rom.SHA256, after.ROM.SHA256) {
			return errors.New("romprobe compare: pairs are from different ROMs")
		}
		exp := romtool.Experiment{Name: before.Name + "->" + after.Name, Changes: changes}
		if _, isControl := controlSet[i/2]; isControl {
			for _, c := range changes {
				control[romtool.ChangeKey(c.Region, c.Address)] = struct{}{}
			}
			continue
		}
		experiments = append(experiments, exp)
	}
	candidates := romtool.Rank(experiments, control)
	symbols, err := prepareSymbols(af.symbols, rom, candidates, af.labels)
	if err != nil {
		return err
	}
	report := compareReport{ROM: rom, Experiments: experiments, Candidates: candidates, Symbols: symbols}
	if af.json {
		return writeJSON(out, report)
	}
	fmt.Fprintf(out, "%d experiment(s), %d ranked candidate(s), ROM %s\n", len(experiments), len(candidates), shortHash(rom.SHA256))
	writeCandidates(out, candidates, af.limit)
	writeSymbols(out, symbols)
	return nil
}

func candidatesFromChanges(changes []romtool.Change, name string) []romtool.Candidate {
	return romtool.Rank([]romtool.Experiment{{Name: name, Changes: changes}}, nil)
}
func writeJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
func writeChanges(out io.Writer, changes []romtool.Change, limit int) {
	n := len(changes)
	if limit > 0 && n > limit {
		n = limit
	}
	for _, c := range changes[:n] {
		bank := "-"
		if c.Bank != nil {
			bank = strconv.Itoa(*c.Bank)
		}
		fmt.Fprintf(out, "%s  %-5s bank=%s  %02X -> %02X  delta=%+d\n", c.AddressHex, c.Region, bank, c.Before, c.After, c.Delta)
	}
	if n < len(changes) {
		fmt.Fprintf(out, "... %d more change(s)\n", len(changes)-n)
	}
	fmt.Fprintf(out, "%d changed address(es)\n", len(changes))
}
func writeCandidates(out io.Writer, candidates []romtool.Candidate, limit int) {
	n := len(candidates)
	if limit > 0 && n > limit {
		n = limit
	}
	for i, c := range candidates[:n] {
		delta := "mixed"
		if c.StableDelta != nil {
			delta = fmt.Sprintf("%+d", *c.StableDelta)
		}
		fmt.Fprintf(out, "%3d  %s  %-5s score=%.2f changed=%d/%d delta=%s\n", i+1, c.AddressHex, c.Region, c.Score, c.Changed, c.Experiments, delta)
	}
	if n < len(candidates) {
		fmt.Fprintf(out, "... %d more candidate(s)\n", len(candidates)-n)
	}
}
func writeSymbols(out io.Writer, symbols []romtool.Symbol) {
	for _, s := range symbols {
		fmt.Fprintf(out, "label  %s = %s@%s", s.Name, s.Region, s.AddressHex)
		if s.Note != "" {
			fmt.Fprintf(out, "  # %s", s.Note)
		}
		fmt.Fprintln(out)
	}
}
