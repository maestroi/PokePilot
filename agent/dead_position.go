package agent

// deadPositionAfter is how many consecutive follow-up rounds may leave the
// player on the exact same tile with no newly completed objective before
// Run stops with StopStuck. The short stuck detector only counts successful
// no-progress rounds; failed "no path" retries at a boxed-in tile never
// increment it, and a strategist will keep replanning them until the
// much larger stagnation watchdog (default 200) fires. Measured:
// run-g9ojxmtgvrff1ezck9g7t1o7x spent 15 rounds on ROUTE_12 (9,62) with
// no completed objective and no reachable battle.
const deadPositionAfter = 3

// deadPosition tracks same-tile, zero-completion streaks across Run rounds.
type deadPosition struct {
	Map       uint8
	X, Y      uint8
	completed int
	streak    int
	set       bool
}

func (d *deadPosition) observe(obs Observation, completed int) int {
	if d.set && d.Map == obs.Map && d.X == obs.X && d.Y == obs.Y && d.completed == completed {
		d.streak++
	} else {
		d.streak = 0
	}
	d.Map, d.X, d.Y, d.completed, d.set = obs.Map, obs.X, obs.Y, completed, true
	return d.streak
}
