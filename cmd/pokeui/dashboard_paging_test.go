package main

import (
	"strings"
	"testing"
)

func TestDashboardPagingLoadsBeforeMainConsole(t *testing.T) {
	html := string(operatorIndexPage())
	paging := strings.Index(html, `<script src="/dashboard_paging.js"></script>`)
	mainScript := strings.Index(html, `<script src="/ui.js"></script>`)
	if paging < 0 {
		t.Fatal("operator page missing dashboard paging script")
	}
	if mainScript < 0 {
		t.Fatal("operator page missing ui.js")
	}
	if paging >= mainScript {
		t.Fatalf("dashboard paging must load before ui.js: paging=%d ui=%d", paging, mainScript)
	}
	if strings.Count(html, `/dashboard_paging.js`) != 1 {
		t.Fatalf("dashboard paging script count=%d, want 1", strings.Count(html, `/dashboard_paging.js`))
	}
}

func TestDashboardPagingUsesActivePollAndServerHistoryPages(t *testing.T) {
	js := string(dashboardPagingJS)
	for _, want := range []string{
		`/v1/dashboard?active=1`,
		`q.set("status","done")`,
		`q.set("limit",String(PAGE))`,
		`q.set("offset",String(Math.max(0,page)*PAGE))`,
		`q.set("facets","1")`,
		`/v1/runs/`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("dashboard paging missing %q", want)
		}
	}
}

func TestBulkCleanupFullHistoryIsExplicit(t *testing.T) {
	if !strings.Contains(string(runCleanupJS), `/v1/dashboard?status=done`) {
		t.Fatal("bulk cleanup should explicitly fetch finished history rather than use the live dashboard poll")
	}
}
