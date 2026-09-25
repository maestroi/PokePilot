package skill

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestDirectBattleCallersDeclareStructuredOwnership is an architecture guard,
// not a complete gameplay test. A story/progression function that drives Battle
// directly must convert the resolved outcome through RequireBattleWin or
// RequireTrainerBattleWin. Functions that intentionally own a different
// semantic result (travel, catching, training, etc.) are listed here with the
// reason that their raw BattleResult belongs to that higher-level result.
//
// Keep this allow-list narrow. Adding an entry is an architectural decision:
// new mandatory/story battles should almost always use the structured required
// battle contract instead.
func TestDirectBattleCallersDeclareStructuredOwnership(t *testing.T) {
	allowedRawBattleOwners := map[string]string{
		"Battle":                    "Compatibility facade delegates to BattleWithOptions with zero-value options; semantic ownership remains with its caller.",
		"fightOnly":                 "Travel resolves incidental encounters into battleResolution.",
		"fleeThenFight":             "Travel may be forced to fight an incidental trainer after RUN is refused.",
		"GetStarter":                "The Oak-lab rival fight is complete on its positive story event even after a loss.",
		"towerBattleResolver":       "Pokemon Tower travel owns incidental encounter resolution, not story victory.",
		"recoverForcedChoiceBattle": "Boundary recovery finishes an already-owned battle and reports generic blackout.",
		"Gym":                       "Gym is a battle primitive that returns BattleResult; its objective/story caller owns the required-win contract.",
		"Catch":                     "Catch owns catch-session outcomes, including non-target battle losses.",
		"catchWanted":               "Catch settles an uncaught target battle before returning a catch-session outcome.",
		"resolveTrainingBattle":     "Train owns incidental wild/trainer battle results as training-session progress, retreat, or blackout outcomes.",
		"CatchWater":                "Water catching owns catch-session outcomes for incidental encounters.",
		"Fish":                      "Fishing owns catch-session outcomes for incidental encounters.",
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read skill package: %v", err)
	}

	fset := token.NewFileSet()
	seenAllowed := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			hasBattle, hasRequiredContract := false, false
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				ident, ok := call.Fun.(*ast.Ident)
				if !ok {
					return true
				}
				switch ident.Name {
				case "Battle", "BattleWithOptions":
					hasBattle = true
				case "RequireBattleWin", "RequireTrainerBattleWin":
					hasRequiredContract = true
				}
				return true
			})
			if !hasBattle {
				continue
			}
			if hasRequiredContract {
				continue
			}
			if reason, ok := allowedRawBattleOwners[fn.Name.Name]; ok {
				if strings.TrimSpace(reason) == "" {
					t.Fatalf("%s:%s raw Battle owner has empty architectural rationale", name, fn.Name.Name)
				}
				seenAllowed[fn.Name.Name] = true
				continue
			}
			t.Errorf("%s:%s drives Battle directly without RequireBattleWin/RequireTrainerBattleWin; mandatory/story battles must emit structured required-battle outcomes, otherwise add a narrowly justified raw-result owner", name, fn.Name.Name)
		}
	}

	for fn, reason := range allowedRawBattleOwners {
		if !seenAllowed[fn] {
			t.Errorf("raw Battle owner allow-list entry %q is stale (%s); remove it rather than leaving a broad future escape hatch", fn, reason)
		}
	}
}
