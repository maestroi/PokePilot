package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/maestroi/pokepilot/romtool"
)

func parseRegions(spec string) ([]romtool.Region, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, errors.New("regions must not be empty")
	}
	parts := strings.Split(spec, ",")
	regions := make([]romtool.Region, 0, len(parts))
	for i, raw := range parts {
		part := strings.TrimSpace(raw)
		colon := strings.IndexByte(part, ':')
		if colon <= 0 {
			return nil, fmt.Errorf("region %d %q must be NAME:START-END", i+1, part)
		}
		name := strings.TrimSpace(part[:colon])
		bounds := strings.SplitN(strings.TrimSpace(part[colon+1:]), "-", 2)
		if len(bounds) != 2 {
			return nil, fmt.Errorf("region %q must use START-END", name)
		}
		start, err := parseUint16(bounds[0])
		if err != nil {
			return nil, fmt.Errorf("region %q start: %w", name, err)
		}
		end, err := parseUint16(bounds[1])
		if err != nil {
			return nil, fmt.Errorf("region %q end: %w", name, err)
		}
		if end < start {
			return nil, fmt.Errorf("region %q ends before it starts", name)
		}
		regions = append(regions, romtool.Region{Name: name, Start: start, End: end})
	}
	return regions, nil
}

func readSnapshotFile(path string) (romtool.Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return romtool.Snapshot{}, fmt.Errorf("romprobe: open %s: %w", path, err)
	}
	defer f.Close()
	snap, err := romtool.ReadSnapshot(f)
	if err != nil {
		return romtool.Snapshot{}, fmt.Errorf("romprobe: read %s: %w", path, err)
	}
	return snap, nil
}

func prepareSymbols(path string, rom romtool.ROMIdentity, candidates []romtool.Candidate, rawLabels []string) ([]romtool.Symbol, error) {
	if len(rawLabels) == 0 {
		if path != "" {
			return nil, errors.New("romprobe: -symbols requires at least one -label")
		}
		return nil, nil
	}
	labels := make([]romtool.Label, 0, len(rawLabels))
	for _, raw := range rawLabels {
		label, err := parseLabel(raw)
		if err != nil {
			return nil, fmt.Errorf("romprobe: %w", err)
		}
		labels = append(labels, label)
	}
	sf, err := romtool.ExportSymbols(rom, candidates, labels)
	if err != nil {
		return nil, fmt.Errorf("romprobe: attach labels: %w", err)
	}
	if path != "" {
		f, err := os.Create(path)
		if err != nil {
			return nil, fmt.Errorf("romprobe: create symbols %s: %w", path, err)
		}
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(sf); err != nil {
			_ = f.Close()
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
	}
	return sf.Symbols, nil
}

func parseLabel(raw string) (romtool.Label, error) {
	left, right, ok := strings.Cut(raw, "=")
	if !ok {
		return romtool.Label{}, fmt.Errorf("label %q must be [REGION@]ADDRESS=NAME[:NOTE]", raw)
	}
	var region string
	if r, a, found := strings.Cut(left, "@"); found {
		region = strings.TrimSpace(r)
		left = a
	}
	addr, err := parseUint16(left)
	if err != nil {
		return romtool.Label{}, fmt.Errorf("label %q address: %w", raw, err)
	}
	name, note := strings.TrimSpace(right), ""
	if n, rest, found := strings.Cut(name, ":"); found {
		name, note = strings.TrimSpace(n), strings.TrimSpace(rest)
	}
	if name == "" {
		return romtool.Label{}, fmt.Errorf("label %q has empty name", raw)
	}
	return romtool.Label{Region: region, Address: addr, Name: name, Note: note}, nil
}

func shortHash(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	if s == "" {
		return "unknown"
	}
	return s
}
func parseUint16(raw string) (uint16, error) {
	s := strings.TrimSpace(raw)
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		if v, err := strconv.ParseUint(s, 10, 16); err == nil {
			return uint16(v), nil
		}
		s = "0x" + s
	}
	v, err := strconv.ParseUint(s, 0, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid address %q", raw)
	}
	return uint16(v), nil
}

type optionalUint16 struct {
	value uint16
	set   bool
}

func (o *optionalUint16) String() string {
	if !o.set {
		return ""
	}
	return fmt.Sprintf("0x%04X", o.value)
}
func (o *optionalUint16) Set(s string) error {
	v, e := parseUint16(s)
	if e == nil {
		o.value, o.set = v, true
	}
	return e
}

type optionalByte struct {
	value byte
	set   bool
}

func (o *optionalByte) String() string {
	if !o.set {
		return ""
	}
	return fmt.Sprintf("0x%02X", o.value)
}
func (o *optionalByte) Set(s string) error {
	v, e := strconv.ParseUint(strings.TrimSpace(s), 0, 8)
	if e != nil {
		return fmt.Errorf("invalid byte %q", s)
	}
	o.value, o.set = byte(v), true
	return nil
}

type optionalInt16 struct {
	value int16
	set   bool
}

func (o *optionalInt16) String() string {
	if !o.set {
		return ""
	}
	return strconv.Itoa(int(o.value))
}
func (o *optionalInt16) Set(s string) error {
	v, e := strconv.ParseInt(strings.TrimSpace(s), 0, 16)
	if e != nil {
		return fmt.Errorf("invalid delta %q", s)
	}
	o.value, o.set = int16(v), true
	return nil
}

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

type intList []int

func (s *intList) String() string {
	parts := make([]string, len(*s))
	for i, v := range *s {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ",")
}
func (s *intList) Set(raw string) error {
	v, e := strconv.Atoi(raw)
	if e != nil {
		return fmt.Errorf("invalid integer %q", raw)
	}
	*s = append(*s, v)
	return nil
}
