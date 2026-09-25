package sym

import (
	"bytes"
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/pokepilot/gen1/canon/canongen"
)

func TestCanonicalViewIsGenerated(t *testing.T) {
	res, err := canongen.Generate(
		canongen.Tree{Dir: "../../pokered", Sym: "pokered.sym"},
		canongen.Tree{Dir: "../../pokeyellow", Sym: "pokeyellow.sym"},
	)
	if err != nil {
		t.Fatal(err)
	}
	want, err := canongen.Source("sym", "Canonical", "cmd/gen1canongen", res)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("canonical_generated.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("canonical_generated.go is stale; run: go run ./cmd/gen1canongen")
	}
}

// Every Gen-I address the shared adapter code reads through red/sym must
// resolve to the same-named Yellow symbol. A canonical address that silently
// read a neighbouring Yellow variable would be a wrong fact, not a crash.
func TestCanonicalViewResolvesEveryRedSymConstant(t *testing.T) {
	red := redSymConstants(t)
	redSyms := decompRAMSymbols(t, "../../pokered/pokered.sym")
	yellowSyms := decompRAMSymbols(t, "../../pokeyellow/pokeyellow.sym")
	byAddr := map[uint16][]string{}
	for name, a := range redSyms {
		byAddr[a] = append(byAddr[a], name)
	}
	for name, addr := range red {
		if addr < 0xC000 || addr == 0xFFFF {
			continue // ROM, VRAM, SRAM and IE pass through
		}
		nat, ok := Canonical.Native(addr)
		if !ok {
			t.Errorf("red/sym.%s (%#04x) has no Yellow storage", name, addr)
			continue
		}
		for _, decomp := range byAddr[addr] {
			if y, ok := yellowSyms[decomp]; ok && y != nat {
				t.Errorf("red/sym.%s (%s %#04x) -> %#04x, Yellow %s is %#04x", name, decomp, addr, nat, decomp, y)
			}
		}
	}
	// Yellow's own hand-maintained symbols agree with the generated view.
	for canonName, yellowAddr := range map[string]uint16{
		"CurMap": CurMap, "YCoord": YCoord, "XCoord": XCoord, "PartyCount": PartyCount,
		"PartyMon1": PartyMon1, "NumBagItems": NumBagItems, "PlayerMoney": PlayerMoney,
		"ObtainedBadges": ObtainedBadges, "EventFlags": EventFlags, "IsInBattle": IsInBattle,
		"WalkCounter": WalkCounter, "CurrentMenuItem": CurrentMenuItem, "TileMap": TileMap,
	} {
		redAddr, ok := red[canonName]
		if !ok {
			t.Fatalf("red/sym has no %s", canonName)
		}
		if got, _ := Canonical.Native(redAddr); got != yellowAddr {
			t.Errorf("%s: canonical %#04x -> %#04x, yellow/sym says %#04x", canonName, redAddr, got, yellowAddr)
		}
	}
}

func redSymConstants(t *testing.T) map[string]uint16 {
	t.Helper()
	fset := token.NewFileSet()
	paths, _ := filepath.Glob("../../red/sym/*.go")
	var files []*ast.File
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	pkg, err := (&types.Config{Importer: importer.Default()}).Check("red/sym", fset, files, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]uint16{}
	for _, name := range pkg.Scope().Names() {
		c, ok := pkg.Scope().Lookup(name).(*types.Const)
		if !ok {
			continue
		}
		if b, ok := c.Type().Underlying().(*types.Basic); !ok || b.Kind() != types.Uint16 {
			continue
		}
		v, _ := constant.Uint64Val(c.Val())
		out[name] = uint16(v)
	}
	if len(out) < 100 {
		t.Fatalf("parsed only %d red/sym address constants", len(out))
	}
	return out
}

func decompRAMSymbols(t *testing.T, path string) map[string]uint16 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]uint16{}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 || len(f[0]) != 7 || f[0][2] != ':' || strings.Contains(f[1], ".") {
			continue
		}
		var a uint16
		for _, c := range f[0][3:] {
			a = a<<4 | uint16(strings.IndexRune("0123456789abcdef", c|0x20))
		}
		if a >= 0xC000 {
			out[f[1]] = a
		}
	}
	return out
}
