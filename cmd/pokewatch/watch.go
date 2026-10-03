package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maestroi/pokepilot/operatorapi"
)

const frozenLabel = "pokepilot.deploy-frozen-until"

type watcher struct {
	token           string
	minFreeGB       int
	freezeRollbacks int
	freezeFor       time.Duration
	freezeService   string
	stacks          []string
	docker          *dockerClient
	gh              *githubClient
	pushURL         string
	started         time.Time

	// tick-goroutine only: last good Docker reads and freeze value.
	lastNodes    []operatorapi.SwarmNode
	lastServices []operatorapi.ServiceState
	lastFrozen   int64

	mu      sync.Mutex
	reports map[string]operatorapi.NodeReport
	seen    map[string]time.Time // rollback service|completedAt → completedAt

	// GitHub numbers refresh every 15 min (search API is rate limited).
	ghAt        time.Time
	triagePRs   []operatorapi.PullRequest
	mergedCount int
	farmOpened  int
	farmClosed  int
}

func runWatch(ctx context.Context, token string) {
	w := &watcher{
		token:           token,
		minFreeGB:       envInt("POKEPILOT_WATCH_MIN_FREE_GB", 20),
		freezeRollbacks: envInt("POKEPILOT_FREEZE_ROLLBACKS", 3),
		freezeFor:       time.Duration(envInt("POKEPILOT_FREEZE_SECONDS", 86400)) * time.Second,
		freezeService:   env("POKEWATCH_FREEZE_SERVICE", "pokefarm_wall"),
		stacks:          splitList(env("POKEWATCH_STACKS", "pokefarm,pokefixer")),
		docker:          newDockerClient(env("DOCKER_SOCKET", "/var/run/docker.sock")),
		gh: &githubClient{http: &http.Client{Timeout: 20 * time.Second}, base: "https://api.github.com",
			token: operatorapi.ReadSecretFile(env("POKEPILOT_GITHUB_TOKEN_FILE", "")), repo: env("POKEPILOT_GITHUB_REPO", "maestroi/PokePilot")},
		pushURL:     env("POKEPILOT_TELEGRAM_URL", "http://telegram:8080") + "/v1/ops",
		started:     time.Now(),
		reports:     map[string]operatorapi.NodeReport{},
		seen:        map[string]time.Time{},
		mergedCount: -1, farmOpened: -1, farmClosed: -1,
	}
	srv := &http.Server{Addr: ":8080", Handler: w.handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() { log.Printf("pokewatch: %v", srv.ListenAndServe()) }()
	client := &http.Client{Timeout: 20 * time.Second}
	tick := time.NewTicker(envDuration("POKEWATCH_INTERVAL", time.Minute))
	defer tick.Stop()
	for {
		snap := w.tick(time.Now())
		if err := operatorapi.PostOps(ctx, client, w.pushURL, token, snap); err != nil {
			log.Printf("pokewatch: push: %v", err)
		}
		select {
		case <-ctx.Done():
			_ = srv.Close()
			return
		case <-tick.C:
		}
	}
}

func (w *watcher) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(rw http.ResponseWriter, _ *http.Request) { rw.WriteHeader(http.StatusOK) })
	mux.HandleFunc("POST /v1/node-report", func(rw http.ResponseWriter, r *http.Request) {
		if !operatorapi.OpsAuthorized(r, w.token) {
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		var rep operatorapi.NodeReport
		if err := json.NewDecoder(http.MaxBytesReader(rw, r.Body, 64<<10)).Decode(&rep); err != nil || rep.Node == "" {
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		rep.At = time.Now().Unix() // trust our clock, not the node's
		w.mu.Lock()
		w.reports[rep.Node] = rep
		w.mu.Unlock()
	})
	return mux
}

// tick gathers live state, applies the freeze, and evaluates every check.
// A failing source only fails its own check; Docker probe failures reuse the
// last good nodes/services so their checks keep their last state rather than
// vanishing (the bot resolves checks that disappear).
func (w *watcher) tick(now time.Time) operatorapi.OpsSnapshot {
	var dockerErrs []string
	nodes, err := w.docker.Nodes()
	if err != nil {
		dockerErrs = append(dockerErrs, "watcher cannot read Swarm state: "+err.Error())
		nodes = w.lastNodes
	} else {
		w.lastNodes = nodes
	}
	services, rolls, err := w.docker.Services(w.stacks)
	if err != nil {
		dockerErrs = append(dockerErrs, "watcher cannot list services: "+err.Error())
		services, rolls = w.lastServices, nil
	} else {
		w.lastServices = services
	}
	events := w.recordRollbacks(rolls, now)

	freezeCheck := operatorapi.CheckResult{Name: "deploy-frozen", OK: true, Grace: 1}
	if v, err := w.docker.ServiceLabel(w.freezeService, frozenLabel); err != nil {
		freezeCheck.OK, freezeCheck.Message = false, "cannot read freeze label: "+err.Error()
	} else if frozen, perr := parseFrozen(v); perr != nil {
		freezeCheck.OK, freezeCheck.Message = false, "cannot read freeze label: "+perr.Error()
	} else {
		if until, changed := w.freezeDecision(now, events, frozen); changed {
			value := ""
			if until > 0 {
				value = strconv.FormatInt(until, 10)
			}
			if err := w.docker.SetServiceLabel(w.freezeService, frozenLabel, value); err != nil {
				log.Printf("pokewatch: freeze label: %v", err)
				freezeCheck.OK, freezeCheck.Message = false, "cannot write freeze label: "+err.Error()
			} else {
				frozen = until
			}
		}
		w.lastFrozen = frozen
	}
	// On a read failure keep the last known value for the snapshot.
	frozen := w.lastFrozen
	if freezeCheck.OK && frozen > now.Unix() {
		freezeCheck.OK = false
		freezeCheck.Message = fmt.Sprintf("deploys frozen until %s after %d+ rollbacks; runs unaffected", time.Unix(frozen, 0).UTC().Format("Jan 2 15:04 UTC"), w.freezeRollbacks)
	}
	w.refreshGitHub(now)
	snap := w.evaluate(now, nodes, services, events)
	snap.FrozenUntil = frozen
	snap.Checks = append(snap.Checks, freezeCheck)
	if len(dockerErrs) > 0 {
		snap.Checks = append(snap.Checks, operatorapi.CheckResult{Name: "docker-api", Grace: 3, Message: strings.Join(dockerErrs, "; ")})
	} else {
		snap.Checks = append(snap.Checks, operatorapi.CheckResult{Name: "docker-api", OK: true, Grace: 3})
	}
	return snap
}

// parseFrozen reads the freeze label; empty means not frozen.
func parseFrozen(v string) (int64, error) {
	if v == "" {
		return 0, nil
	}
	return strconv.ParseInt(v, 10, 64)
}

// recordRollbacks remembers each (service, completion) once and returns the
// events inside the freeze window.
func (w *watcher) recordRollbacks(rolls []rollbackEvent, now time.Time) []rollbackEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, r := range rolls {
		key := r.Service + "|" + r.At.UTC().Format(time.RFC3339)
		if _, ok := w.seen[key]; !ok {
			w.seen[key] = r.At
		}
	}
	var out []rollbackEvent
	for key, at := range w.seen {
		if now.Sub(at) > w.freezeFor {
			delete(w.seen, key)
			continue
		}
		out = append(out, rollbackEvent{Service: strings.SplitN(key, "|", 2)[0], At: at})
	}
	return out
}

// freezeDecision returns the new frozen-until value and whether it changed.
func (w *watcher) freezeDecision(now time.Time, events []rollbackEvent, frozenUntil int64) (int64, bool) {
	if frozenUntil > 0 && now.Unix() >= frozenUntil {
		return 0, true
	}
	if frozenUntil == 0 && len(events) >= w.freezeRollbacks {
		return now.Add(w.freezeFor).Unix(), true
	}
	return frozenUntil, false
}

func (w *watcher) refreshGitHub(now time.Time) {
	if w.gh == nil || now.Sub(w.ghAt) < 15*time.Minute {
		return
	}
	w.ghAt = now
	prs, err := w.gh.TriagePRs()
	if err == nil {
		w.triagePRs = prs
	}
	since := now.Add(-24 * time.Hour).UTC().Format("2006-01-02T15:04:05Z")
	w.mergedCount = w.gh.Count("is:pr is:merged merged:>=" + since)
	w.farmOpened = w.gh.Count(`is:issue in:title "[farm]" created:>=` + since)
	w.farmClosed = w.gh.Count(`is:issue is:closed in:title "[farm]" closed:>=` + since)
}

// evaluate turns gathered state into checks and the snapshot the bot renders.
func (w *watcher) evaluate(now time.Time, nodes []operatorapi.SwarmNode, services []operatorapi.ServiceState, _ []rollbackEvent) operatorapi.OpsSnapshot {
	s := operatorapi.OpsSnapshot{At: now.Unix(), Nodes: nodes, Services: services,
		TriagePRs: w.triagePRs, Merged24h: w.mergedCount, FarmOpened: w.farmOpened, FarmClosed: w.farmClosed}
	add := func(name string, grace int, bad bool, msg string) {
		c := operatorapi.CheckResult{Name: name, Grace: grace, OK: !bad}
		if bad {
			c.Message = msg
		}
		s.Checks = append(s.Checks, c)
	}

	managers, reachable := 0, 0
	for _, n := range nodes {
		down := n.Status != "ready"
		unreach := n.Manager && n.ManagerStatus == "unreachable"
		msg := fmt.Sprintf("node %s is %s", n.Hostname, n.Status)
		if unreach {
			msg = fmt.Sprintf("manager %s is unreachable", n.Hostname)
		}
		add("node:"+n.Hostname, 2, down || unreach, msg)
		if n.Manager {
			managers++
			if !down && !unreach {
				reachable++
			}
		}
	}
	if managers > 0 {
		add("quorum", 2, reachable < managers,
			fmt.Sprintf("%d/%d managers reachable", reachable, managers))
	}

	for _, svc := range services {
		add("service:"+svc.Name, 3, svc.Running < svc.Desired, fmt.Sprintf("%s at %d/%d replicas", svc.Name, svc.Running, svc.Desired))
		add("rollback:"+svc.Name, 1, strings.HasPrefix(svc.UpdateState, "rollback"),
			fmt.Sprintf("Swarm rolled %s back from a crash-looping image (rollout holds until the next merge)", svc.Name))
	}

	w.mu.Lock()
	reports := make([]operatorapi.NodeReport, 0, len(w.reports))
	for _, r := range w.reports {
		reports = append(reports, r)
	}
	w.mu.Unlock()
	sort.Slice(reports, func(i, j int) bool { return reports[i].Node < reports[j].Node })
	s.Disks = reports
	byNode := map[string]operatorapi.NodeReport{}
	for _, r := range reports {
		byNode[r.Node] = r
		for _, d := range r.Disks {
			add("disk:"+r.Node+":"+d.Mount, 2, d.FreeGB < w.minFreeGB,
				fmt.Sprintf("%s %s has %dG free (under %dG)", r.Node, d.Mount, d.FreeGB, w.minFreeGB))
		}
		if r.Fixer != nil {
			f := *r.Fixer
			s.Fixer = &f
		}
	}
	warm := now.Sub(w.started) >= 15*time.Minute
	for _, n := range nodes {
		if n.Status != "ready" {
			continue // node:<host> already pages
		}
		r, ok := byNode[n.Hostname]
		stale := (ok && now.Unix()-r.At > 15*60) || (!ok && warm)
		add("node-report:"+n.Hostname, 1, stale, fmt.Sprintf("no node report from %s for 15m (pokefarm-ops_node task down?)", n.Hostname))
	}

	if s.Fixer != nil {
		add("paid-cap", 1, s.Fixer.PaidCap > 0 && s.Fixer.PaidStarts24h >= s.Fixer.PaidCap,
			fmt.Sprintf("paid fixer cap reached (%d/%d starts in 24h); only qwen runs until it ages out", s.Fixer.PaidStarts24h, s.Fixer.PaidCap))
		add("blocked-keys", 1, len(s.Fixer.BlockedKeys) > 0,
			"fixer stopped on triage keys (all tiers failed or verdict parked): "+strings.Join(s.Fixer.BlockedKeys, ", "))
	}

	var stuck []string
	for _, pr := range w.triagePRs {
		if now.Unix()-pr.CreatedAt > 12*3600 {
			stuck = append(stuck, fmt.Sprintf("#%d", pr.Number))
		}
	}
	add("stuck-prs", 1, len(stuck) > 0, "fixer PRs open over 12h (CI failing?): "+strings.Join(stuck, ", "))
	return s
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
