package skill

// catchBlackoutOutcome preserves Catch's specific diagnostic while also
// unwrapping to ErrBlackedOut. agent.Run already treats ErrBlackedOut as a
// recoverable game outcome: the party respawned healed at a Pokemon Center,
// so losing a non-wanted wild battle during a hunt must not consume the
// engineering-failure budget or kill the whole farm run.
//
// ErrCatchBlackout predates ErrBlackedOut and is declared in catch.go as a
// package sentinel. Rebinding it during package initialization keeps existing
// errors.Is(err, ErrCatchBlackout) callers working while adding the broader
// errors.Is(err, ErrBlackedOut) classification without changing its text.
type catchBlackoutOutcome struct{}

func (catchBlackoutOutcome) Error() string {
	return "skill: Catch: blacked out while fighting a non-wanted battle"
}

func (catchBlackoutOutcome) Unwrap() error { return ErrBlackedOut }

func init() {
	ErrCatchBlackout = catchBlackoutOutcome{}
}
