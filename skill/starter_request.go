package skill

import redstarter "github.com/maestroi/pokepilot/red/starter"

// StarterFromRequest maps the public starter request syntax to the physical
// Oak Lab ball the deterministic skill should take. Canonical starters retain
// their original slots; random/fixed experiments always use the middle ball,
// whose species byte is patched by the runner before gameplay starts.
func StarterFromRequest(request string) (Starter, bool) {
	sel, err := redstarter.Resolve(request, 0)
	if err != nil {
		return 0, false
	}
	if sel.Experiment() {
		return StarterSquirtle, true
	}
	switch sel.Slot {
	case "charmander":
		return StarterCharmander, true
	case "squirtle":
		return StarterSquirtle, true
	case "bulbasaur":
		return StarterBulbasaur, true
	default:
		return 0, false
	}
}
