// Package canongen derives a canon.View from two vendored pret Gen-I
// decompilations: the canonical engine (pokered) and a native revision
// (pokeyellow). It reads each tree's RGBDS .sym file for RAM symbols and its
// event/toggle constant tables for flag numbering, so no address or flag index
// in the generated view is hand-counted.
package canongen

import (
	"bufio"
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/maestroi/pokepilot/gen1/canon"
)

// Tree is one vendored decompilation checkout.
type Tree struct {
	Dir string // e.g. "pokered"
	Sym string // .sym file name inside Dir, e.g. "pokered.sym"
}

// Result is a generated view plus the facts a reviewer needs to trust it.
type Result struct {
	View canon.View
	// Dropped lists canonical RAM symbols with no native counterpart; their
	// bytes read as zero through the view.
	Dropped []string
	// Truncated lists canonical symbols whose native storage is shorter; only
	// the leading native-sized bytes are mapped.
	Truncated []string
}

const (
	wramStart = 0xC000
	echoStart = 0xE000
	hramStart = 0xFF80
	hramEnd   = 0xFFFF
)

var symLine = regexp.MustCompile(`^([0-9a-fA-F]{2}):([0-9a-fA-F]{4}) (\S+)$`)

// ramSymbols returns name->address for global WRAM/HRAM symbols.
func ramSymbols(path string) (map[string]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]int{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := symLine.FindStringSubmatch(strings.TrimSpace(sc.Text()))
		if m == nil || strings.Contains(m[3], ".") {
			continue
		}
		addr, _ := strconv.ParseInt(m[2], 16, 32)
		a := int(addr)
		if (a >= wramStart && a < echoStart) || (a >= hramStart && a < hramEnd) {
			out[m[3]] = a
		}
	}
	return out, sc.Err()
}

// constTable evaluates an RGBDS const_def/const/const_skip/const_next table.
func constTable(path string) (map[string]int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	num := func(s string) (int, error) {
		n, err := strconv.ParseInt(strings.Replace(s, "$", "0x", 1), 0, 32)
		return int(n), err
	}
	out := map[string]int{}
	next := 0
	for _, line := range strings.Split(string(data), "\n") {
		if i := strings.IndexByte(line, ';'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "const_def":
			next = 0
			if len(fields) > 1 {
				if next, err = num(fields[1]); err != nil {
					return nil, fmt.Errorf("%s: %q: %w", path, line, err)
				}
			}
		case "const":
			out[strings.TrimSuffix(fields[1], ",")] = next
			next++
		case "const_skip":
			step := 1
			if len(fields) > 1 {
				if step, err = num(fields[1]); err != nil {
					return nil, fmt.Errorf("%s: %q: %w", path, line, err)
				}
			}
			next += step
		case "const_next":
			if next, err = num(fields[1]); err != nil {
				return nil, fmt.Errorf("%s: %q: %w", path, line, err)
			}
		}
	}
	return out, nil
}

// Generate builds the view that reads native in canonical coordinates.
func Generate(canonical, native Tree) (Result, error) {
	var res Result
	cs, err := ramSymbols(filepath.Join(canonical.Dir, canonical.Sym))
	if err != nil {
		return res, err
	}
	ns, err := ramSymbols(filepath.Join(native.Dir, native.Sym))
	if err != nil {
		return res, err
	}

	// Canonical boundaries: every canonical symbol start, plus region ends.
	byAddr := map[int][]string{}
	for name, a := range cs {
		byAddr[a] = append(byAddr[a], name)
	}
	bounds := []int{echoStart, hramEnd}
	for a := range byAddr {
		bounds = append(bounds, a)
		sort.Strings(byAddr[a])
	}
	sort.Ints(bounds)

	// nativeAt returns the native address shared by the canonical symbols at
	// a, or -1 when none of them exists natively.
	nativeAt := func(a int) (int, error) {
		at := -1
		for _, name := range byAddr[a] {
			n, ok := ns[name]
			if !ok {
				continue
			}
			if at >= 0 && at != n {
				return 0, fmt.Errorf("canonical %#04x aliases %v at different native addresses", a, byAddr[a])
			}
			at = n
		}
		return at, nil
	}

	for i, a := range bounds {
		if _, ok := byAddr[a]; !ok || i+1 == len(bounds) {
			continue
		}
		end := bounds[i+1]
		if a < echoStart && end > echoStart {
			end = echoStart
		}
		n, err := nativeAt(a)
		if err != nil {
			return res, err
		}
		if n < 0 {
			res.Dropped = append(res.Dropped, strings.Join(byAddr[a], "/"))
			continue
		}
		size := end - a
		// Bound by the native address of the next canonical symbol that
		// exists natively: yellow-only labels inside a span are
		// subdivisions, not size changes.
		for j := i + 1; j < len(bounds); j++ {
			nn, err := nativeAt(bounds[j])
			if err != nil {
				return res, err
			}
			if nn < 0 {
				if _, sym := byAddr[bounds[j]]; sym {
					continue
				}
				break // a region end
			}
			if nn > n && nn-n < size {
				res.Truncated = append(res.Truncated, fmt.Sprintf("%s (%d of %d bytes)", strings.Join(byAddr[a], "/"), nn-n, size))
				size = nn - n
			}
			break
		}
		res.View.Spans = appendSpan(res.View.Spans, canon.Span{Canon: uint16(a), Native: uint16(n), Len: uint16(size)})
	}

	events, err := flagArray(canonical, native, cs, ns, "event_constants.asm", "wEventFlags", false)
	if err != nil {
		return res, err
	}
	toggles, err := flagArray(canonical, native, cs, ns, "toggle_constants.asm", "wToggleableObjectFlags", true)
	if err != nil {
		return res, err
	}
	res.View.Flags = []canon.FlagArray{events.array, toggles.array}
	listC, okC := cs["wToggleableObjectList"]
	listN, okN := ns["wToggleableObjectList"]
	if !okC || !okN {
		return res, fmt.Errorf("wToggleableObjectList missing from a symbol table")
	}
	res.View.Lists = []canon.IndexList{{
		Canon: uint16(listC), Native: uint16(listN),
		Len: 16*2 + 1, Stride: 2, ValueOffset: 1, Terminator: 0xff,
		NativeToCanon: toggles.nativeToCanon,
	}}
	return res, nil
}

// appendSpan merges s into the previous span when both are contiguous.
func appendSpan(spans []canon.Span, s canon.Span) []canon.Span {
	if n := len(spans); n > 0 {
		p := &spans[n-1]
		if int(p.Canon)+int(p.Len) == int(s.Canon) && int(p.Native)+int(p.Len) == int(s.Native) {
			p.Len += s.Len
			return spans
		}
	}
	return append(spans, s)
}

type flagMap struct {
	array         canon.FlagArray
	nativeToCanon []int16
}

// flagArray renumbers a flag table by constant name. With allocateNative,
// native-only flags are given unused canonical indices so a canonical
// consumer still sees their state (toggleable objects the native game hides).
func flagArray(canonical, native Tree, cs, ns map[string]int, table, symbol string, allocateNative bool) (flagMap, error) {
	var fm flagMap
	ct, err := constTable(filepath.Join(canonical.Dir, "constants", table))
	if err != nil {
		return fm, err
	}
	nt, err := constTable(filepath.Join(native.Dir, "constants", table))
	if err != nil {
		return fm, err
	}
	cAddr, okC := cs[symbol]
	nAddr, okN := ns[symbol]
	if !okC || !okN {
		return fm, fmt.Errorf("%s missing from a symbol table", symbol)
	}
	bits := 8 * (nextSymbol(cs, cAddr) - cAddr)
	nbits := 8 * (nextSymbol(ns, nAddr) - nAddr)
	c2n := make([]int16, bits)
	for i := range c2n {
		c2n[i] = -1
	}
	n2c := make([]int16, nbits)
	for i := range n2c {
		n2c[i] = -1
	}
	used := make([]bool, bits)
	for name, c := range ct {
		if c < 0 || c >= bits {
			continue
		}
		used[c] = true
		if n, ok := nt[name]; ok && n >= 0 && n < nbits {
			c2n[c] = int16(n)
			n2c[n] = int16(c)
		}
	}
	if allocateNative {
		var names []string
		for name, n := range nt {
			if _, ok := ct[name]; !ok && n >= 0 && n < nbits {
				names = append(names, name)
			}
		}
		sort.Slice(names, func(i, j int) bool { return nt[names[i]] < nt[names[j]] })
		free := 0
		for _, name := range names {
			for free < bits && used[free] {
				free++
			}
			if free == bits {
				return fm, fmt.Errorf("%s: no free canonical index for native-only %s", table, name)
			}
			used[free] = true
			c2n[free] = int16(nt[name])
			n2c[nt[name]] = int16(free)
		}
	}
	fm.array = canon.FlagArray{Canon: uint16(cAddr), Native: uint16(nAddr), CanonToNative: c2n}
	fm.nativeToCanon = n2c
	return fm, nil
}

func nextSymbol(syms map[string]int, a int) int {
	next := 0x10000
	for _, v := range syms {
		if v > a && v < next {
			next = v
		}
	}
	return next
}

// Source renders r as a Go file declaring `var <name> = canon.View{...}`.
func Source(pkg, name, generator string, r Result) ([]byte, error) {
	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by %s; DO NOT EDIT.\n\npackage %s\n\n", generator, pkg)
	fmt.Fprintf(&b, "import \"github.com/maestroi/pokepilot/gen1/canon\"\n\n")
	if len(r.Dropped) > 0 {
		fmt.Fprintf(&b, "// Canonical RAM with no native storage (reads as zero):\n")
		for _, d := range r.Dropped {
			fmt.Fprintf(&b, "//   %s\n", d)
		}
	}
	if len(r.Truncated) > 0 {
		fmt.Fprintf(&b, "//\n// Canonical RAM with shorter native storage:\n")
		for _, d := range r.Truncated {
			fmt.Fprintf(&b, "//   %s\n", d)
		}
	}
	fmt.Fprintf(&b, "var %s = canon.View{\n\tSpans: []canon.Span{\n", name)
	for _, s := range r.View.Spans {
		fmt.Fprintf(&b, "\t\t{Canon: %#04x, Native: %#04x, Len: %d},\n", s.Canon, s.Native, s.Len)
	}
	fmt.Fprintf(&b, "\t},\n\tFlags: []canon.FlagArray{\n")
	for _, f := range r.View.Flags {
		fmt.Fprintf(&b, "\t\t{Canon: %#04x, Native: %#04x, CanonToNative: %s},\n", f.Canon, f.Native, int16s(f.CanonToNative))
	}
	fmt.Fprintf(&b, "\t},\n\tLists: []canon.IndexList{\n")
	for _, l := range r.View.Lists {
		fmt.Fprintf(&b, "\t\t{Canon: %#04x, Native: %#04x, Len: %d, Stride: %d, ValueOffset: %d, Terminator: %#02x, NativeToCanon: %s},\n",
			l.Canon, l.Native, l.Len, l.Stride, l.ValueOffset, l.Terminator, int16s(l.NativeToCanon))
	}
	fmt.Fprintf(&b, "\t},\n}\n")
	return format.Source(b.Bytes())
}

func int16s(v []int16) string {
	var b strings.Builder
	b.WriteString("[]int16{")
	for i, x := range v {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.Itoa(int(x)))
	}
	b.WriteString("}")
	return b.String()
}
