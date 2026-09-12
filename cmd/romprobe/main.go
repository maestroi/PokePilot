// Command romprobe is PokePilot's developer-facing ROM reverse-engineering tool.
package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
	"github.com/maestroi/pokepilot/romtool"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(usageText())
	}
	switch args[0] {
	case "capture":
		return runCapture(args[1:], out)
	case "diff":
		return runDiff(args[1:], out)
	case "compare":
		return runCompare(args[1:], out)
	case "experiment":
		return runExperiment(args[1:], out)
	case "help", "-h", "--help":
		_, _ = fmt.Fprintln(out, usageText())
		return nil
	default:
		return fmt.Errorf("romprobe: unknown command %q\n%s", args[0], usageText())
	}
}

func usageText() string {
	return `usage: romprobe COMMAND [options]

commands:
  capture     capture named WRAM/HRAM ranges from a ROM/save-state
  diff        diff two snapshot JSON files with address/value filters
  compare     rank addresses across two or more snapshot transitions
  experiment  run deterministic button experiments from one baseline state`
}

func runCapture(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("romprobe capture", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	romPath := fs.String("rom", "", "path to GB/GBC ROM")
	statePath := fs.String("state", "", "optional raw or checked GomeBoy state")
	name := fs.String("name", "", "checkpoint name")
	outputPath := fs.String("out", "", "snapshot JSON path; omit for stdout")
	regionSpec := fs.String("regions", "wram:C000-DFFF,hram:FF80-FFFE", "comma-separated name:START-END ranges")
	warmup := fs.Int("warmup", 0, "frames to advance before capture")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("romprobe capture: %w", err)
	}
	if *romPath == "" || strings.TrimSpace(*name) == "" {
		return errors.New("romprobe capture: -rom and -name are required")
	}
	if *warmup < 0 {
		return errors.New("romprobe capture: -warmup must be >= 0")
	}
	regions, err := parseRegions(*regionSpec)
	if err != nil {
		return fmt.Errorf("romprobe capture: %w", err)
	}

	e, err := gomeboy.New(gomeboy.WithROM(*romPath), gomeboy.Headless(), gomeboy.WithoutVideo())
	if err != nil {
		return fmt.Errorf("romprobe capture: open ROM: %w", err)
	}
	defer e.Close()
	if *statePath != "" {
		if err := loadState(e, *statePath); err != nil {
			return fmt.Errorf("romprobe capture: %w", err)
		}
	}
	if *warmup > 0 {
		e.StepFrames(*warmup)
	}

	identity := romIdentity(e)
	snap, err := romtool.Capture(e, *name, romtool.CaptureMeta{ROM: identity, Frame: e.FrameCount(), Cycle: e.Cycle()}, regions)
	if err != nil {
		return fmt.Errorf("romprobe capture: %w", err)
	}
	if *outputPath == "" {
		return romtool.WriteSnapshot(out, snap)
	}
	f, err := os.Create(*outputPath)
	if err != nil {
		return fmt.Errorf("romprobe capture: create %s: %w", *outputPath, err)
	}
	if err := romtool.WriteSnapshot(f, snap); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("romprobe capture: close %s: %w", *outputPath, err)
	}
	fmt.Fprintf(out, "captured %s frame=%d cycle=%d rom=%s -> %s\n", snap.Name, snap.Frame, snap.Cycle, shortHash(snap.ROM.SHA256), *outputPath)
	return nil
}

func romIdentity(e *gomeboy.Emulator) romtool.ROMIdentity {
	sum := e.ROMSHA256()
	cart := e.Cartridge()
	return romtool.ROMIdentity{Title: cart.Title, SHA256: hex.EncodeToString(sum[:]), Model: fmt.Sprint(e.Model())}
}

func loadState(e *gomeboy.Emulator, path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read state %s: %w", path, err)
	}
	if bytes.HasPrefix(b, []byte("GMBSTATE")) {
		if err := e.LoadStateChecked(b); err != nil {
			return fmt.Errorf("load checked state %s: %w", path, err)
		}
		return nil
	}
	if err := e.LoadState(b); err != nil {
		return fmt.Errorf("load state %s: %w", path, err)
	}
	return nil
}
