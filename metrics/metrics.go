package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Options identifies the process publishing this registry.
type Options struct {
	// Service is one of the Service* constants.
	Service string
	// Revision is the git SHA stamped into the binary. Empty or unsafe values
	// are published as dev.
	Revision string
}

// Registry is one process's Prometheus registry. It is not the global default,
// so libraries cannot accidentally publish series into it.
type Registry struct {
	reg     *prometheus.Registry
	service string
	handler http.Handler
}

// New returns a registry that already exports build metadata, a liveness
// gauge, Go runtime metrics, and process metrics.
func New(opts Options) (*Registry, error) {
	if err := checkService(opts.Service); err != nil {
		return nil, err
	}
	rev := normalizeRevision(opts.Revision)
	reg := prometheus.NewRegistry()
	build := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pokepilot_build_info",
		Help: "Build metadata for this process. The sample value is always 1.",
	}, []string{"service", "revision"})
	build.WithLabelValues(opts.Service, rev).Set(1)
	up := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pokepilot_up",
		Help: "1 while this process is serving its metrics endpoint.",
	}, []string{"service"})
	up.WithLabelValues(opts.Service).Set(1)
	if err := reg.Register(build); err != nil {
		return nil, err
	}
	if err := reg.Register(up); err != nil {
		return nil, err
	}
	if err := reg.Register(collectors.NewGoCollector()); err != nil {
		return nil, err
	}
	if err := reg.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})); err != nil {
		return nil, err
	}
	r := &Registry{reg: reg, service: opts.Service}
	r.handler = promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	return r, nil
}

// Must is New for process startup, where an invalid service name is a bug.
func Must(opts Options) *Registry {
	reg, err := New(opts)
	if err != nil {
		panic(err)
	}
	return reg
}

// Handler serves the Prometheus text exposition of this registry.
func (r *Registry) Handler() http.Handler { return r.handler }

// Wrap serves GET /metrics from this registry and forwards every other
// request to next.
func (r *Registry) Wrap(next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/metrics" && (req.Method == http.MethodGet || req.Method == http.MethodHead) {
			r.handler.ServeHTTP(w, req)
			return
		}
		next.ServeHTTP(w, req)
	})
}

// Register adds a collector that has already been checked against the label
// rules. Service-specific collectors, such as the Telegram operator, use this.
func (r *Registry) Register(c prometheus.Collector) error {
	return r.reg.Register(c)
}

// NewCounterVec registers a counter. Name must start with pokepilot_ and end
// with _total. labels must pass CheckLabels.
func (r *Registry) NewCounterVec(name, help string, labels []string) (*prometheus.CounterVec, error) {
	if err := checkName(name, counterName, "counter"); err != nil {
		return nil, err
	}
	if err := CheckLabels(labels); err != nil {
		return nil, err
	}
	cv := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels)
	if err := r.reg.Register(cv); err != nil {
		return nil, err
	}
	return cv, nil
}

// NewGaugeVec registers a gauge. Name must start with pokepilot_.
func (r *Registry) NewGaugeVec(name, help string, labels []string) (*prometheus.GaugeVec, error) {
	if err := checkName(name, gaugeName, "gauge"); err != nil {
		return nil, err
	}
	if err := CheckLabels(labels); err != nil {
		return nil, err
	}
	gv := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: help}, labels)
	if err := r.reg.Register(gv); err != nil {
		return nil, err
	}
	return gv, nil
}

// NewHistogramVec registers a histogram. Name must end in _seconds or _bytes.
// A nil or empty bucket list uses DurationBuckets.
func (r *Registry) NewHistogramVec(name, help string, labels []string, buckets []float64) (*prometheus.HistogramVec, error) {
	if err := checkName(name, histogramName, "histogram"); err != nil {
		return nil, err
	}
	if err := CheckLabels(labels); err != nil {
		return nil, err
	}
	hv := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    name,
		Help:    help,
		Buckets: copyBuckets(buckets),
	}, labels)
	if err := r.reg.Register(hv); err != nil {
		return nil, err
	}
	return hv, nil
}

func copyBuckets(buckets []float64) []float64 {
	if len(buckets) == 0 {
		buckets = DurationBuckets
	}
	out := make([]float64, len(buckets))
	copy(out, buckets)
	return out
}
