package main

import (
	"sort"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

type alertKind int

const (
	alertOpen alertKind = iota + 1
	alertRemind
	alertResolve
)

type alertState struct {
	Name, Source, Message, RunID, Link string
	Bad                                int
	Open                               bool
	Since                              time.Time // first bad observation of the open episode
	Notified                           time.Time
	MutedUntil                         time.Time
	Cards                              map[int64]int64 // chat → alert card message id
}

type alertAction struct {
	Kind  alertKind
	State *alertState // live pointer; renderer may set Cards
}

type alertBook struct {
	remind time.Duration
	states map[string]*alertState
	seeded map[string]bool
}

func newAlertBook(remind time.Duration) *alertBook {
	return &alertBook{remind: remind, states: map[string]*alertState{}, seeded: map[string]bool{}}
}

// Observe applies one full result list from source. Results previously seen
// from the same source but absent now count as OK. The first call per source
// adopts bad results as open without paging and returns them as `adopted`.
func (a *alertBook) Observe(source string, results []operatorapi.CheckResult, now time.Time) ([]alertAction, []*alertState) {
	var actions []alertAction
	var adopted []*alertState
	first := !a.seeded[source]
	a.seeded[source] = true
	present := map[string]bool{}
	for _, r := range results {
		present[r.Name] = true
		st := a.states[r.Name]
		if st == nil {
			st = &alertState{Name: r.Name, Source: source, Cards: map[int64]int64{}}
			a.states[r.Name] = st
		}
		if r.OK {
			if st.Open {
				st.Open = false
				actions = append(actions, alertAction{Kind: alertResolve, State: st})
			}
			st.Bad = 0
			continue
		}
		st.Bad++
		st.Message, st.RunID, st.Link = r.Message, r.RunID, r.Link
		if st.Bad == 1 {
			st.Since = now
		}
		grace := r.Grace
		if grace < 1 {
			grace = 1
		}
		switch {
		case !st.Open && first:
			st.Open, st.Notified = true, now
			adopted = append(adopted, st)
		case !st.Open && st.Bad >= grace:
			st.Open = true
			if now.After(st.MutedUntil) {
				st.Notified = now
				actions = append(actions, alertAction{Kind: alertOpen, State: st})
			}
		case st.Open && now.After(st.MutedUntil) && now.Sub(st.Notified) >= a.remind:
			st.Notified = now
			actions = append(actions, alertAction{Kind: alertRemind, State: st})
		}
	}
	for name, st := range a.states {
		if st.Source != source || present[name] {
			continue
		}
		if st.Open {
			actions = append(actions, alertAction{Kind: alertResolve, State: st})
		}
		delete(a.states, name)
	}
	return actions, adopted
}

func (a *alertBook) Mute(name string, until time.Time) bool {
	st := a.states[name]
	if st == nil || !st.Open {
		return false
	}
	st.MutedUntil = until
	// The reminder falls due the moment the mute expires.
	st.Notified = until.Add(-a.remind)
	return true
}

func (a *alertBook) Open() []*alertState {
	var out []*alertState
	for _, st := range a.states {
		if st.Open {
			out = append(out, st)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Since.Before(out[j].Since) })
	return out
}
