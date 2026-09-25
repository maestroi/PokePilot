// Command gen1canongen writes a Gen-I revision's canonical memory view from
// the vendored decompilations. Run from the repository root:
//
//	go run ./cmd/gen1canongen -native pokeyellow -out yellow/sym/canonical_generated.go
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/maestroi/pokepilot/gen1/canon/canongen"
)

func main() {
	canonDir := flag.String("canon", "pokered", "canonical engine decomp directory")
	nativeDir := flag.String("native", "pokeyellow", "native revision decomp directory")
	pkg := flag.String("pkg", "sym", "package of the generated file")
	name := flag.String("var", "Canonical", "variable name of the generated view")
	out := flag.String("out", "yellow/sym/canonical_generated.go", "output file")
	flag.Parse()

	res, err := canongen.Generate(
		canongen.Tree{Dir: *canonDir, Sym: *canonDir + ".sym"},
		canongen.Tree{Dir: *nativeDir, Sym: *nativeDir + ".sym"},
	)
	if err == nil {
		var src []byte
		if src, err = canongen.Source(*pkg, *name, "cmd/gen1canongen", res); err == nil {
			err = os.WriteFile(*out, src, 0o644)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen1canongen:", err)
		os.Exit(1)
	}
}
