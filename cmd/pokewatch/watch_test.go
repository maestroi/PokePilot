package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

func check(s operatorapi.OpsSnapshot, name string) (operatorapi.CheckResult, bool) {
	for _, c := range s.Checks {
		if c.Name == name {
			return c, true
		}
	}
	return operatorapi.CheckResult{}, false
}

func TestEvaluateFlagsUnreachableManagerAndQuorum(t *testing.T) {
	w := newWatcherForTest()
	now := time.Unix(1_800_000_000, 0)
	s := w.evaluate(now, []operatorapi.SwarmNode{
		{Hostname: "m1", Manager: true, Status: "ready", ManagerStatus: "leader"},
		{Hostname: "m2", Manager: true, Status: "ready", ManagerStatus: "reachable"},
		{Hostname: "m3", Manager: true, Status: "ready", ManagerStatus: "unreachable"},
		{Hostname: "w1", Status: "down"},
	}, nil, nil)
	if c, _ := check(s, "node:m3"); c.OK || !strings.Contains(c.Message, "unreachable") {
		t.Fatalf("m3: %+v", c)
	}
	if c, _ := check(s, "node:w1"); c.OK {
		t.Fatalf("w1 down: %+v", c)
	}
	if c, _ := check(s, "quorum"); c.OK || !strings.Contains(c.Message, "2/3") {
		t.Fatalf("quorum: %+v", c)
	}
}

func TestEvaluateReplicasAndRollback(t *testing.T) {
	w := newWatcherForTest()
	s := w.evaluate(time.Now(), nil, []operatorapi.ServiceState{
		{Name: "pokefarm_wall", Running: 1, Desired: 1},
		{Name: "pokefixer_fixer", Running: 1, Desired: 2},
		{Name: "pokefarm_ui", Running: 1, Desired: 1, UpdateState: "rollback_completed"},
	}, nil)
	if c, _ := check(s, "service:pokefixer_fixer"); c.OK || c.Grace != 3 {
		t.Fatalf("fixer replicas: %+v", c)
	}
	if c, _ := check(s, "service:pokefarm_wall"); !c.OK {
		t.Fatalf("wall ok: %+v", c)
	}
	if c, _ := check(s, "rollback:pokefarm_ui"); c.OK {
		t.Fatalf("rollback: %+v", c)
	}
}

func TestEvaluateDiskAndMissingNodeReport(t *testing.T) {
	w := newWatcherForTest()
	now := time.Unix(1_800_000_000, 0)
	w.reports["n1"] = operatorapi.NodeReport{Node: "n1", At: now.Add(-time.Minute).Unix(), Disks: []operatorapi.DiskFree{{Mount: "/", FreeGB: 5}}}
	w.reports["n2"] = operatorapi.NodeReport{Node: "n2", At: now.Add(-20 * time.Minute).Unix()}
	s := w.evaluate(now, []operatorapi.SwarmNode{{Hostname: "n1", Status: "ready"}, {Hostname: "n2", Status: "ready"}, {Hostname: "n3", Status: "ready"}}, nil, nil)
	if c, _ := check(s, "disk:n1:/"); c.OK || !strings.Contains(c.Message, "5G") {
		t.Fatalf("disk: %+v", c)
	}
	if c, _ := check(s, "node-report:n2"); c.OK {
		t.Fatalf("stale report: %+v", c)
	}
	if c, ok := check(s, "node-report:n3"); !ok || c.OK {
		t.Fatalf("never-reported node must fail once watch has run 15m: %+v", c)
	}
}

func TestEvaluateFixerChecks(t *testing.T) {
	w := newWatcherForTest()
	now := time.Now()
	w.reports["f"] = operatorapi.NodeReport{Node: "f", At: now.Unix(), Fixer: &operatorapi.FixerSummary{PaidStarts24h: 20, PaidCap: 20, BlockedKeys: []string{"k1"}}}
	s := w.evaluate(now, []operatorapi.SwarmNode{{Hostname: "f", Status: "ready"}}, nil, nil)
	if c, _ := check(s, "paid-cap"); c.OK {
		t.Fatalf("paid cap: %+v", c)
	}
	if c, _ := check(s, "blocked-keys"); c.OK || !strings.Contains(c.Message, "k1") {
		t.Fatalf("blocked: %+v", c)
	}
	if s.Fixer == nil || s.Fixer.PaidStarts24h != 20 {
		t.Fatalf("snapshot fixer: %+v", s.Fixer)
	}
}

func TestFreezeSetsAndLiftsLabel(t *testing.T) {
	w := newWatcherForTest()
	now := time.Unix(1_800_000_000, 0)
	events := []rollbackEvent{{"a", now.Add(-3 * time.Hour)}, {"b", now.Add(-2 * time.Hour)}, {"c", now.Add(-time.Hour)}}
	until, changed := w.freezeDecision(now, events, 0)
	if !changed || until != now.Add(24*time.Hour).Unix() {
		t.Fatalf("freeze: %d %v", until, changed)
	}
	until, changed = w.freezeDecision(now.Add(25*time.Hour), nil, until)
	if !changed || until != 0 {
		t.Fatalf("lift: %d %v", until, changed)
	}
}

func TestNodeReportEndpointRequiresToken(t *testing.T) {
	w := newWatcherForTest()
	h := w.handler()
	body, _ := json.Marshal(operatorapi.NodeReport{Node: "n9"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/node-report", strings.NewReader(string(body))))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/node-report", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || w.reports["n9"].Node != "n9" {
		t.Fatalf("with token: %d %+v", rec.Code, w.reports)
	}
}

func TestDockerClientDecodesNodesAndServices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/nodes"):
			_, _ = rw.Write([]byte(`[{"Description":{"Hostname":"m1"},"Spec":{"Role":"manager"},"Status":{"State":"ready"},"ManagerStatus":{"Reachability":"reachable","Leader":true}}]`))
		case strings.HasSuffix(r.URL.Path, "/services"):
			_, _ = rw.Write([]byte(`[{"ID":"x","Version":{"Index":5},"Spec":{"Name":"pokefarm_wall","Labels":{"com.docker.stack.namespace":"pokefarm"}},"ServiceStatus":{"RunningTasks":1,"DesiredTasks":1},"UpdateStatus":{"State":"rollback_completed","CompletedAt":"2026-10-03T00:00:00Z"}}]`))
		}
	}))
	defer srv.Close()
	d := &dockerClient{http: srv.Client(), base: srv.URL}
	nodes, err := d.Nodes()
	if err != nil || len(nodes) != 1 || nodes[0].ManagerStatus != "leader" {
		t.Fatalf("nodes: %+v %v", nodes, err)
	}
	svcs, rolls, err := d.Services([]string{"pokefarm"})
	if err != nil || len(svcs) != 1 || svcs[0].Running != 1 || len(rolls) != 1 || rolls[0].Service != "pokefarm_wall" {
		t.Fatalf("services: %+v %+v %v", svcs, rolls, err)
	}
}

func newWatcherForTest() *watcher {
	return &watcher{
		token: "test", minFreeGB: 20, freezeRollbacks: 3, freezeFor: 24 * time.Hour,
		reports: map[string]operatorapi.NodeReport{}, started: time.Unix(0, 0),
		mergedCount: -1, farmOpened: -1, farmClosed: -1,
	}
}

type fakeDocker struct {
	t         *testing.T
	failLabel bool
	nodes     string
	posted    []byte
	postPath  string
	query     string
}

func (f *fakeDocker) start() (*dockerClient, func()) {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/nodes"):
			if f.nodes == "" {
				f.nodes = `[]`
			}
			_, _ = rw.Write([]byte(f.nodes))
		case strings.HasSuffix(p, "/services"):
			f.query = r.URL.RawQuery
			_, _ = rw.Write([]byte(`[]`))
		case strings.HasSuffix(p, "/update"):
			f.postPath = p + "?" + r.URL.RawQuery
			f.posted, _ = io.ReadAll(r.Body)
		case strings.Contains(p, "/services/"):
			if f.failLabel {
				rw.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, _ = rw.Write([]byte(`{"ID":"x","Version":{"Index":7},"Spec":{"Name":"pokefarm_wall","Labels":{"keep":"1"},"Mode":{"Replicated":{"Replicas":1}},"TaskTemplate":{"ForceUpdate":9007199254740993}}}`))
		}
	}))
	return &dockerClient{http: srv.Client(), base: srv.URL + "/v1.43"}, srv.Close
}

func freezeWatcher(d *dockerClient) *watcher {
	w := newWatcherForTest()
	w.docker, w.freezeService = d, "pokefarm-ops_watch"
	w.seen = map[string]time.Time{}
	return w
}

func TestTickWritesFreezeLabelPreservingSpec(t *testing.T) {
	f := &fakeDocker{t: t}
	d, stop := f.start()
	defer stop()
	w := freezeWatcher(d)
	now := time.Unix(1_800_000_000, 0)
	w.seen = map[string]time.Time{"a|1": now.Add(-3 * time.Hour), "b|1": now.Add(-2 * time.Hour), "c|1": now.Add(-time.Hour)}
	w.tick(now)
	if f.postPath != "/v1.43/services/pokefarm-ops_watch/update?version=7" {
		t.Fatalf("post path: %q", f.postPath)
	}
	var spec map[string]any
	dec := json.NewDecoder(strings.NewReader(string(f.posted)))
	dec.UseNumber()
	if err := dec.Decode(&spec); err != nil {
		t.Fatal(err)
	}
	labels := spec["Labels"].(map[string]any)
	if labels["keep"] != "1" || labels[frozenLabel] != strconv.FormatInt(now.Add(24*time.Hour).Unix(), 10) || len(labels) != 2 {
		t.Fatalf("labels: %v", labels)
	}
	if !strings.Contains(string(f.posted), `"ForceUpdate":9007199254740993`) || spec["Mode"] == nil || spec["Name"] != "pokefarm_wall" {
		t.Fatalf("spec not preserved: %s", f.posted)
	}
	if !strings.Contains(f.query, "status=true") {
		t.Fatalf("services query: %q", f.query)
	}
}

func TestTickLabelReadFailureDoesNotWriteAndFails(t *testing.T) {
	f := &fakeDocker{t: t, failLabel: true}
	d, stop := f.start()
	defer stop()
	w := freezeWatcher(d)
	now := time.Unix(1_800_000_000, 0)
	w.seen = map[string]time.Time{"a|1": now.Add(-3 * time.Hour), "b|1": now.Add(-2 * time.Hour), "c|1": now.Add(-time.Hour)}
	s := w.tick(now)
	if f.posted != nil {
		t.Fatalf("wrote label despite read failure: %s", f.posted)
	}
	if c, _ := check(s, "deploy-frozen"); c.OK || !strings.Contains(c.Message, "cannot read freeze label") {
		t.Fatalf("deploy-frozen: %+v", c)
	}
}

func TestTickDockerFailureKeepsCachedChecks(t *testing.T) {
	f := &fakeDocker{t: t}
	d, stop := f.start()
	w := freezeWatcher(d)
	w.lastNodes = []operatorapi.SwarmNode{{Hostname: "w1", Status: "ready"}}
	stop() // every Docker call now fails
	s := w.tick(time.Unix(1_800_000_000, 0))
	if _, ok := check(s, "node:w1"); !ok {
		t.Fatal("node check vanished on docker outage")
	}
	if c, _ := check(s, "docker-api"); c.OK || !strings.Contains(c.Message, "Swarm state") || !strings.Contains(c.Message, "services") {
		t.Fatalf("docker-api: %+v", c)
	}
}

func TestDefaultFreezeServiceIsWatcherOwn(t *testing.T) {
	t.Setenv("POKEWATCH_FREEZE_SERVICE", "")
	if got := defaultFreezeService(); got != "pokefarm-ops_watch" {
		t.Fatalf("freeze service default %q", got)
	}
}

func TestSnapshotWarmth(t *testing.T) {
	f := &fakeDocker{t: t, nodes: `[{"Description":{"Hostname":"n1"},"Spec":{"Role":"worker"},"Status":{"State":"ready"}}]`}
	d, stop := f.start()
	defer stop()
	w := freezeWatcher(d)
	now := time.Unix(1_800_000_000, 0)
	w.started = now
	if s := w.tick(now); s.Warm {
		t.Fatal("fresh watcher without node reports must be cold")
	}
	w.reports["n1"] = operatorapi.NodeReport{Node: "n1", At: now.Unix()}
	if s := w.tick(now.Add(time.Minute)); !s.Warm {
		t.Fatal("every ready node reported and Docker read: warm")
	}

	// Docker never read: cold even after 15 minutes.
	f2 := &fakeDocker{t: t}
	d2, stop2 := f2.start()
	w2 := freezeWatcher(d2)
	w2.started = now
	stop2()
	if s := w2.tick(now.Add(time.Hour)); s.Warm {
		t.Fatal("no successful Docker read: must stay cold")
	}
}

func TestEvaluateFixerReportMissing(t *testing.T) {
	w := newWatcherForTest()
	w.warm = true
	now := time.Now()
	svcs := []operatorapi.ServiceState{{Name: "pokefixer_fixer", Running: 1, Desired: 1}}
	w.reports["n1"] = operatorapi.NodeReport{Node: "n1", At: now.Unix()}
	s := w.evaluate(now, nil, svcs, nil)
	if c, ok := check(s, "fixer-report"); !ok || c.OK || c.Grace != 2 {
		t.Fatalf("fixer running without ledger summary: %+v %v", c, ok)
	}
	w.reports["n1"] = operatorapi.NodeReport{Node: "n1", At: now.Unix(), Fixer: &operatorapi.FixerSummary{}}
	if c, _ := check(w.evaluate(now, nil, svcs, nil), "fixer-report"); !c.OK {
		t.Fatalf("summary present: %+v", c)
	}
	svcs[0].Desired = 0
	delete(w.reports, "n1")
	if c, _ := check(w.evaluate(now, nil, svcs, nil), "fixer-report"); !c.OK {
		t.Fatalf("fixer scaled to zero: %+v", c)
	}
}
