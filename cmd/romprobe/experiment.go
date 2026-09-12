package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
	probe "github.com/maestroi/gomeboy/pkg/memprobe"
	"github.com/maestroi/pokepilot/romtool"
)

type experimentReport struct {
	ROM        romtool.ROMIdentity  `json:"rom"`
	Banks      romtool.BankContext  `json:"banks"`
	Results    []romtool.Experiment `json:"results"`
	Candidates []romtool.Candidate  `json:"candidates"`
	Symbols    []romtool.Symbol     `json:"symbols,omitempty"`
}

func runExperiment(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("romprobe experiment", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	romPath := fs.String("rom", "", "path to GB/GBC ROM")
	statePath := fs.String("state", "", "optional raw or checked GomeBoy state")
	regionSpec := fs.String("regions", "wram:C000-DFFF,hram:FF80-FFFE", "comma-separated name:START-END ranges")
	actionSpec := fs.String("actions", "control,right,down", "comma-separated control/up/down/left/right/a/b/start/select")
	warmup := fs.Int("warmup", 0, "frames before baseline")
	hold := fs.Int("hold", 1, "frames to hold a button")
	settle := fs.Int("settle", 7, "frames after releasing a button")
	repeat := fs.Int("repeat", 1, "repeat each non-control experiment from the same baseline")
	af := addAnalysisFlags(fs)
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("romprobe experiment: %w", err)
	}
	if *romPath == "" {
		return errors.New("romprobe experiment: -rom is required")
	}
	if *warmup < 0 || *hold < 0 || *settle < 0 || *repeat < 1 {
		return errors.New("romprobe experiment: frame counts must be >= 0 and -repeat >= 1")
	}
	if err := af.validate(); err != nil {
		return fmt.Errorf("romprobe experiment: %w", err)
	}
	regions, err := parseRegions(*regionSpec)
	if err != nil {
		return err
	}
	actions, controls, err := parseActions(*actionSpec, *hold, *settle, *repeat)
	if err != nil {
		return err
	}

	e, err := gomeboy.New(gomeboy.WithROM(*romPath), gomeboy.Headless(), gomeboy.WithoutVideo())
	if err != nil {
		return fmt.Errorf("romprobe experiment: open ROM: %w", err)
	}
	defer e.Close()
	if *statePath != "" {
		if err := loadState(e, *statePath); err != nil {
			return fmt.Errorf("romprobe experiment: %w", err)
		}
	}
	if *warmup > 0 {
		e.StepFrames(*warmup)
	}

	identity := romIdentity(e)
	baseline, err := romtool.Capture(e, "experiment-baseline", romtool.CaptureMeta{ROM: identity, Frame: e.FrameCount(), Cycle: e.Cycle()}, regions)
	if err != nil {
		return err
	}
	pRegions := make([]probe.Region, 0, len(regions))
	for _, r := range regions {
		pRegions = append(pRegions, probe.Region{Name: r.Name, Start: r.Start, Length: int(r.End-r.Start) + 1})
	}
	results, err := probe.Run(e, pRegions, actions)
	if err != nil {
		return fmt.Errorf("romprobe experiment: %w", err)
	}

	exps := make([]romtool.Experiment, 0, len(results))
	exclude := map[string]struct{}{}
	filter := af.filter()
	for _, result := range results {
		changes := make([]romtool.Change, 0, len(result.Changes))
		for _, c := range result.Changes {
			if !matchesProbeFilter(c.Region, c.Address, c.Before, c.After, c.Delta, filter) {
				continue
			}
			changes = append(changes, romtool.Change{Region: c.Region, Address: c.Address, AddressHex: fmt.Sprintf("0x%04X", c.Address), Bank: romtool.BankForAddress(c.Address, baseline.Banks), Before: c.Before, After: c.After, Delta: c.Delta})
		}
		exp := romtool.Experiment{Name: result.Action, Changes: changes}
		if controls[result.Action] {
			for _, c := range changes {
				exclude[romtool.ChangeKey(c.Region, c.Address)] = struct{}{}
			}
			continue
		}
		exps = append(exps, exp)
	}
	candidates := romtool.Rank(exps, exclude)
	symbols, err := prepareSymbols(af.symbols, identity, candidates, af.labels)
	if err != nil {
		return err
	}
	report := experimentReport{ROM: identity, Banks: baseline.Banks, Results: exps, Candidates: candidates, Symbols: symbols}
	if af.json {
		return writeJSON(out, report)
	}
	fmt.Fprintf(out, "ROM %s (%s), baseline frame=%d, WRAM bank=%d; %d experiment(s), %d candidate(s)\n", identity.Title, shortHash(identity.SHA256), baseline.Frame, baseline.Banks.WRAMBank, len(exps), len(candidates))
	writeCandidates(out, candidates, af.limit)
	writeSymbols(out, symbols)
	return nil
}

func matchesProbeFilter(region string, addr uint16, before, after byte, delta int16, f romtool.DiffFilter) bool {
	if f.Region != "" && !strings.EqualFold(f.Region, region) {
		return false
	}
	if f.Start != nil && addr < *f.Start || f.End != nil && addr > *f.End {
		return false
	}
	if f.Before != nil && before != *f.Before || f.After != nil && after != *f.After || f.Delta != nil && delta != *f.Delta {
		return false
	}
	return true
}

func parseActions(spec string, hold, settle, repeat int) ([]probe.Action, map[string]bool, error) {
	buttons := map[string]gomeboy.Button{"a": gomeboy.ButtonA, "b": gomeboy.ButtonB, "start": gomeboy.ButtonStart, "select": gomeboy.ButtonSelect, "up": gomeboy.ButtonUp, "down": gomeboy.ButtonDown, "left": gomeboy.ButtonLeft, "right": gomeboy.ButtonRight}
	var actions []probe.Action
	controls := map[string]bool{}
	for _, raw := range strings.Split(spec, ",") {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			return nil, nil, errors.New("empty experiment action")
		}
		if name == "control" || name == "wait" || name == "none" {
			actionName := "control"
			if controls[actionName] {
				actionName = fmt.Sprintf("control-%d", len(controls)+1)
			}
			actions = append(actions, probe.Wait(actionName, hold+settle))
			controls[actionName] = true
			continue
		}
		button, ok := buttons[name]
		if !ok {
			return nil, nil, fmt.Errorf("unknown action %q", name)
		}
		for n := 1; n <= repeat; n++ {
			actionName := name
			if repeat > 1 {
				actionName = fmt.Sprintf("%s-%d", name, n)
			}
			actions = append(actions, probe.Tap(actionName, button, hold, settle))
		}
	}
	if len(actions) == 0 {
		return nil, nil, errors.New("no actions")
	}
	return actions, controls, nil
}
