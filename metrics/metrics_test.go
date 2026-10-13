package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExpositionIncludesCounterGaugeHistogramAndBuildInfo(t *testing.T) {
	reg, err := New(Options{Service: ServiceWall, Revision: "abc123"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	runs, err := reg.NewCounterVec("pokepilot_runs_completed_total", "Completed runs.", []string{"game", "result"})
	if err != nil {
		t.Fatalf("counter: %v", err)
	}
	runs.WithLabelValues("pokemon-red", "completed").Inc()

	active, err := reg.NewGaugeVec("pokepilot_runs_active", "Active runs.", []string{"game"})
	if err != nil {
		t.Fatalf("gauge: %v", err)
	}
	active.WithLabelValues("pokemon-red").Set(2)

	dur, err := reg.NewHistogramVec("pokepilot_run_duration_seconds", "Completed run duration.", []string{"game"}, DurationBuckets)
	if err != nil {
		t.Fatalf("histogram: %v", err)
	}
	dur.WithLabelValues("pokemon-red").Observe(1.5)

	body := scrape(t, reg.Handler())
	for _, want := range []string{
		`pokepilot_runs_completed_total{game="pokemon-red",result="completed"}`,
		`pokepilot_runs_active{game="pokemon-red"}`,
		"pokepilot_run_duration_seconds_bucket",
		"pokepilot_run_duration_seconds_sum",
		"pokepilot_run_duration_seconds_count",
		`pokepilot_build_info{`,
		`revision="abc123"`,
		`service="wall"`,
		`pokepilot_up{service="wall"}`,
		"go_goroutines",
		"process_start_time_seconds",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("exposition missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, "run_id=") {
		t.Fatalf("exposition leaked a run_id label:\n%s", body)
	}
}

func TestRejectsHighCardinalityLabelNames(t *testing.T) {
	reg := Must(Options{Service: ServiceRunner, Revision: "dev"})
	for _, label := range []string{
		"run_id", "failure_id", "family_id", "trace_id", "goal_id",
		"url", "exception", "prompt", "widget_id",
	} {
		_, err := reg.NewCounterVec("pokepilot_example_total", "example", []string{"game", label})
		if err == nil {
			t.Errorf("label %q was accepted", label)
		}
	}
	if _, err := reg.NewCounterVec("pokepilot_example_total", "example", []string{"game", "result"}); err != nil {
		t.Fatal(err)
	}
}

func TestRejectsMetricNamesThatBreakConventions(t *testing.T) {
	reg := Must(Options{Service: ServiceIssues, Revision: "dev"})
	if _, err := reg.NewCounterVec("runs_total", "missing prefix", nil); err == nil {
		t.Fatal("accepted a counter without the pokepilot_ prefix")
	}
	if _, err := reg.NewCounterVec("pokepilot_runs", "missing _total", nil); err == nil {
		t.Fatal("accepted a counter without the _total suffix")
	}
	if _, err := reg.NewHistogramVec("pokepilot_run_duration", "missing unit", nil, nil); err == nil {
		t.Fatal("accepted a histogram without a unit suffix")
	}
}

func TestWrapServesMetricsAndDelegatesOtherRoutes(t *testing.T) {
	reg := Must(Options{Service: ServiceUI, Revision: "dev"})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(reg.Wrap(next))
	defer srv.Close()

	metricsResp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer metricsResp.Body.Close()
	body, _ := io.ReadAll(metricsResp.Body)
	if metricsResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics = %d", metricsResp.StatusCode)
	}
	if !strings.Contains(string(body), `pokepilot_up{service="ui"}`) {
		t.Fatalf("GET /metrics body missing up gauge:\n%s", body)
	}

	other, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Body.Close()
	if other.StatusCode != http.StatusNoContent {
		t.Fatalf("GET /healthz = %d, want 204", other.StatusCode)
	}
}

func TestRejectsUnknownServiceAndSanitizesRevision(t *testing.T) {
	if _, err := New(Options{Service: "wherever", Revision: "abc"}); err == nil {
		t.Fatal("accepted an unknown service label")
	}
	reg, err := New(Options{Service: ServiceReplay, Revision: "dead beef"})
	if err != nil {
		t.Fatal(err)
	}
	body := scrape(t, reg.Handler())
	if !strings.Contains(body, `revision="dev"`) {
		t.Fatalf("unsafe revision was kept:\n%s", body)
	}
	if strings.Contains(body, "dead beef") {
		t.Fatalf("unsafe revision leaked:\n%s", body)
	}
}

func scrape(t *testing.T, h http.Handler) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d\n%s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}
