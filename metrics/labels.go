package metrics

import (
	"fmt"
	"regexp"
	"strings"
)

// DurationBuckets are the default latency histogram buckets, in seconds.
var DurationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

const metricPrefix = "pokepilot_"

// Service label values. They match the Swarm service names operators scrape.
const (
	ServiceWall      = "wall"
	ServiceUI        = "ui"
	ServiceSpectator = "spectator"
	ServiceRunner    = "runner"
	ServiceReplay    = "replay"
	ServiceIssues    = "issues"
	ServiceTelegram  = "telegram"
	ServiceTrader    = "virtualtrader"
	ServiceWatch     = "watch"
)

var (
	services = map[string]struct{}{
		ServiceWall:      {},
		ServiceUI:        {},
		ServiceSpectator: {},
		ServiceRunner:    {},
		ServiceReplay:    {},
		ServiceIssues:    {},
		ServiceTelegram:  {},
		ServiceTrader:    {},
		ServiceWatch:     {},
	}
	// forbiddenLabels are names that would explode cardinality or copy
	// unbounded request data into Prometheus. Any name ending in _id is also
	// rejected, even when it is not listed here.
	forbiddenLabels = map[string]struct{}{
		"id":         {},
		"run_id":     {},
		"failure_id": {},
		"family_id":  {},
		"trace_id":   {},
		"span_id":    {},
		"goal_id":    {},
		"job_id":     {},
		"user_id":    {},
		"chat_id":    {},
		"url":        {},
		"path":       {},
		"exception":  {},
		"error":      {},
		"message":    {},
		"prompt":     {},
	}
	labelName     = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	revision      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
	counterName   = regexp.MustCompile(`^pokepilot_[a-z0-9_]+_total$`)
	gaugeName     = regexp.MustCompile(`^pokepilot_[a-z0-9_]+$`)
	histogramName = regexp.MustCompile(`^pokepilot_[a-z0-9_]+_(seconds|bytes|frames|rounds)$`)
)

func checkService(service string) error {
	if _, ok := services[service]; !ok {
		return fmt.Errorf("metrics: unknown service %q", service)
	}
	return nil
}

func normalizeRevision(value string) string {
	value = strings.TrimSpace(value)
	if !revision.MatchString(value) {
		return "dev"
	}
	return value
}

// CheckLabels reports names that must not be Prometheus labels.
func CheckLabels(names []string) error {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if !labelName.MatchString(name) {
			return fmt.Errorf("metrics: label %q is not a Prometheus label name", name)
		}
		if _, forbidden := forbiddenLabels[name]; forbidden || strings.HasSuffix(name, "_id") {
			return fmt.Errorf("metrics: label %q is unbounded; keep identifiers in logs and traces", name)
		}
		if _, dup := seen[name]; dup {
			return fmt.Errorf("metrics: duplicate label %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func checkName(name string, pattern *regexp.Regexp, kind string) error {
	if !strings.HasPrefix(name, metricPrefix) || !pattern.MatchString(name) {
		return fmt.Errorf("metrics: %s name %q must match %s", kind, name, pattern.String())
	}
	return nil
}
