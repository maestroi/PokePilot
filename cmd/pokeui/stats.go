package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// statsDashboard is the subset of pokewall's dashboard contract needed for
// aggregate outcomes. Keeping this deliberately small lets old/new walls add
// fields without making the browser relay depend on the whole private wire.
type statsDashboard struct {
	Runs []statsRun `json:"runs"`
}

type statsRun struct {
	RunID      string       `json:"run_id"`
	Status     string       `json:"status"`
	Planner    string       `json:"planner"`
	Starter    string       `json:"starter"`
	Goal       string       `json:"goal"`
	LLMProfile string       `json:"llm_profile"`
	MaxRounds  int          `json:"max_rounds"`
	MaxFrames  int          `json:"max_frames"`
	Endless    bool         `json:"endless"`
	RandomSeed bool         `json:"random_seed"`
	Attempts   int          `json:"attempts"`
	Reason     string       `json:"reason"`
	Stats      *statsLLM    `json:"stats"`
	Player     *statsPlayer `json:"player"`
}

type statsLLM struct {
	Calls    int `json:"calls"`
	Rounds   int `json:"rounds"`
	Rejected int `json:"rejected"`
	Repeats  int `json:"repeats"`

	AvgSeconds           float64 `json:"avg_seconds"`
	SuccessfulAvgSeconds float64 `json:"successful_avg_seconds"`
	RejectedAvgSeconds   float64 `json:"rejected_avg_seconds"`
	StrategicAvgSeconds  float64 `json:"strategic_avg_seconds"`

	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	Transport        int `json:"transport"`
	Fallbacks        int `json:"fallbacks"`
	Failovers        int `json:"failovers"`

	Backend       string `json:"backend"`
	Model         string `json:"model"`
	ResponseModel string `json:"response_model"`

	StrategicCalls int `json:"strategic_calls"`
	FastCalls      int `json:"fast_calls"`

	GoalSummary  string `json:"goal_summary"`
	GoalCurrent  int    `json:"goal_current"`
	GoalTarget   int    `json:"goal_target"`
	GoalComplete bool   `json:"goal_complete"`
}

type statsPlayer struct {
	Badges []string `json:"badges"`
}

type outcomeCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type badgeBucket struct {
	Badges int `json:"badges"`
	Count  int `json:"count"`
}

type farmOutcomeStats struct {
	CompletedAttempts        int                      `json:"completed_attempts"`
	SettledRuns              int                      `json:"settled_runs"`
	UsableProgressRuns       int                      `json:"usable_progress_runs"`
	GoalTrackedRuns          int                      `json:"goal_tracked_runs"`
	GoalWins                 int                      `json:"goal_wins"`
	AtLeastOneBadge          int                      `json:"at_least_one_badge"`
	BestBadges               int                      `json:"best_badges"`
	RetryableFailureAttempts int                      `json:"retryable_failure_attempts"`
	TerminalReasons          []outcomeCount           `json:"terminal_reasons"`
	BadgeDistribution        []badgeBucket            `json:"badge_distribution"`
	EndlessExperiments       []endlessExperimentStats `json:"endless_experiments"`
	LLM                      llmAggregateStats        `json:"llm"`
}

type llmAggregateStats struct {
	TrackedRuns          int               `json:"tracked_runs"`
	Calls                int               `json:"calls"`
	Rounds               int               `json:"rounds"`
	Rejected             int               `json:"rejected"`
	Repeats              int               `json:"repeats"`
	AvgSeconds           float64           `json:"avg_seconds"`
	SuccessfulAvgSeconds float64           `json:"successful_avg_seconds"`
	RejectedAvgSeconds   float64           `json:"rejected_avg_seconds"`
	StrategicAvgSeconds  float64           `json:"strategic_avg_seconds"`
	PromptTokens         int64             `json:"prompt_tokens"`
	CompletionTokens     int64             `json:"completion_tokens"`
	Transport            int               `json:"transport"`
	Fallbacks            int               `json:"fallbacks"`
	Failovers            int               `json:"failovers"`
	StrategicCalls       int               `json:"strategic_calls"`
	FastCalls            int               `json:"fast_calls"`
	Profiles             []llmProfileStats `json:"profiles"`
	LatestModels         []llmModelStats   `json:"latest_models"`
}

type llmProfileStats struct {
	Profile          string  `json:"profile"`
	Runs             int     `json:"runs"`
	Calls            int     `json:"calls"`
	Rejected         int     `json:"rejected"`
	AvgSeconds       float64 `json:"avg_seconds"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	Transport        int     `json:"transport"`
	Fallbacks        int     `json:"fallbacks"`
}

type llmModelStats struct {
	Name    string `json:"name"`
	Backend string `json:"backend,omitempty"`
	Runs    int    `json:"runs"`
}

type llmWeightSums struct {
	allSeconds       float64
	successSeconds   float64
	successCalls     int
	rejectedSeconds  float64
	rejectedCalls    int
	strategicSeconds float64
	strategicCalls   int
}

type llmProfileAccumulator struct {
	stats         llmProfileStats
	latencySum    float64
	latencyWeight int
}

type endlessExperimentStats struct {
	Key                      string         `json:"key"`
	Planner                  string         `json:"planner"`
	Starter                  string         `json:"starter,omitempty"`
	Goal                     string         `json:"goal,omitempty"`
	LLMProfile               string         `json:"llm_profile,omitempty"`
	MaxRounds                int            `json:"max_rounds"`
	MaxFrames                int            `json:"max_frames"`
	RandomSeed               bool           `json:"random_seed"`
	RunRecords               int            `json:"run_records"`
	SettledRuns              int            `json:"settled_runs"`
	CompletedAttempts        int            `json:"completed_attempts"`
	UsableProgressRuns       int            `json:"usable_progress_runs"`
	GoalTrackedRuns          int            `json:"goal_tracked_runs"`
	GoalWins                 int            `json:"goal_wins"`
	AtLeastOneBadge          int            `json:"at_least_one_badge"`
	BestBadges               int            `json:"best_badges"`
	RetryableFailureAttempts int            `json:"retryable_failure_attempts"`
	TerminalReasons          []outcomeCount `json:"terminal_reasons"`
}

// statsHandler turns the wall's durable dashboard snapshot into aggregate
// benchmark data. It is intentionally read-only: no runner/farm protocol
// changes are required, and old wall state remains valid.
func statsHandler(wallBase string) http.HandlerFunc {
	client := &http.Client{Timeout: proxyTimeout}
	return func(res http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), proxyTimeout)
		defer cancel()

		up, err := http.NewRequestWithContext(ctx, http.MethodGet, wallBase+"/v1/dashboard", nil)
		if err != nil {
			writeUnreachable(res)
			return
		}
		resp, err := client.Do(up)
		if err != nil {
			writeUnreachable(res)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			writeUnreachable(res)
			return
		}

		var dashboard statsDashboard
		if err := json.NewDecoder(resp.Body).Decode(&dashboard); err != nil {
			writeUnreachable(res)
			return
		}
		res.Header().Set("Content-Type", "application/json")
		res.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(res).Encode(summarizeOutcomes(dashboard.Runs))
	}
}

func summarizeOutcomes(runs []statsRun) farmOutcomeStats {
	var out farmOutcomeStats
	reasons := map[string]int{}
	badges := make([]int, 9)
	groups := map[string]*endlessExperimentStats{}
	profiles := map[string]*llmProfileAccumulator{}
	models := map[string]*llmModelStats{}
	var llmWeights llmWeightSums

	for _, run := range runs {
		attempts := completedAttempts(run)
		out.CompletedAttempts += attempts
		out.RetryableFailureAttempts += retryableFailures(run, attempts)
		addLLMStats(&out.LLM, &llmWeights, profiles, models, run)

		settled := isSettled(run)
		if settled {
			out.SettledRuns++
			reason := strings.TrimSpace(run.Reason)
			if reason == "" {
				reason = "unknown"
			}
			reasons[reason]++
		}

		if settled && run.Player != nil {
			n := len(run.Player.Badges)
			if n > 8 {
				n = 8
			}
			out.UsableProgressRuns++
			badges[n]++
			if n > 0 {
				out.AtLeastOneBadge++
			}
			if n > out.BestBadges {
				out.BestBadges = n
			}
		}
		if settled && goalTracked(run.Stats) {
			out.GoalTrackedRuns++
			if run.Stats.GoalComplete {
				out.GoalWins++
			}
		}

		if run.Endless {
			key := endlessKey(run)
			g := groups[key]
			if g == nil {
				g = &endlessExperimentStats{
					Key:        key,
					Planner:    run.Planner,
					Starter:    run.Starter,
					Goal:       run.Goal,
					LLMProfile: run.LLMProfile,
					MaxRounds:  run.MaxRounds,
					MaxFrames:  run.MaxFrames,
					RandomSeed: run.RandomSeed,
				}
				groups[key] = g
			}
			addRunToEndless(g, run, attempts)
		}
	}

	finalizeLLMStats(&out.LLM, llmWeights, profiles, models)
	out.TerminalReasons = sortedCounts(reasons)
	out.BadgeDistribution = make([]badgeBucket, 0, len(badges))
	for i, count := range badges {
		out.BadgeDistribution = append(out.BadgeDistribution, badgeBucket{Badges: i, Count: count})
	}
	out.EndlessExperiments = make([]endlessExperimentStats, 0, len(groups))
	for _, g := range groups {
		out.EndlessExperiments = append(out.EndlessExperiments, *g)
	}
	sort.Slice(out.EndlessExperiments, func(i, j int) bool {
		a, b := out.EndlessExperiments[i], out.EndlessExperiments[j]
		if a.CompletedAttempts != b.CompletedAttempts {
			return a.CompletedAttempts > b.CompletedAttempts
		}
		return a.Key < b.Key
	})
	return out
}

func addLLMStats(out *llmAggregateStats, weights *llmWeightSums, profiles map[string]*llmProfileAccumulator, models map[string]*llmModelStats, run statsRun) {
	s := run.Stats
	if s == nil || s.Calls <= 0 {
		return
	}
	out.TrackedRuns++
	out.Calls += s.Calls
	out.Rounds += s.Rounds
	out.Rejected += s.Rejected
	out.Repeats += s.Repeats
	out.PromptTokens += int64(s.PromptTokens)
	out.CompletionTokens += int64(s.CompletionTokens)
	out.Transport += s.Transport
	out.Fallbacks += s.Fallbacks
	out.Failovers += s.Failovers
	out.StrategicCalls += s.StrategicCalls
	out.FastCalls += s.FastCalls
	weights.allSeconds += s.AvgSeconds * float64(s.Calls)

	successCalls := s.Calls - s.Rejected
	if successCalls > 0 && s.SuccessfulAvgSeconds > 0 {
		weights.successSeconds += s.SuccessfulAvgSeconds * float64(successCalls)
		weights.successCalls += successCalls
	}
	if s.Rejected > 0 && s.RejectedAvgSeconds > 0 {
		weights.rejectedSeconds += s.RejectedAvgSeconds * float64(s.Rejected)
		weights.rejectedCalls += s.Rejected
	}
	if s.StrategicCalls > 0 && s.StrategicAvgSeconds > 0 {
		weights.strategicSeconds += s.StrategicAvgSeconds * float64(s.StrategicCalls)
		weights.strategicCalls += s.StrategicCalls
	}

	profile := strings.TrimSpace(run.LLMProfile)
	if profile == "" {
		profile = "legacy"
	}
	p := profiles[profile]
	if p == nil {
		p = &llmProfileAccumulator{stats: llmProfileStats{Profile: profile}}
		profiles[profile] = p
	}
	p.stats.Runs++
	p.stats.Calls += s.Calls
	p.stats.Rejected += s.Rejected
	p.stats.PromptTokens += int64(s.PromptTokens)
	p.stats.CompletionTokens += int64(s.CompletionTokens)
	p.stats.Transport += s.Transport
	p.stats.Fallbacks += s.Fallbacks
	p.latencySum += s.AvgSeconds * float64(s.Calls)
	p.latencyWeight += s.Calls

	model := strings.TrimSpace(s.ResponseModel)
	if model == "" {
		model = strings.TrimSpace(s.Model)
	}
	if model != "" {
		backend := strings.TrimSpace(s.Backend)
		key := model + "\x1f" + backend
		m := models[key]
		if m == nil {
			m = &llmModelStats{Name: model, Backend: backend}
			models[key] = m
		}
		m.Runs++
	}
}

func finalizeLLMStats(out *llmAggregateStats, weights llmWeightSums, profiles map[string]*llmProfileAccumulator, models map[string]*llmModelStats) {
	if out.Calls > 0 {
		out.AvgSeconds = weights.allSeconds / float64(out.Calls)
	}
	if weights.successCalls > 0 {
		out.SuccessfulAvgSeconds = weights.successSeconds / float64(weights.successCalls)
	}
	if weights.rejectedCalls > 0 {
		out.RejectedAvgSeconds = weights.rejectedSeconds / float64(weights.rejectedCalls)
	}
	if weights.strategicCalls > 0 {
		out.StrategicAvgSeconds = weights.strategicSeconds / float64(weights.strategicCalls)
	}

	out.Profiles = make([]llmProfileStats, 0, len(profiles))
	for _, p := range profiles {
		if p.latencyWeight > 0 {
			p.stats.AvgSeconds = p.latencySum / float64(p.latencyWeight)
		}
		out.Profiles = append(out.Profiles, p.stats)
	}
	sort.Slice(out.Profiles, func(i, j int) bool {
		if out.Profiles[i].Calls != out.Profiles[j].Calls {
			return out.Profiles[i].Calls > out.Profiles[j].Calls
		}
		return out.Profiles[i].Profile < out.Profiles[j].Profile
	})

	out.LatestModels = make([]llmModelStats, 0, len(models))
	for _, m := range models {
		out.LatestModels = append(out.LatestModels, *m)
	}
	sort.Slice(out.LatestModels, func(i, j int) bool {
		if out.LatestModels[i].Runs != out.LatestModels[j].Runs {
			return out.LatestModels[i].Runs > out.LatestModels[j].Runs
		}
		if out.LatestModels[i].Name != out.LatestModels[j].Name {
			return out.LatestModels[i].Name < out.LatestModels[j].Name
		}
		return out.LatestModels[i].Backend < out.LatestModels[j].Backend
	})
}

func addRunToEndless(g *endlessExperimentStats, run statsRun, attempts int) {
	g.RunRecords++
	g.CompletedAttempts += attempts
	g.RetryableFailureAttempts += retryableFailures(run, attempts)
	if !isSettled(run) {
		return
	}
	g.SettledRuns++
	reason := strings.TrimSpace(run.Reason)
	if reason == "" {
		reason = "unknown"
	}
	counts := make(map[string]int, len(g.TerminalReasons)+1)
	for _, item := range g.TerminalReasons {
		counts[item.Name] = item.Count
	}
	counts[reason]++
	g.TerminalReasons = sortedCounts(counts)

	if run.Player != nil {
		n := len(run.Player.Badges)
		if n > 8 {
			n = 8
		}
		g.UsableProgressRuns++
		if n > 0 {
			g.AtLeastOneBadge++
		}
		if n > g.BestBadges {
			g.BestBadges = n
		}
	}
	if goalTracked(run.Stats) {
		g.GoalTrackedRuns++
		if run.Stats.GoalComplete {
			g.GoalWins++
		}
	}
}

func completedAttempts(run statsRun) int {
	if run.Attempts > 0 {
		return run.Attempts
	}
	// Very old persisted tiles predate the attempts counter. A settled tile is
	// still one completed attempt; treating it as zero would skew historical
	// denominators after upgrading the UI.
	if isSettled(run) {
		return 1
	}
	return 0
}

// retryableFailures recovers failures hidden by successful retries. Pokewall
// retries only error/lost; therefore every completed attempt before a final
// non-error settlement is exactly one error-or-lost attempt. If the final
// reason is error/lost too, all completed attempts are retryable failures.
func retryableFailures(run statsRun, attempts int) int {
	if attempts <= 0 {
		return 0
	}
	if !isSettled(run) {
		return attempts
	}
	switch run.Reason {
	case "error", "lost":
		return attempts
	default:
		if attempts > 1 {
			return attempts - 1
		}
		return 0
	}
}

func isSettled(run statsRun) bool {
	return run.Status == "done" || strings.TrimSpace(run.Reason) != ""
}

func goalTracked(stats *statsLLM) bool {
	return stats != nil && (stats.GoalComplete || stats.GoalTarget > 0 || strings.TrimSpace(stats.GoalSummary) != "")
}

func sortedCounts(counts map[string]int) []outcomeCount {
	out := make([]outcomeCount, 0, len(counts))
	for name, count := range counts {
		out = append(out, outcomeCount{Name: name, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Endless mode creates independent successor runs with the same settings.
// Until the wall carries an explicit lineage id, identical configurations are
// the stable experiment boundary. This is enough for long random-seed farm
// benchmarks and is intentionally exposed as a config key, not a run id.
func endlessKey(run statsRun) string {
	parts := []string{
		run.Planner,
		run.Starter,
		run.Goal,
		run.LLMProfile,
		strconv.Itoa(run.MaxRounds),
		strconv.Itoa(run.MaxFrames),
		strconv.FormatBool(run.RandomSeed),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return fmt.Sprintf("%x", sum[:6])
}
