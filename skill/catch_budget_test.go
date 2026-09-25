package skill

import "testing"

func TestCatchHuntFrameBudgetStopsOnlyAtConfiguredBoundary(t *testing.T) {
	if catchHuntFrameBudgetReached(100, 100+catchHuntFrameCap-1) {
		t.Fatal("catch frame budget expired one frame early")
	}
	if !catchHuntFrameBudgetReached(100, 100+catchHuntFrameCap) {
		t.Fatal("catch frame budget did not expire at its configured boundary")
	}
	if catchHuntFrameBudgetReached(200, 100) {
		t.Fatal("wrapped/backwards frame count was treated as an expired hunt")
	}
}
