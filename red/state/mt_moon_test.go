package state

import "testing"

func TestMtMoonEventIndices(t *testing.T) {
	events := parseEventConstants(t)
	for name, event := range map[string]Event{"EVENT_BEAT_MT_MOON_EXIT_SUPER_NERD": EventBeatMtMoonSuperNerd, "EVENT_GOT_DOME_FOSSIL": EventGotDomeFossil, "EVENT_GOT_HELIX_FOSSIL": EventGotHelixFossil} {
		if events[name] != uint16(event) {
			t.Errorf("%s=%d want %d", name, event, events[name])
		}
	}
}
