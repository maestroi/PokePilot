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

type failureDashboard struct {
	UID    string `json:"uid"`
	Title  string `json:"title"`
	Panels []struct {
		ID      int    `json:"id"`
		Title   string `json:"title"`
		Type    string `json:"type"`
		GridPos struct {
			X int `json:"x"`
			Y int `json:"y"`
			W int `json:"w"`
			H int `json:"h"`
		} `json:"gridPos"`
		Targets []struct {
			Expr       string `json:"expr"`
			Instant    bool   `json:"instant"`
			Datasource struct {
				UID string `json:"uid"`
			} `json:"datasource"`
		} `json:"targets"`
		FieldConfig struct {
			Defaults struct {
				Links []struct {
					URL string `json:"url"`
				} `json:"links"`
			} `json:"defaults"`
		} `json:"fieldConfig"`
	} `json:"panels"`
	Templating struct {
		List []struct {
			Name string `json:"name"`
		} `json:"list"`
	} `json:"templating"`
}

// These series are exported by the wall today. The dashboard must not
// silently depend on imagined subsystem counters.
var supportedFailureSeries = map[string]bool{
	"pokepilot_run_failures_total":                     true,
	"pokepilot_failure_recovery_attempts_total":        true,
	"pokepilot_failure_supervisor_recoveries_total":    true,
	"pokepilot_failure_time_to_failure_seconds_bucket": true,
	"pokepilot_runs_failed_total":                      true,
	"pokepilot_runs_completed_total":                   true,
}

func TestFailureDashboardDatasourceAndQueries(t *testing.T) {
	b, err := os.ReadFile("grafana-pokepilot-failures.json")
	if err != nil {
		t.Fatal(err)
	}
	var d failureDashboard
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	if d.UID != "pokepilot-failures" || len(d.Panels) < 8 {
		t.Fatalf("unexpected dashboard %q with %d panels", d.UID, len(d.Panels))
	}
	variables := map[string]bool{}
	for _, v := range d.Templating.List {
		variables[v.Name] = true
	}
	for _, v := range []string{"DS_PROMETHEUS", "DS_LOKI", "game"} {
		if !variables[v] {
			t.Errorf("missing template variable %s", v)
		}
	}
	metricRE := regexp.MustCompile(`pokepilot_[a-z0-9_]+`)
	grid := map[string]string{}
	ids := map[int]bool{}
	links := 0
	causeLink := false
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
			if !strings.Contains(target.Expr, "${game:regex}") {
				t.Errorf("%s lacks game filter", p.Title)
			}
			for _, metric := range metricRE.FindAllString(target.Expr, -1) {
				if !supportedFailureSeries[metric] {
					t.Errorf("%s queries nonexistent metric %s", p.Title, metric)
				}
			}
			for _, secret := range []string{"run_id", "failure_id", "family_id", "trace_id"} {
				if strings.Contains(target.Expr, secret) {
					t.Errorf("%s uses identifier %s as a Prometheus label", p.Title, secret)
				}
			}
		}
		for _, link := range p.FieldConfig.Defaults.Links {
			links++
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
			}
			if len(pane.Queries) != 1 || !strings.Contains(pane.Queries[0].Expr, "objective_failure") {
				t.Errorf("%s link missing failure log query", p.Title)
				continue
			}
			if p.Title == "Terminal failures by cause" {
				if strings.Contains(pane.Queries[0].Expr, "${__field.labels.game}") && strings.Contains(pane.Queries[0].Expr, "${__field.labels.failure_type}") {
					causeLink = true
				}
			}
		}
	}
	if links < 4 || !causeLink {
		t.Fatalf("missing metric-to-Loki correlation links: links=%d cause=%v", links, causeLink)
	}
}
