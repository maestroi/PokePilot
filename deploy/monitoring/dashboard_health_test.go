package monitoring

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Metrics the wall exports today. Dashboards in this file must not query
// series that later issues have not landed.
var exportedSeries = map[string]bool{
	"pokepilot_runs_active":                           true,
	"pokepilot_runs_started_total":                    true,
	"pokepilot_runs_completed_total":                  true,
	"pokepilot_runs_failed_total":                     true,
	"pokepilot_runs_aborted_total":                    true,
	"pokepilot_run_duration_seconds_bucket":           true,
	"pokepilot_run_rounds_bucket":                     true,
	"pokepilot_run_frames_bucket":                     true,
	"pokepilot_last_successful_run_timestamp_seconds": true,
	"pokepilot_run_milestones_total":                  true,
	"pokepilot_run_stopping_milestone_total":          true,
	"pokepilot_run_failures_total":                    true,
	"pokepilot_up":                                    true,
	"pokepilot_build_info":                            true,
}

var noGameLabel = map[string]bool{
	"pokepilot_up":         true,
	"pokepilot_build_info": true,
}

func TestOverviewAndRunsDashboards(t *testing.T) {
	cases := []struct {
		file   string
		uid    string
		min    int
		lokiOn string
	}{
		{"grafana-pokepilot-overview.json", "pokepilot-overview", 12, "Objective failures by type"},
		{"grafana-pokepilot-runs.json", "pokepilot-runs", 6, ""},
	}
	for _, tc := range cases {
		t.Run(tc.uid, func(t *testing.T) {
			assertDashboard(t, tc.file, tc.uid, tc.min, tc.lokiOn)
		})
	}
}

func assertDashboard(t *testing.T, file, uid string, minPanels int, lokiTitle string) {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var d failureDashboard
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	if d.UID != uid || len(d.Panels) < minPanels {
		t.Fatalf("unexpected dashboard %q with %d panels", d.UID, len(d.Panels))
	}
	variables := map[string]bool{}
	for _, v := range d.Templating.List {
		variables[v.Name] = true
	}
	for _, name := range []string{"DS_PROMETHEUS", "DS_LOKI", "game"} {
		if !variables[name] {
			t.Errorf("missing template variable %s", name)
		}
	}
	metricRE := regexp.MustCompile(`pokepilot_[a-z0-9_]+`)
	grid := map[string]string{}
	ids := map[int]bool{}
	sawLoki := lokiTitle == ""
	for _, p := range d.Panels {
		if ids[p.ID] {
			t.Errorf("duplicate panel id %d", p.ID)
		}
		ids[p.ID] = true
		if p.GridPos.W <= 0 || p.GridPos.X < 0 || p.GridPos.Y < 0 || p.GridPos.X+p.GridPos.W > 24 {
			t.Errorf("invalid grid position in %s", p.Title)
		}
		for x := p.GridPos.X; x < p.GridPos.X+p.GridPos.W; x++ {
			for y := p.GridPos.Y; y < p.GridPos.Y+p.GridPos.H; y++ {
				key := fmt.Sprintf("%d/%d", x, y)
				if other, ok := grid[key]; ok {
					t.Errorf("panels overlap at %s: %s and %s", key, other, p.Title)
				}
				grid[key] = p.Title
			}
		}
		if len(p.Targets) == 0 {
			t.Errorf("panel %s has no query", p.Title)
		}
		for _, target := range p.Targets {
			if target.Datasource.UID != "${DS_PROMETHEUS}" {
				t.Errorf("%s hardcodes datasource %s", p.Title, target.Datasource.UID)
			}
			metrics := metricRE.FindAllString(target.Expr, -1)
			if len(metrics) == 0 {
				t.Errorf("%s has no pokepilot series", p.Title)
			}
			needsGame := false
			for _, metric := range metrics {
				if !exportedSeries[metric] {
					t.Errorf("%s queries series that is not exported yet: %s", p.Title, metric)
				}
				if !noGameLabel[metric] {
					needsGame = true
				}
			}
			if needsGame && !strings.Contains(target.Expr, "${game:regex}") {
				t.Errorf("%s lacks game filter", p.Title)
			}
			for _, secret := range []string{"run_id", "failure_id", "family_id", "trace_id"} {
				if strings.Contains(target.Expr, secret) {
					t.Errorf("%s uses identifier %s as a Prometheus label", p.Title, secret)
				}
			}
		}
		for _, link := range p.FieldConfig.Defaults.Links {
			parsed, err := url.Parse(link.URL)
			if err != nil {
				t.Errorf("%s link URL: %v", p.Title, err)
				continue
			}
			if parsed.Path != "/explore" || parsed.Query().Get("schemaVersion") != "1" {
				t.Errorf("%s has unsupported Explore link: %s", p.Title, link.URL)
			}
			var panes map[string]struct {
				Datasource string `json:"datasource"`
				Queries    []struct {
					Expr string `json:"expr"`
				} `json:"queries"`
				Range struct {
					From string `json:"from"`
					To   string `json:"to"`
				} `json:"range"`
			}
			if err := json.Unmarshal([]byte(parsed.Query().Get("panes")), &panes); err != nil {
				t.Errorf("%s has malformed Explore panes: %v", p.Title, err)
				continue
			}
			pane, ok := panes["F"]
			if !ok || pane.Datasource != "${DS_LOKI}" || pane.Range.From != "${__from}" || pane.Range.To != "${__to}" {
				t.Errorf("%s link does not preserve Loki datasource and time range", p.Title)
				continue
			}
			if len(pane.Queries) != 1 || !strings.Contains(pane.Queries[0].Expr, "objective_failure") {
				t.Errorf("%s link missing failure log query", p.Title)
				continue
			}
			if p.Title == lokiTitle &&
				strings.Contains(pane.Queries[0].Expr, "${__field.labels.game}") &&
				strings.Contains(pane.Queries[0].Expr, "${__field.labels.failure_type}") {
				sawLoki = true
			}
		}
	}
	if !sawLoki {
		t.Fatalf("missing Loki drill-down on %q", lokiTitle)
	}
}
