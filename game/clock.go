package game

import "sort"

// ClockLeg is a route leg whose only effect is the clock. Until is the period
// the planner must observe before the following encounter is open. Adapters
// that have no clock never produce one, so a game without an RTC does not
// grow a wait.
type ClockLeg struct {
	Until TimeOfDay `json:"until"`
}

// TimedSlot is one wild-encounter window. An empty Times list is not
// clock-gated (Gen 1 grass, Gen 2 water). A slot may name more than one
// period when the adapter has already merged them.
type TimedSlot struct {
	Species SpeciesID
	Level   uint8
	Times   []TimeOfDay
}

// SpeciesClock is one clock-gated species on the current map. Ungated species
// are omitted: they need no leg. Open is the positive "can be met now"
// postcondition. Wait is set only when Open is false and the current clock is
// a known period, so the next matching period is a real leg rather than a guess.
type SpeciesClock struct {
	Species  SpeciesID
	Open     bool
	Wait     *ClockLeg
	MinLevel uint8
	MaxLevel uint8
	Slots    int
}

// clockCycle is the order a wait walks. Morning follows night, matching the
// cartridge periods morning 04:00–09:59, day 10:00–17:59, night 18:00–03:59.
var clockCycle = []TimeOfDay{TimeMorning, TimeDay, TimeNight}

// PlanSpeciesClocks reports which species need a clock leg at now.
// now empty means the adapter has no clock: gated species stay closed and no
// wait is invented. Species present in every period, or in any ungated slot,
// are not clock-gated.
func PlanSpeciesClocks(now TimeOfDay, slots []TimedSlot) []SpeciesClock {
	type bucket struct {
		ungated bool
		times   map[TimeOfDay]bool
		min     uint8
		max     uint8
		slots   int
		init    bool
	}
	order := make([]SpeciesID, 0)
	buckets := map[SpeciesID]*bucket{}
	for _, slot := range slots {
		if slot.Species == "" {
			continue
		}
		b := buckets[slot.Species]
		if b == nil {
			b = &bucket{times: map[TimeOfDay]bool{}}
			buckets[slot.Species] = b
			order = append(order, slot.Species)
		}
		b.slots++
		if !b.init || slot.Level < b.min {
			b.min = slot.Level
		}
		if !b.init || slot.Level > b.max {
			b.max = slot.Level
		}
		b.init = true
		if len(slot.Times) == 0 {
			b.ungated = true
			continue
		}
		for _, period := range slot.Times {
			if knownClockPeriod(period) {
				b.times[period] = true
			}
		}
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	var out []SpeciesClock
	for _, id := range order {
		b := buckets[id]
		if b.ungated || len(b.times) == 0 || len(b.times) == len(clockCycle) {
			continue
		}
		sc := SpeciesClock{
			Species:  id,
			MinLevel: b.min,
			MaxLevel: b.max,
			Slots:    b.slots,
		}
		if b.times[now] {
			sc.Open = true
		} else if until, ok := nextClockPeriod(now, b.times); ok {
			sc.Wait = &ClockLeg{Until: until}
		}
		out = append(out, sc)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func knownClockPeriod(period TimeOfDay) bool {
	switch period {
	case TimeMorning, TimeDay, TimeNight:
		return true
	default:
		return false
	}
}

// nextClockPeriod is the next period in clockCycle, after now, that the
// species can be met in. now must itself be a known period.
func nextClockPeriod(now TimeOfDay, open map[TimeOfDay]bool) (TimeOfDay, bool) {
	start := -1
	for i, period := range clockCycle {
		if period == now {
			start = i
			break
		}
	}
	if start < 0 {
		return "", false
	}
	for step := 1; step <= len(clockCycle); step++ {
		period := clockCycle[(start+step)%len(clockCycle)]
		if open[period] {
			return period, true
		}
	}
	return "", false
}
