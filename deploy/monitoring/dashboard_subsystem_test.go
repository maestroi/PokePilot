package monitoring

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Series these dashboards are allowed to query. LLM request, token, replay
// queue, and HTTP RED metrics are not exported yet; a panel must not invent them.
var subsystemMetrics = map[string]bool{
	"pokepilot_runs_started_total":   true,
	"pokepilot_runs_completed_total": true,
	"pokepilot_runs_failed_total":    true,
	"pokepilot_runs_aborted_total":   true,
	"pokepilot_run_rounds_bucket":    true,
	"pokepilot_up":                   true,
	"pokepilot_build_info":           true,
	"up":                             true,
	"go_goroutines":                  true,
	"go_memstats_heap_alloc_bytes":   true,
	"process_resident_memory_bytes":  true,
	"process_cpu_seconds_total":      true,
	"process_open_fds":               true,
	"process_start_time_seconds":     true,
}

var metricRE = regexp.MustCompile(`pokepilot_[a-z0-9_]+|go_[a-z0-9_]+|process_[a-z0-9_]+|\bup\b`)

func TestSubsystemDashboardsUseExportedSeries(t *testing.T) {
	cases := []struct {
		file    string
		uid     string
		min     int
		vars    []string
		needs   []string
		game    bool
		replay  bool
		service bool
	}{
		{
			file: "grafana-pokepilot-agent.json",
			uid:  "pokepilot-agent",
			min:  10,
			vars: []string{"DS_PROMETHEUS", "DS_LOKI", "game"},
			needs: []string{
				`mode="llm"`,
				"pokepilot_run_rounds_bucket",
				"by (mode)",
				"#2335",
				"tokens",
			},
			game: true,
		},
		{
			file: "grafana-pokepilot-replay.json",
			uid:  "pokepilot-replay",
			min:  8,
			vars: []string{"DS_PROMETHEUS", "DS_LOKI"},
			needs: []string{
				`service="replay"`,
				`job="pokepilot-replay"`,
				"process_resident_memory_bytes",
				"process_cpu_seconds_total",
				"#2335",
				"Queue depth",
			},
			replay: true,
		},
		{
			file: "grafana-pokepilot-services.json",
			uid:  "pokepilot-services",
			min:  10,
			vars: []string{"DS_PROMETHEUS", "DS_LOKI", "job", "service", "host_dashboard", "container_dashboard"},
			needs: []string{
				"${job:pipe}",
				"${service:pipe}",
				"${host_dashboard}",
				"${container_dashboard}",
				"pokepilot_build_info",
				"#2335",
				"Host dashboard UID",
			},
			service: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.uid, func(t *testing.T) {
			d := loadSubsystem(t, tc.file)
			if d.UID != tc.uid || len(d.Panels) < tc.min {
				t.Fatalf("unexpected dashboard %q with %d panels", d.UID, len(d.Panels))
			}
			vars := map[string]bool{}
			for _, v := range d.Templating.List {
				vars[v.Name] = true
			}
			for _, name := range tc.vars {
				if !vars[name] {
					t.Errorf("missing template variable %s", name)
				}
			}
			blob := d.Description + "\n" + d.rawLinks()
			for _, panel := range d.Panels {
				blob += "\n" + panel.Title + "\n" + panel.Description
				for _, target := range panel.Targets {
					blob += "\n" + target.Expr
				}
			}
			for _, expr := range annotationExprs(d) {
				blob += "\n" + expr
			}
			for _, need := range tc.needs {
				if !strings.Contains(blob, need) {
					t.Errorf("missing %q", need)
				}
			}
			assertPanels(t, d, tc.game, tc.replay, tc.service)
		})
	}
}

func TestPokePilotDashboardsLinkEachOther(t *testing.T) {
	files := []string{
		"grafana-pokepilot-overview.json",
		"grafana-pokepilot-runs.json",
		"grafana-pokepilot-failures.json",
		"grafana-pokepilot-agent.json",
		"grafana-pokepilot-replay.json",
		"grafana-pokepilot-services.json",
	}
	uids := []string{
		"/d/pokepilot-overview",
		"/d/pokepilot-runs",
		"/d/pokepilot-failures",
		"/d/pokepilot-agent",
		"/d/pokepilot-replay",
		"/d/pokepilot-services",
	}
	for i, file := range files {
		d := loadSubsystem(t, file)
		got := map[string]bool{}
		for _, link := range d.Links {
			got[link.URL] = true
			if link.Type != "link" {
				t.Errorf("%s link %s has type %s", file, link.Title, link.Type)
			}
		}
		for j, uid := range uids {
			if i == j {
				if got[uid] {
					t.Errorf("%s links to itself", file)
				}
				continue
			}
			if !got[uid] {
				t.Errorf("%s missing link to %s", file, uid)
			}
		}
	}
}

type subsystemDashboard struct {
	UID         string `json:"uid"`
	Description string `json:"description"`
	Links       []struct {
		Title string `json:"title"`
		Type  string `json:"type"`
		URL   string `json:"url"`
	} `json:"links"`
	Panels []struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		GridPos     struct {
			X int `json:"x"`
			Y int `json:"y"`
			W int `json:"w"`
			H int `json:"h"`
		} `json:"gridPos"`
		Targets []struct {
			Expr       string `json:"expr"`
			Datasource struct {
				UID string `json:"uid"`
			} `json:"datasource"`
		} `json:"targets"`
	} `json:"panels"`
	Templating struct {
		List []struct {
			Name string `json:"name"`
		} `json:"list"`
	} `json:"templating"`
	Annotations struct {
		List []struct {
			Expr string `json:"expr"`
		} `json:"list"`
	} `json:"annotations"`
}

func (d subsystemDashboard) rawLinks() string {
	var b strings.Builder
	for _, link := range d.Links {
		b.WriteString(link.URL)
		b.WriteByte('\n')
	}
	return b.String()
}

func loadSubsystem(t *testing.T, file string) subsystemDashboard {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var d subsystemDashboard
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func annotationExprs(d subsystemDashboard) []string {
	var out []string
	for _, ann := range d.Annotations.List {
		if ann.Expr != "" {
			out = append(out, ann.Expr)
		}
	}
	return out
}

func assertPanels(t *testing.T, d subsystemDashboard, game, replay, service bool) {
	t.Helper()
	grid := map[string]string{}
	ids := map[int]bool{}
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
			checkExpr(t, p.Title, target.Expr, target.Datasource.UID, game, replay, service)
		}
	}
	for _, expr := range annotationExprs(d) {
		checkExpr(t, "annotation", expr, "${DS_PROMETHEUS}", false, false, service)
	}
}

func checkExpr(t *testing.T, title, expr, datasource string, game, replay, service bool) {
	t.Helper()
	if datasource != "" && datasource != "${DS_PROMETHEUS}" {
		t.Errorf("%s hardcodes datasource %s", title, datasource)
	}
	for _, secret := range []string{"run_id", "failure_id", "family_id", "trace_id", "pokepilot_llm", "pokepilot_replay", "node_", "container_", "http_requests"} {
		if strings.Contains(expr, secret) {
			t.Errorf("%s uses %s", title, secret)
		}
	}
	metrics := metricRE.FindAllString(expr, -1)
	if len(metrics) == 0 {
		t.Errorf("%s has no known series: %s", title, expr)
	}
	for _, metric := range metrics {
		if !subsystemMetrics[metric] {
			t.Errorf("%s queries series that is not exported: %s", title, metric)
		}
	}
	joined := strings.Join(metrics, " ")
	switch {
	case game && (strings.Contains(joined, "pokepilot_runs_") || strings.Contains(joined, "pokepilot_run_rounds")):
		if !strings.Contains(expr, "${game:regex}") {
			t.Errorf("%s lacks game filter", title)
		}
		if strings.Contains(joined, "pokepilot_runs_") && !strings.Contains(expr, "mode=") && !strings.Contains(expr, "by (mode)") {
			t.Errorf("%s run counter is not limited to a planner mode", title)
		}
	case replay:
		if !strings.Contains(expr, `service="replay"`) && !strings.Contains(expr, `job="pokepilot-replay"`) {
			t.Errorf("%s is not limited to the replay process", title)
		}
	case service:
		if strings.Contains(joined, "pokepilot_") && !strings.Contains(expr, "${service:pipe}") {
			t.Errorf("%s lacks service filter", title)
		}
		if (strings.Contains(joined, "process_") || strings.Contains(joined, "go_") || strings.Contains(joined, "up")) && !strings.Contains(joined, "pokepilot_up") && !strings.Contains(expr, "${job:pipe}") {
			t.Errorf("%s lacks job filter", title)
		}
	}
}
