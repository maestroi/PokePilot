package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultReplayMaxJobs         = 2
	defaultReplayMinScratchBytes = uint64(1 << 30)
	defaultReplayJobTimeout      = 8 * time.Hour
	defaultReplaySegmentTimeout  = 2 * time.Hour
	replayScratchDirName         = "pokepilot-replay"
	replayScratchJobPrefix       = "job-"
)

type replayCapacityConfig struct {
	MaxJobs         int
	MinScratchBytes uint64
	ScratchDir      string
	JobTimeout      time.Duration
	SegmentTimeout  time.Duration
}

type replayDeferredJob struct {
	Since  time.Time
	Reason string
	Detail string
}

type replayResourceMetrics struct {
	Completed uint64
	Failures  uint64
	Retries   uint64
	Bytes     uint64
	Nanos     uint64
}

func replayCapacityFromEnv() replayCapacityConfig {
	cfg := replayCapacityConfig{
		MaxJobs:         envPositiveInt("POKEPILOT_REPLAY_MAX_JOBS", defaultReplayMaxJobs),
		MinScratchBytes: envUint64("POKEPILOT_REPLAY_MIN_SCRATCH_BYTES", defaultReplayMinScratchBytes),
		ScratchDir:      strings.TrimSpace(os.Getenv("POKEPILOT_REPLAY_SCRATCH_DIR")),
		JobTimeout:      envDuration("POKEPILOT_REPLAY_JOB_TIMEOUT", defaultReplayJobTimeout),
		SegmentTimeout:  envDuration("POKEPILOT_REPLAY_SEGMENT_TIMEOUT", defaultReplaySegmentTimeout),
	}
	if cfg.ScratchDir == "" {
		cfg.ScratchDir = filepath.Join(os.TempDir(), replayScratchDirName)
	}
	return cfg
}

func envPositiveInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func envUint64(name string, fallback uint64) uint64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

func envDuration(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func freeDiskBytes(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}

func prepareReplayScratch(root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return fmt.Errorf("replay scratch directory is empty")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), replayScratchJobPrefix) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func replayScratchUsage(root string) (uint64, error) {
	var total uint64
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), replayScratchJobPrefix) {
			continue
		}
		err := filepath.WalkDir(filepath.Join(root, entry.Name()), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Size() > 0 {
				total += uint64(info.Size())
			}
			return nil
		})
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func replayContextError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx != nil && ctx.Err() != nil {
		return fmt.Errorf("%w: %v", ctx.Err(), err)
	}
	return err
}

func withReplayTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, timeout)
}

func (s *replayServer) tryAdmitRenderJob(id string) (func(), string, string) {
	if s == nil {
		return nil, "worker_unavailable", "replay worker is nil"
	}
	id = strings.TrimSpace(id)
	if id == "" {
		id = "anonymous"
	}
	free, err := s.scratchFree(s.capacity.ScratchDir)
	if err != nil {
		detail := "scratch availability check failed: " + err.Error()
		s.deferRenderJob(id, "scratch_unavailable", detail)
		return nil, "scratch_unavailable", detail
	}
	if free < s.capacity.MinScratchBytes {
		detail := fmt.Sprintf("scratch free bytes %d below floor %d", free, s.capacity.MinScratchBytes)
		s.deferRenderJob(id, "low_scratch", detail)
		return nil, "low_scratch", detail
	}
	select {
	case s.jobSlots <- struct{}{}:
		s.resourceMu.Lock()
		delete(s.deferredJobs, id)
		s.activeJobs[id] = time.Now()
		s.resourceMu.Unlock()
		var once sync.Once
		return func() {
			once.Do(func() {
				s.resourceMu.Lock()
				delete(s.activeJobs, id)
				s.resourceMu.Unlock()
				<-s.jobSlots
			})
		}, "", ""
	default:
		s.deferRenderJob(id, "max_jobs", fmt.Sprintf("offline render concurrency limit %d reached", cap(s.jobSlots)))
		return nil, "max_jobs", fmt.Sprintf("offline render concurrency limit %d reached", cap(s.jobSlots))
	}
}

func (s *replayServer) deferRenderJob(id, reason, detail string) {
	s.resourceMu.Lock()
	defer s.resourceMu.Unlock()
	existing, ok := s.deferredJobs[id]
	if !ok {
		existing.Since = time.Now()
	}
	existing.Reason = reason
	existing.Detail = detail
	s.deferredJobs[id] = existing
}

func (s *replayServer) clearDeferredRenderJob(id string) {
	s.resourceMu.Lock()
	delete(s.deferredJobs, strings.TrimSpace(id))
	s.resourceMu.Unlock()
}

func (s *replayServer) registerRenderCancel(id string, cancel context.CancelFunc) {
	if strings.TrimSpace(id) == "" || cancel == nil {
		return
	}
	s.resourceMu.Lock()
	s.activeCancels[id] = cancel
	s.resourceMu.Unlock()
}

func (s *replayServer) unregisterRenderCancel(id string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	s.resourceMu.Lock()
	delete(s.activeCancels, id)
	s.resourceMu.Unlock()
}

func (s *replayServer) cancelActiveRender(id string) bool {
	s.resourceMu.Lock()
	cancel := s.activeCancels[strings.TrimSpace(id)]
	s.resourceMu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (s *replayServer) recordRenderSuccess(size int64, elapsed time.Duration) {
	s.renderCompleted.Add(1)
	if size > 0 {
		s.renderBytes.Add(uint64(size))
	}
	if elapsed > 0 {
		s.renderNanos.Add(uint64(elapsed))
	}
}

func (s *replayServer) replayResourceHealth() map[string]any {
	free, freeErr := s.scratchFree(s.capacity.ScratchDir)
	usage, usageErr := replayScratchUsage(s.capacity.ScratchDir)

	s.resourceMu.Lock()
	queueDepth := len(s.deferredJobs)
	activeJobs := len(s.activeJobs)
	var oldest time.Duration
	now := time.Now()
	for _, started := range s.activeJobs {
		if age := now.Sub(started); age > oldest {
			oldest = age
		}
	}
	for _, deferred := range s.deferredJobs {
		if age := now.Sub(deferred.Since); age > oldest {
			oldest = age
		}
	}
	deferred := make(map[string]int)
	for _, job := range s.deferredJobs {
		deferred[job.Reason]++
	}
	s.resourceMu.Unlock()

	completed := s.renderCompleted.Load()
	bytes := s.renderBytes.Load()
	nanos := s.renderNanos.Load()
	throughput := float64(0)
	if nanos > 0 {
		throughput = float64(bytes) / (float64(nanos) / float64(time.Second))
	}
	health := map[string]any{
		"offline_job_limit":          cap(s.jobSlots),
		"offline_jobs_active":        activeJobs,
		"offline_queue_depth":        queueDepth,
		"offline_queue_reasons":      deferred,
		"segment_worker_limit":       cap(replayRenderSlots),
		"active_segments":            len(replayRenderSlots),
		"active_encoder_processes":   s.encoderProcesses.Load(),
		"render_failures":            s.renderFailures.Load(),
		"render_retries":             s.renderRetries.Load(),
		"render_completed":           completed,
		"render_bytes":               bytes,
		"render_throughput_bytes_sec": throughput,
		"oldest_render_age_ms":       oldest.Milliseconds(),
		"scratch_dir":                s.capacity.ScratchDir,
		"scratch_min_free_bytes":     s.capacity.MinScratchBytes,
		"scratch_free_bytes":         free,
		"scratch_use_bytes":          usage,
		"scratch_ok":                 freeErr == nil && free >= s.capacity.MinScratchBytes,
		"job_timeout_seconds":        int64(s.capacity.JobTimeout / time.Second),
		"segment_timeout_seconds":    int64(s.capacity.SegmentTimeout / time.Second),
	}
	if freeErr != nil {
		health["scratch_error"] = freeErr.Error()
	}
	if usageErr != nil {
		health["scratch_usage_error"] = usageErr.Error()
	}
	return health
}
