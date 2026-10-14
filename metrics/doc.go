// Package metrics is the Prometheus instrumentation surface for PokePilot.
//
// Every long-running service exposes GET /metrics from the registry returned
// by New. Metric names use the pokepilot_ prefix. Counters end in _total.
// Histograms that record time end in _seconds; histograms that record size
// end in _bytes. Histograms of emulator frames end in _frames, and histograms
// of planner rounds end in _rounds. Latency and runtime distributions use
// Histogram, not Summary, so percentiles can be aggregated across tasks.
//
// Labels stay bounded. Preferred labels are game, result, milestone, failure_type,
// stage, backend, service, version, mode, status, and method. Milestone
// values are the bounded funnel vocabulary in this package (starter,
// badge_1..badge_16, a fixed story list, completed), never free-form text.
// Constructors
// reject high-cardinality names: run_id, failure_id, family_id, trace_id,
// goal_id, any other *_id, and free-form values such as url, exception, and
// prompt. Those identifiers belong in structured logs and traces.
//
// Each process also publishes pokepilot_build_info (service and revision),
// pokepilot_up, the standard Go collector, and the standard process collector.
// Revision is the git SHA stamped into the binary. An empty or unsafe value
// is reported as dev.
//
// Cardinality is one series per bounded label combination per process, plus
// the fixed build, up, Go, and process series. Do not put a series per run,
// failure, goal, or request URL into this registry.
package metrics
