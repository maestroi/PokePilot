package main

import (
	"testing"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

func bad(name string, grace int) operatorapi.CheckResult {
	return operatorapi.CheckResult{Name: name, Grace: grace, Message: name + " broke"}
}
func ok(name string) operatorapi.CheckResult { return operatorapi.CheckResult{Name: name, OK: true} }

func kinds(actions []alertAction) []alertKind {
	out := []alertKind{}
	for _, a := range actions {
		out = append(out, a.Kind)
	}
	return out
}

func TestAlertGraceRemindResolve(t *testing.T) {
	a := newAlertBook(12 * time.Hour)
	t0 := time.Unix(1_800_000_000, 0)
	a.Observe("watch", []operatorapi.CheckResult{ok("disk")}, t0) // seed
	if acts, _ := a.Observe("watch", []operatorapi.CheckResult{bad("disk", 2)}, t0.Add(time.Minute)); len(acts) != 0 {
		t.Fatalf("inside grace: %v", kinds(acts))
	}
	acts, _ := a.Observe("watch", []operatorapi.CheckResult{bad("disk", 2)}, t0.Add(2*time.Minute))
	if len(acts) != 1 || acts[0].Kind != alertOpen {
		t.Fatalf("open: %v", kinds(acts))
	}
	if acts, _ = a.Observe("watch", []operatorapi.CheckResult{bad("disk", 2)}, t0.Add(3*time.Hour)); len(acts) != 0 {
		t.Fatalf("no reminder before 12h: %v", kinds(acts))
	}
	if acts, _ = a.Observe("watch", []operatorapi.CheckResult{bad("disk", 2)}, t0.Add(13*time.Hour)); len(acts) != 1 || acts[0].Kind != alertRemind {
		t.Fatalf("remind: %v", kinds(acts))
	}
	acts, _ = a.Observe("watch", []operatorapi.CheckResult{ok("disk")}, t0.Add(14*time.Hour))
	if len(acts) != 1 || acts[0].Kind != alertResolve || acts[0].State.Since != t0.Add(time.Minute) {
		t.Fatalf("resolve: %+v", acts)
	}
}

func TestAlertMissingResultFromSameSourceResolves(t *testing.T) {
	a := newAlertBook(12 * time.Hour)
	now := time.Now()
	a.Observe("bot", nil, now)
	a.Observe("bot", []operatorapi.CheckResult{bad("stall:r1", 1)}, now)
	acts, _ := a.Observe("bot", nil, now.Add(time.Minute))
	if len(acts) != 1 || acts[0].Kind != alertResolve {
		t.Fatalf("run gone → resolve: %v", kinds(acts))
	}
	// another source's list never resolves this source's checks
	a.Observe("bot", []operatorapi.CheckResult{bad("wall", 1)}, now)
	if acts, _ := a.Observe("watch", nil, now); len(acts) != 0 {
		t.Fatalf("cross-source resolve: %v", kinds(acts))
	}
}

func TestAlertRestartAdoptsWithoutPaging(t *testing.T) {
	a := newAlertBook(12 * time.Hour)
	acts, adopted := a.Observe("watch", []operatorapi.CheckResult{bad("quorum", 2), ok("disk")}, time.Now())
	if len(acts) != 0 || len(adopted) != 1 || !adopted[0].Open {
		t.Fatalf("restart: acts=%v adopted=%v", kinds(acts), adopted)
	}
	acts, _ = a.Observe("watch", []operatorapi.CheckResult{ok("quorum")}, time.Now())
	if len(acts) != 1 || acts[0].Kind != alertResolve {
		t.Fatalf("adopted alert must still resolve: %v", kinds(acts))
	}
}

func TestAlertMuteSuppressesOpenAndRemind(t *testing.T) {
	a := newAlertBook(12 * time.Hour)
	t0 := time.Now()
	a.Observe("watch", nil, t0)
	a.Observe("watch", []operatorapi.CheckResult{bad("paid-cap", 1)}, t0)
	if !a.Mute("paid-cap", t0.Add(12*time.Hour)) {
		t.Fatal("mute open alert")
	}
	if acts, _ := a.Observe("watch", []operatorapi.CheckResult{bad("paid-cap", 1)}, t0.Add(11*time.Hour+59*time.Minute)); len(acts) != 0 {
		t.Fatalf("muted: %v", kinds(acts))
	}
	if acts, _ := a.Observe("watch", []operatorapi.CheckResult{bad("paid-cap", 1)}, t0.Add(13*time.Hour)); len(acts) != 1 || acts[0].Kind != alertRemind {
		t.Fatalf("mute expired → remind: %v", kinds(acts))
	}
}
