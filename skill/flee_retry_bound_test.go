package skill

import "testing"

func TestGuaranteedWildFleeAttemptsMatchesROMBound(t *testing.T) {
	const (
		worstCaseQuotient = 0
		retryBonus        = 30
		maxByte           = 0xff
	)

	if got := worstCaseQuotient + retryBonus*(guaranteedWildFleeAttempts-1); got <= maxByte {
		t.Fatalf("%d flee attempts are not enough to guarantee overflow from quotient 0: got %d, want > %d", guaranteedWildFleeAttempts, got, maxByte)
	}
	if got := worstCaseQuotient + retryBonus*(guaranteedWildFleeAttempts-2); got > maxByte {
		t.Fatalf("%d flee attempts are not the minimal guaranteed bound: previous attempt already reaches %d", guaranteedWildFleeAttempts, got)
	}
}
