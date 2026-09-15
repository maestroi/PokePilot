from pathlib import Path


def replace(path: str, old: str, new: str, count: int = 1) -> None:
    p = Path(path)
    text = p.read_text()
    found = text.count(old)
    if found < count:
        raise SystemExit(f"{path}: expected {count} match(es), found {found}: {old[:160]!r}")
    p.write_text(text.replace(old, new, count))


# Experiment concurrency belongs to each arm, not the request as a whole.
replace(
    "farm/experiment.go",
    '\tMaxFrames          int           `json:"max_frames,omitempty"`\n\tMaxParallelWorkers int           `json:"max_parallel_workers,omitempty"`\n}\n\n// ExperimentRunMeta',
    '\tMaxFrames          int           `json:"max_frames,omitempty"`\n}\n\n// ExperimentRunMeta',
)

# Show active workers for deployments without a model-host controller as well.
replace(
    "cmd/pokewall/model_experiments.go",
    '\tstatuses := map[string]modelHostStatus{}\n\tqueued := c.queuedByDeployment()\n\tfor _, d := range deployments {\n\t\tview := deploymentView{ModelDeployment: d, State: "ready", Queued: queued[d.ID]}',
    '\tstatuses := map[string]modelHostStatus{}\n\tqueued := c.queuedByDeployment()\n\tactive := c.activeByDeployment()\n\tfor _, d := range deployments {\n\t\tview := deploymentView{ModelDeployment: d, State: "ready", ActiveLeases: active[d.ID], Queued: queued[d.ID]}',
)
replace(
    "cmd/pokewall/model_experiments.go",
    '\t\t\t\tstatuses[d.ControlURL] = status\n\t\t\t\tview.Loaded, view.ActiveLeases, view.Error = status.DeploymentID, status.ActiveLeases, status.Error',
    '\t\t\t\tstatuses[d.ControlURL] = status\n\t\t\t\tview.Loaded, view.Error = status.DeploymentID, status.Error\n\t\t\t\tif status.ActiveLeases > view.ActiveLeases {\n\t\t\t\t\tview.ActiveLeases = status.ActiveLeases\n\t\t\t\t}',
)
replace(
    "cmd/pokewall/model_experiments.go",
    'func (c *modelExperimentController) queuedByDeployment() map[string]int {',
    '''func (c *modelExperimentController) activeByDeployment() map[string]int {
\tc.wall.mu.Lock()
\tactiveIDs := make([]string, 0)
\tfor runID, tile := range c.wall.tiles {
\t\tif tile != nil && !tile.Finished && (tile.Status == statusLeased || tile.Status == statusRunning) {
\t\t\tactiveIDs = append(activeIDs, runID)
\t\t}
\t}
\tc.wall.mu.Unlock()
\tout := map[string]int{}
\tfor _, runID := range activeIDs {
\t\tif meta, ok := c.runMeta(runID); ok {
\t\t\tout[meta.Deployment]++
\t\t}
\t}
\treturn out
}

func (c *modelExperimentController) queuedByDeployment() map[string]int {''',
)

# An experiment can request a stricter cap, never more parallelism than the deployment permits.
replace(
    "cmd/pokewall/model_experiments.go",
    '''\tfor _, id := range []string{request.ArmA.Deployment, request.ArmB.Deployment} {
\t\td, ok := c.registry.Deployment(id)
\t\tif !ok || !d.Enabled {
\t\t\twriteJSON(w, http.StatusBadRequest, map[string]string{"error": "deployment " + id + " is unavailable"})
\t\t\treturn
\t\t}
\t}''',
    '''\tfor _, arm := range []*farm.ExperimentArm{&request.ArmA, &request.ArmB} {
\t\td, ok := c.registry.Deployment(arm.Deployment)
\t\tif !ok || !d.Enabled {
\t\t\twriteJSON(w, http.StatusBadRequest, map[string]string{"error": "deployment " + arm.Deployment + " is unavailable"})
\t\t\treturn
\t\t}
\t\tlimit := d.ParallelLimit()
\t\tif arm.MaxParallelWorkers < 0 || arm.MaxParallelWorkers > limit {
\t\t\twriteJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("deployment %s allows at most %d parallel worker(s)", arm.Deployment, limit)})
\t\t\treturn
\t\t}
\t\tif arm.MaxParallelWorkers == 0 {
\t\t\tarm.MaxParallelWorkers = limit
\t\t}
\t}''',
)
replace(
    "cmd/pokewall/model_experiments.go",
    '''\tif parallel == 0 {
\t\tparallel = d.ParallelLimit()
\t}
\tcomparable := farm.ComparableRunConfig{''',
    '''\tif parallel == 0 {
\t\tparallel = d.ParallelLimit()
\t}
\tif parallel > d.ParallelLimit() {
\t\treturn runExperimentMeta{}, fmt.Errorf("deployment %q allows at most %d parallel worker(s)", deployment, d.ParallelLimit())
\t}
\tcomparable := farm.ComparableRunConfig{''',
)

# Operator UI limits the picker to what the deployment says is safe.
replace(
    "web/src/operator/PairedExperimentPanel.vue",
    '''function seconds(value: number | undefined): string {
  return `${Number(value || 0).toFixed(2)}s`
}

async function submit(): Promise<void> {''',
    '''function seconds(value: number | undefined): string {
  return `${Number(value || 0).toFixed(2)}s`
}

function workerLimit(deploymentID: string): number {
  return Math.max(1, Number(deployments.value.find((deployment) => deployment.id === deploymentID)?.max_parallel_workers || 1))
}

async function submit(): Promise<void> {''',
)
replace(
    "web/src/operator/PairedExperimentPanel.vue",
    '''  if (!form.arm_a || !form.arm_b || form.arm_a === form.arm_b) {
    error.value = 'Choose two different deployments for Arm A and Arm B.'
    return
  }
  submitting.value = true''',
    '''  if (!form.arm_a || !form.arm_b || form.arm_a === form.arm_b) {
    error.value = 'Choose two different deployments for Arm A and Arm B.'
    return
  }
  const armALimit = workerLimit(form.arm_a)
  const armBLimit = workerLimit(form.arm_b)
  if (Number(form.arm_a_workers) < 1 || Number(form.arm_a_workers) > armALimit || Number(form.arm_b_workers) < 1 || Number(form.arm_b_workers) > armBLimit) {
    error.value = `Worker count exceeds deployment capacity (A max ${armALimit}, B max ${armBLimit}).`
    return
  }
  submitting.value = true''',
)
replace(
    "web/src/operator/PairedExperimentPanel.vue",
    '<input v-model.number="form.arm_a_workers" type="number" min="1" max="16" :class="fieldClass" />',
    '<input v-model.number="form.arm_a_workers" type="number" min="1" :max="workerLimit(form.arm_a)" :class="fieldClass" />',
)
replace(
    "web/src/operator/PairedExperimentPanel.vue",
    '<input v-model.number="form.arm_b_workers" type="number" min="1" max="16" :class="fieldClass" />',
    '<input v-model.number="form.arm_b_workers" type="number" min="1" :max="workerLimit(form.arm_b)" :class="fieldClass" />',
)

# Extend regression coverage: wall telemetry works without a model host and overrides cannot exceed deployment capacity.
replace(
    "cmd/pokewall/model_experiments_test.go",
    '''\tif leaseA.Code != http.StatusOK || json.Unmarshal(leaseA.Body.Bytes(), &first) != nil || first.RunID != "a-1" {
\t\tt.Fatalf("first lease = %d %s", leaseA.Code, leaseA.Body.String())
\t}
\tleaseB := requestJSON(t, h, http.MethodPost, "/v1/lease", map[string]any{})''',
    '''\tif leaseA.Code != http.StatusOK || json.Unmarshal(leaseA.Body.Bytes(), &first) != nil || first.RunID != "a-1" {
\t\tt.Fatalf("first lease = %d %s", leaseA.Code, leaseA.Body.String())
\t}
\tmodels := requestJSON(t, h, http.MethodGet, "/v1/models", nil)
\tvar modelSnapshot struct {
\t\tDeployments []deploymentView `json:"deployments"`
\t}
\tif models.Code != http.StatusOK || json.Unmarshal(models.Body.Bytes(), &modelSnapshot) != nil {
\t\tt.Fatalf("models = %d %s", models.Code, models.Body.String())
\t}
\tfoundModelA := false
\tfor _, deployment := range modelSnapshot.Deployments {
\t\tif deployment.ID == "model-a" {
\t\t\tfoundModelA = deployment.ActiveLeases == 1 && deployment.Queued == 1
\t\t}
\t}
\tif !foundModelA {
\t\tt.Fatalf("model-a should report 1 active / 1 queued: %s", models.Body.String())
\t}
\tleaseB := requestJSON(t, h, http.MethodPost, "/v1/lease", map[string]any{})''',
)
replace(
    "cmd/pokewall/model_experiments_test.go",
    '{ID: "b", ModelID: "b", Compute: "b", Endpoint: "http://b/v1", APIModel: "b", Enabled: true},\n\t})\n\tt.Setenv("POKEPILOT_MODEL_REGISTRY", registry)\n\tw := NewWall("")\n\th := modelExperimentHTTPHandler(w, w.Handler())\n\tcreated := requestJSON(t, h, http.MethodPost, "/v1/experiments", farm.ExperimentRequest{\n\t\tArmA:  farm.ExperimentArm{Deployment: "a", MaxParallelWorkers: 1},\n\t\tArmB:  farm.ExperimentArm{Deployment: "b", MaxParallelWorkers: 2},',
    '{ID: "b", ModelID: "b", Compute: "b", Endpoint: "http://b/v1", APIModel: "b", Enabled: true, MaxParallelWorkers: 2},\n\t})\n\tt.Setenv("POKEPILOT_MODEL_REGISTRY", registry)\n\tw := NewWall("")\n\th := modelExperimentHTTPHandler(w, w.Handler())\n\tcreated := requestJSON(t, h, http.MethodPost, "/v1/experiments", farm.ExperimentRequest{\n\t\tArmA:  farm.ExperimentArm{Deployment: "a", MaxParallelWorkers: 1},\n\t\tArmB:  farm.ExperimentArm{Deployment: "b", MaxParallelWorkers: 2},',
)

p = Path("cmd/pokewall/model_experiments_test.go")
p.write_text(
    p.read_text()
    + r'''

func TestExperimentRejectsConcurrencyAboveDeploymentLimit(t *testing.T) {
	registry := writeModelRegistry(t, []farm.ModelDeployment{
		{ID: "a", ModelID: "a", Compute: "a", Endpoint: "http://a/v1", APIModel: "a", Enabled: true, MaxParallelWorkers: 1},
		{ID: "b", ModelID: "b", Compute: "b", Endpoint: "http://b/v1", APIModel: "b", Enabled: true, MaxParallelWorkers: 2},
	})
	t.Setenv("POKEPILOT_MODEL_REGISTRY", registry)
	w := NewWall("")
	h := modelExperimentHTTPHandler(w, w.Handler())
	created := requestJSON(t, h, http.MethodPost, "/v1/experiments", farm.ExperimentRequest{
		ArmA: farm.ExperimentArm{Deployment: "a", MaxParallelWorkers: 2},
		ArmB: farm.ExperimentArm{Deployment: "b", MaxParallelWorkers: 1},
		Seeds: []int64{1}, Goal: "Earn the Boulder Badge.",
	})
	if created.Code != http.StatusBadRequest {
		t.Fatalf("create = %d %s, want 400", created.Code, created.Body.String())
	}
}
'''
)
