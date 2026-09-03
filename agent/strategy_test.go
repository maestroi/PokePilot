package agent

import (
	"strings"
	"testing"
)

func TestStrategicMemoryDetectsLongHorizonStallUsingIntent(t *testing.T) {
	var m StrategicMemory
	obs := Observation{Map: 1, Badges: []string{"boulder"}, Events: []string{"x"}, Party: []PartyMon{{Level: 12}}}
	if !m.ObserveProgress(obs) {
		t.Fatal("first observation should establish progress baseline")
	}
	for i := 0; i < 4; i++ {
		if m.ObserveProgress(obs) {
			t.Fatal("identical observation counted as progress")
		}
	}
	reason := m.ReplanReason(4, "train before trying the gym again")
	if !strings.Contains(reason, `Intent "train before trying the gym again"`) || !strings.Contains(reason, "materially different") {
		t.Fatalf("replan reason = %q", reason)
	}
}

func TestStrategicMemoryDetectsPingPongMapLoop(t *testing.T) {
	var m StrategicMemory
	base := Observation{Party: []PartyMon{{Level: 12}}}
	maps := []uint8{1, 2, 1, 2, 1, 2, 1}
	for i, mapID := range maps {
		obs := base
		obs.Map = mapID
		progressed := m.ObserveProgress(obs)
		if i < 2 && !progressed {
			t.Fatalf("observation %d on new map %d did not count as progress", i, mapID)
		}
	}
	if m.NoProgress != 5 {
		t.Fatalf("NoProgress = %d, want 5 after repeated A-B loop", m.NoProgress)
	}
	if reason := m.ReplanReason(4, "explore"); reason == "" {
		t.Fatal("ping-pong map loop did not trigger replan signal")
	}
}

func TestStrategicMemoryCreditsMapAfterItLeavesRecentWindow(t *testing.T) {
	var m StrategicMemory
	base := Observation{Party: []PartyMon{{Level: 12}}}
	for _, mapID := range []uint8{1, 2, 3, 4, 5} {
		obs := base
		obs.Map = mapID
		if !m.ObserveProgress(obs) {
			t.Fatalf("new map %d did not count as progress", mapID)
		}
	}
	obs := base
	obs.Map = 1
	if !m.ObserveProgress(obs) {
		t.Fatal("map outside recent window should count as fresh local progress")
	}
}

func TestStrategicMemoryCanSignalWithoutIntent(t *testing.T) {
	var m StrategicMemory
	obs := Observation{Map: 1, Party: []PartyMon{{Level: 12}}}
	m.ObserveProgress(obs)
	m.ObserveProgress(obs)
	if reason := m.ReplanReason(1, ""); !strings.Contains(reason, "No measurable progress for 1 rounds") {
		t.Fatalf("replan reason = %q", reason)
	}
}

func TestStrategicMemoryResetsStallOnMeasurableProgress(t *testing.T) {
	var m StrategicMemory
	obs := Observation{Map: 1, Party: []PartyMon{{Level: 12}}}
	m.ObserveProgress(obs)
	m.ObserveProgress(obs)
	m.ObserveProgress(obs)
	if m.NoProgress != 2 {
		t.Fatalf("NoProgress = %d, want 2", m.NoProgress)
	}
	obs.Party[0].Level = 13
	if !m.ObserveProgress(obs) || m.NoProgress != 0 {
		t.Fatalf("level gain did not reset stall: %+v", m)
	}
}

func TestStrategicMemoryReplansWhenOnlyLevelsAdvance(t *testing.T) {
	var m StrategicMemory
	obs := Observation{
		Map:    2,
		Badges: []string{"boulder"},
		Party:  []PartyMon{{Level: 16}},
	}
	m.ObserveProgress(obs)
	for level := uint8(17); level <= 20; level++ {
		obs.Party[0].Level = level
		if !m.ObserveProgress(obs) {
			t.Fatalf("level %d should still count as measurable progress", level)
		}
	}
	if m.NoProgress != 0 {
		t.Fatalf("NoProgress = %d, want 0 while levels are rising", m.NoProgress)
	}
	if m.NoWorldProgress != 4 {
		t.Fatalf("NoWorldProgress = %d, want 4", m.NoWorldProgress)
	}
	reason := m.ReplanReason(4, "keep training near pewter")
	if !strings.Contains(reason, "no new badge, event, or map progress") || !strings.Contains(reason, "re-evaluate") {
		t.Fatalf("replan reason = %q", reason)
	}
}

func TestStrategicMemoryWorldProgressResetsLevelOnlyBackstop(t *testing.T) {
	var m StrategicMemory
	obs := Observation{Map: 1, Party: []PartyMon{{Level: 12}}}
	m.ObserveProgress(obs)
	for level := uint8(13); level <= 15; level++ {
		obs.Party[0].Level = level
		m.ObserveProgress(obs)
	}
	if m.NoWorldProgress != 3 {
		t.Fatalf("NoWorldProgress = %d, want 3 before exploration", m.NoWorldProgress)
	}
	obs.Map = 2
	if !m.ObserveProgress(obs) || m.NoWorldProgress != 0 {
		t.Fatalf("new map did not reset world-progress backstop: %+v", m)
	}
}

func TestReplanReasonWaitsForThreshold(t *testing.T) {
	m := StrategicMemory{NoProgress: 3, NoWorldProgress: 3}
	if got := m.ReplanReason(4, "explore north"); got != "" {
		t.Fatalf("early replan reason = %q", got)
	}
}
