package state

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

const eventConstantsFile = "testdata/event_constants.asm"

// parseEventConstants replays the RGBDS const counter over
// constants/event_constants.asm and returns each EVENT_ name's bit index.
//
// The counter starts at const_def, advances by one per const, jumps forward
// by const_skip (bare const_skip means 1), and is reset outright by
// const_next. Hand-counting this is what shifted every index past a missed
// const_skip by two — see the note on Event in progress.go.
func parseEventConstants(t *testing.T) map[string]uint16 {
	t.Helper()
	data, err := os.ReadFile(eventConstantsFile)
	if err != nil {
		t.Fatal("reading vendored event constants: ", err)
	}

	events := make(map[string]uint16)
	var counter uint16
	for n, line := range strings.Split(string(data), "\n") {
		if i := strings.IndexByte(line, ';'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "const_def":
			counter = 0
			if len(fields) > 1 {
				counter = evalConstExpr(t, n+1, fields[1:])
			}
		case "const":
			if len(fields) < 2 {
				t.Fatalf("%s:%d: const with no name", eventConstantsFile, n+1)
			}
			events[fields[1]] = counter
			counter++
		case "const_skip":
			step := uint16(1)
			if len(fields) > 1 {
				step = evalConstExpr(t, n+1, fields[1:])
			}
			counter += step
		case "const_next":
			counter = evalConstExpr(t, n+1, fields[1:])
		}
	}
	return events
}

// evalConstExpr evaluates the tiny arithmetic the file actually uses: a
// hex or decimal term, optionally followed by + or - terms ("$F0 - 2").
func evalConstExpr(t *testing.T, line int, fields []string) uint16 {
	t.Helper()
	toks := strings.Fields(strings.Join(fields, " "))
	if len(toks) == 0 || len(toks)%2 != 1 {
		t.Fatalf("%s:%d: cannot evaluate %q", eventConstantsFile, line, strings.Join(fields, " "))
	}
	term := func(s string) uint16 {
		base, digits := 10, s
		if strings.HasPrefix(s, "$") {
			base, digits = 16, s[1:]
		}
		v, err := strconv.ParseUint(digits, base, 16)
		if err != nil {
			t.Fatalf("%s:%d: bad term %q: %v", eventConstantsFile, line, s, err)
		}
		return uint16(v)
	}
	total := term(toks[0])
	for i := 1; i < len(toks); i += 2 {
		switch toks[i] {
		case "+":
			total += term(toks[i+1])
		case "-":
			total -= term(toks[i+1])
		default:
			t.Fatalf("%s:%d: unsupported operator %q", eventConstantsFile, line, toks[i])
		}
	}
	return total
}

// TestEventIndicesMatchDecomp guards the hand-counted Event constants. They
// cannot come from pokered.sym: RGBDS emits only labels, and these are
// assembler constants with no address (grep -c EVENT_ pokered.sym == 0).
func TestEventIndicesMatchDecomp(t *testing.T) {
	events := parseEventConstants(t)

	pairs := []struct {
		label string
		event Event
	}{
		{"EVENT_FOLLOWED_OAK_INTO_LAB", EventFollowedOakIntoLab},
		{"EVENT_OAK_ASKED_TO_CHOOSE_MON", EventOakAskedToChooseMon},
		{"EVENT_GOT_STARTER", EventGotStarter},
		{"EVENT_BATTLED_RIVAL_IN_OAKS_LAB", EventBattledRivalInOaksLab},
		{"EVENT_GOT_POKEBALLS_FROM_OAK", EventGotPokeballsFromOak},
		{"EVENT_GOT_POKEDEX", EventGotPokedex},
		{"EVENT_OAK_APPEARED_IN_PALLET", EventOakAppearedInPallet},
		{"EVENT_GOT_OAKS_PARCEL", EventGotOaksParcel},
	}
	for _, p := range pairs {
		want, ok := events[p.label]
		if !ok {
			t.Errorf("label %s not found in %s", p.label, eventConstantsFile)
			continue
		}
		if uint16(p.event) != want {
			t.Errorf("%s = %d, want %d (from %s)", p.label, uint16(p.event), want, eventConstantsFile)
		}
	}
}
