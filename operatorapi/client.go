package operatorapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrNotConfigured = errors.New("operator integration is not configured")

type Client struct {
	WallBase         string
	ReplayBase       string
	AlertmanagerBase string
	HTTP             *http.Client
}

type Dashboard struct {
	Now         int64    `json:"now"`
	WallVersion string   `json:"wall_version,omitempty"`
	Runs        []Run    `json:"runs"`
	Workers     []Worker `json:"workers"`
	Total       int      `json:"total"`
}

type Worker struct {
	Addr     string `json:"addr,omitempty"`
	Version  string `json:"version,omitempty"`
	LastSeen int64  `json:"last_seen,omitempty"`
}

type Run struct {
	RunID           string         `json:"run_id"`
	Status          string         `json:"status"`
	Game            string         `json:"game,omitempty"`
	Planner         string         `json:"planner,omitempty"`
	Starter         string         `json:"starter,omitempty"`
	Goal            string         `json:"goal,omitempty"`
	PlayStyle       string         `json:"play_style,omitempty"`
	Seed            int64          `json:"seed"`
	QueuedAt        int64          `json:"queued_at,omitempty"`
	EndedAt         int64          `json:"ended_at,omitempty"`
	Frame           uint64         `json:"frame"`
	Map             uint8          `json:"map"`
	X               uint8          `json:"x"`
	Y               uint8          `json:"y"`
	MapsVisited     int            `json:"maps_visited,omitempty"`
	Trace           string         `json:"trace,omitempty"`
	Question        string         `json:"question,omitempty"`
	Decision        string         `json:"decision,omitempty"`
	StopSoFar       string         `json:"stop_so_far,omitempty"`
	Stats           *RunStats      `json:"stats,omitempty"`
	Player          *Player        `json:"player,omitempty"`
	GameState       map[string]any `json:"game_state,omitempty"`
	Attempts        int            `json:"attempts"`
	LossRecoveries  int            `json:"loss_recoveries,omitempty"`
	RecoveryBadges  int            `json:"recovery_badges,omitempty"`
	RecoveryEvents  int            `json:"recovery_events,omitempty"`
	RecoveryMaps    int            `json:"recovery_maps,omitempty"`
	Reason          string         `json:"reason,omitempty"`
	Detail          string         `json:"detail,omitempty"`
	ReplayAvailable bool           `json:"replay_available,omitempty"`
}

type RunStats struct {
	Round        int    `json:"round"`
	RoundsLeft   int    `json:"rounds_left"`
	GoalSummary  string `json:"goal_summary,omitempty"`
	GoalKind     string `json:"goal_kind,omitempty"`
	GoalID       string `json:"goal_id,omitempty"`
	GoalComplete bool   `json:"goal_complete,omitempty"`
	GoalCurrent  int    `json:"goal_current,omitempty"`
	GoalTarget   int    `json:"goal_target,omitempty"`
	Endpoint     string `json:"endpoint,omitempty"`
	Model        string `json:"model,omitempty"`
}

type Player struct {
	Money      uint32     `json:"money"`
	Badges     []string   `json:"badges,omitempty"`
	Party      []PartyMon `json:"party"`
	DexOwned   int        `json:"dex_owned,omitempty"`
	DexSeen    int        `json:"dex_seen,omitempty"`
	Milestones []string   `json:"milestones,omitempty"`
}

type PartyMon struct {
	Name   string `json:"name"`
	Level  uint8  `json:"level"`
	HP     uint16 `json:"hp"`
	MaxHP  uint16 `json:"max_hp"`
	Status string `json:"status,omitempty"`
}

type RunInspection struct {
	Run    Run        `json:"run"`
	Finish *RunFinish `json:"finish,omitempty"`
}

type RunFinish struct {
	Attempt int      `json:"attempt,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	Detail  string   `json:"detail,omitempty"`
	Trace   []string `json:"trace_tail,omitempty"`
}

type TriageIssue struct {
	IssueNumber int64  `json:"issue_number"`
	Status      string `json:"status"`
	Resolution  string `json:"resolution"`
}

type TriageGroup struct {
	Pattern     string       `json:"pattern"`
	Key         string       `json:"key"`
	Fingerprint string       `json:"fingerprint"`
	Count       int          `json:"count"`
	Example     string       `json:"example"`
	RunIDs      []string     `json:"run_ids"`
	Issue       *TriageIssue `json:"issue,omitempty"`
}

type Checkpoint struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Frame        uint64 `json:"frame,omitempty"`
	Round        int    `json:"round,omitempty"`
	Replayable   bool   `json:"replayable"`
	HasKnowledge bool   `json:"has_knowledge,omitempty"`
}

type ReplayHealth struct {
	Status         string `json:"status"`
	Encoder        string `json:"encoder,omitempty"`
	ActiveRenders  int64  `json:"active_renders,omitempty"`
	LiveSessions   int    `json:"live_sessions,omitempty"`
	ScratchFree    uint64 `json:"scratch_free_bytes,omitempty"`
	CapacityStatus string `json:"capacity_status,omitempty"`
}

type Alert struct {
	Status struct {
		State string `json:"state"`
	} `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`
	Fingerprint string            `json:"fingerprint"`
}

type MediaRenderJob struct {
	ID            string `json:"id"`
	RunID         string `json:"run_id"`
	Mode          string `json:"mode"`
	State         string `json:"state"`
	Stage         string `json:"stage,omitempty"`
	SegmentsTotal int    `json:"segments_total,omitempty"`
	SegmentsDone  int    `json:"segments_done,omitempty"`
	RetryCount    int    `json:"retry_count,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	FailureClass  string `json:"failure_class,omitempty"`
	ResultSize    int64  `json:"result_size,omitempty"`
	UpdatedAt     int64  `json:"updated_at_unix_ms,omitempty"`
	FinishedAt    int64  `json:"finished_at_unix_ms,omitempty"`
}

type MediaRenderJobList struct {
	Jobs   []MediaRenderJob `json:"jobs"`
	Total  int              `json:"total"`
	States map[string]int   `json:"states"`
}

type RestartResult struct {
	RunID      string
	Method     string
	Checkpoint string
}

func New(wallBase, replayBase, alertmanagerBase string) *Client {
	return &Client{
		WallBase:         strings.TrimRight(strings.TrimSpace(wallBase), "/"),
		ReplayBase:       strings.TrimRight(strings.TrimSpace(replayBase), "/"),
		AlertmanagerBase: strings.TrimRight(strings.TrimSpace(alertmanagerBase), "/"),
		HTTP:             &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) Dashboard(ctx context.Context, active bool, limit int) (Dashboard, error) {
	var out Dashboard
	q := url.Values{}
	if active {
		q.Set("active", "true")
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprint(limit))
	}
	path := "/v1/dashboard"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	err := c.wallJSON(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) Run(ctx context.Context, id string) (RunInspection, error) {
	var out RunInspection
	err := c.wallJSON(ctx, http.MethodGet, runPath(id), nil, &out)
	return out, err
}

func (c *Client) Triage(ctx context.Context) ([]TriageGroup, error) {
	var out []TriageGroup
	err := c.wallJSON(ctx, http.MethodGet, "/v1/triage", nil, &out)
	return out, err
}

// runPath and triagePath are the single owner of the operator endpoint shapes
// that the Telegram bot and the admin/MCP control plane both call. Keeping
// them here is what makes the control operations reusable rather than
// re-spelled per surface.
func runPath(id string) string {
	return "/v1/runs/" + url.PathEscape(strings.TrimSpace(id))
}

func triagePath(key string) string {
	return "/v1/triage/" + url.PathEscape(strings.TrimSpace(key))
}

// FlagStuck asks the wall to stop an active run as stuck so the failure
// reaches triage like a stagnation-watchdog stop.
func (c *Client) FlagStuck(ctx context.Context, id, note string) error {
	return c.wallJSON(ctx, http.MethodPost, runPath(id)+"/flag-stuck", map[string]string{"note": note}, &map[string]any{})
}

// CancelRun cooperatively cancels one run and returns the wall's decoded
// response. Callers that surface the upstream payload (the admin control
// plane) and callers that only need the outcome (the Telegram bot) share this
// one implementation of the endpoint, request shape and error decoding.
func (c *Client) CancelRun(ctx context.Context, id string) (map[string]any, error) {
	out := map[string]any{}
	if err := c.wallJSON(ctx, http.MethodPost, runPath(id)+"/cancel", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// InvestigateFailure queues the existing triage investigation for one failure
// key and returns the wall's decoded response.
func (c *Client) InvestigateFailure(ctx context.Context, key string) (map[string]any, error) {
	out := map[string]any{}
	if err := c.wallJSON(ctx, http.MethodPost, triagePath(key)+"/investigate", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Investigate(ctx context.Context, key string) error {
	_, err := c.InvestigateFailure(ctx, key)
	return err
}

func (c *Client) Stop(ctx context.Context, id string) error {
	_, err := c.CancelRun(ctx, id)
	return err
}

func (c *Client) QueueReplay(ctx context.Context, id string) error {
	base := c.ReplayBase
	if base == "" {
		return ErrNotConfigured
	}
	var out map[string]any
	return c.baseJSON(ctx, base, http.MethodPost, runPath(id)+"/replay/render", nil, &out)
}

func (c *Client) ReplayHealth(ctx context.Context) (ReplayHealth, error) {
	var out ReplayHealth
	if c.ReplayBase == "" {
		return out, ErrNotConfigured
	}
	err := c.baseJSON(ctx, c.ReplayBase, http.MethodGet, "/healthz", nil, &out)
	return out, err
}

func (c *Client) Frame(ctx context.Context, id string) ([]byte, string, error) {
	if c.WallBase == "" {
		return nil, "", ErrNotConfigured
	}
	path := "/frame?run=" + url.QueryEscape(strings.TrimSpace(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.WallBase+path, nil)
	if err != nil {
		return nil, "", err
	}
	res, err := c.http().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, "", decodeHTTPError(res)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 {
		return nil, "", errors.New("empty frame")
	}
	return data, res.Header.Get("Content-Type"), nil
}

func (c *Client) Alerts(ctx context.Context) ([]Alert, error) {
	if c.AlertmanagerBase == "" {
		return nil, ErrNotConfigured
	}
	var out []Alert
	err := c.baseJSON(ctx, c.AlertmanagerBase, http.MethodGet, "/api/v2/alerts", nil, &out)
	return out, err
}

// ResolvedAlerts returns the alerts Alertmanager still retains but that are no
// longer firing. Alertmanager's default /api/v2/alerts query reports active
// alerts only, so a recent recovery is invisible without asking explicitly.
func (c *Client) ResolvedAlerts(ctx context.Context) ([]Alert, error) {
	if c.AlertmanagerBase == "" {
		return nil, ErrNotConfigured
	}
	var out []Alert
	err := c.baseJSON(ctx, c.AlertmanagerBase, http.MethodGet,
		"/api/v2/alerts?active=false&silenced=false&inhibited=false&unprocessed=false", nil, &out)
	return out, err
}

func (c *Client) MediaRenderJobs(ctx context.Context, limit int) (MediaRenderJobList, error) {
	var out MediaRenderJobList
	path := "/v1/media/render-jobs"
	if limit > 0 {
		path += "?limit=" + strconv.Itoa(limit)
	}
	err := c.wallJSON(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) Restart(ctx context.Context, id string) (RestartResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return RestartResult{}, errors.New("run id is required")
	}

	checkpoints, _ := c.checkpoints(ctx, id)
	for _, cp := range checkpoints {
		if !cp.Replayable {
			continue
		}
		var repro struct {
			RunID string `json:"run_id"`
		}
		body := map[string]any{"checkpoint": cp.Name}
		if err := c.wallJSON(ctx, http.MethodPost, "/v1/runs/"+url.PathEscape(id)+"/repro", body, &repro); err == nil && repro.RunID != "" {
			if stopErr := c.Stop(ctx, id); stopErr != nil {
				return RestartResult{}, fmt.Errorf("replacement %s queued but original stop failed: %w", repro.RunID, stopErr)
			}
			return RestartResult{RunID: repro.RunID, Method: "checkpoint", Checkpoint: cp.Name}, nil
		}
	}

	var cloned struct {
		RunID string `json:"run_id"`
	}
	if err := c.wallJSON(ctx, http.MethodPost, "/v1/runs/"+url.PathEscape(id)+"/clone", nil, &cloned); err != nil {
		return RestartResult{}, err
	}
	if cloned.RunID == "" {
		return RestartResult{}, errors.New("restart clone returned no run id")
	}
	if stopErr := c.Stop(ctx, id); stopErr != nil {
		return RestartResult{}, fmt.Errorf("replacement %s queued but original stop failed: %w", cloned.RunID, stopErr)
	}
	return RestartResult{RunID: cloned.RunID, Method: "fresh-clone"}, nil
}

func (c *Client) FindTriageForRun(ctx context.Context, runID string) (*TriageGroup, error) {
	groups, err := c.Triage(ctx)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		for _, id := range groups[i].RunIDs {
			if id == runID {
				return &groups[i], nil
			}
		}
	}
	return nil, nil
}

func (c *Client) checkPointsPath(id string) string {
	return "/v1/runs/" + url.PathEscape(id) + "/checkpoints"
}

func (c *Client) checkpoints(ctx context.Context, id string) ([]Checkpoint, error) {
	var out struct {
		Checkpoints []Checkpoint `json:"checkpoints"`
	}
	if err := c.wallJSON(ctx, http.MethodGet, c.checkPointsPath(id), nil, &out); err != nil {
		return nil, err
	}
	// The wall currently returns newest checkpoints first. Keep that contract
	// when present, but make the choice stable if an older server did not.
	sort.SliceStable(out.Checkpoints, func(i, j int) bool {
		return out.Checkpoints[i].Frame > out.Checkpoints[j].Frame
	})
	return out.Checkpoints, nil
}

func (c *Client) wallJSON(ctx context.Context, method, path string, body any, out any) error {
	if c.WallBase == "" {
		return ErrNotConfigured
	}
	return c.baseJSON(ctx, c.WallBase, method, path, body, out)
}

func (c *Client) baseJSON(ctx context.Context, base, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return decodeHTTPError(res)
	}
	if out == nil || res.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func decodeHTTPError(res *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(res.Body, 32<<10))
	var payload struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &payload) == nil && strings.TrimSpace(payload.Error) != "" {
		return fmt.Errorf("operator API %s: %s", res.Status, strings.TrimSpace(payload.Error))
	}
	if text := strings.TrimSpace(string(data)); text != "" {
		return fmt.Errorf("operator API %s: %s", res.Status, text)
	}
	return fmt.Errorf("operator API %s", res.Status)
}
