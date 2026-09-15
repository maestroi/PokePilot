from pathlib import Path
import json


def replace(path: str, old: str, new: str, count: int = 1) -> None:
    p = Path(path)
    text = p.read_text()
    found = text.count(old)
    if found < count:
        raise SystemExit(f"{path}: expected at least {count} occurrence(s), found {found}: {old[:160]!r}")
    p.write_text(text.replace(old, new, count))


# Deployment registry: safe default is one active worker per model deployment.
replace(
    "farm/model_registry.go",
    '\tEngineVersion string `json:"engine_version,omitempty"`\n\tEngineConfig  string `json:"engine_config,omitempty"`\n\t// LegacyProfile is only the compatibility adapter used by existing',
    '\tEngineVersion      string `json:"engine_version,omitempty"`\n\tEngineConfig       string `json:"engine_config,omitempty"`\n\tMaxParallelWorkers int    `json:"max_parallel_workers,omitempty"`\n\t// LegacyProfile is only the compatibility adapter used by existing',
)
replace(
    "farm/model_registry.go",
    '\tEngineVersion string `json:"engine_version,omitempty"`\n\tEngineConfig  string `json:"engine_config,omitempty"`\n}\n\nconst postgresRegistryEnvPrefix',
    '\tEngineVersion      string `json:"engine_version,omitempty"`\n\tEngineConfig       string `json:"engine_config,omitempty"`\n\tMaxParallelWorkers int    `json:"max_parallel_workers,omitempty"`\n}\n\nconst postgresRegistryEnvPrefix',
)
replace(
    "farm/model_registry.go",
    '\t\tif strings.TrimSpace(d.APIModel) == "" {\n\t\t\treturn fmt.Errorf("model registry: deployment %q has empty api_model", id)\n\t\t}\n\t\tswitch p := strings.TrimSpace(d.LegacyProfile); p {',
    '\t\tif strings.TrimSpace(d.APIModel) == "" {\n\t\t\treturn fmt.Errorf("model registry: deployment %q has empty api_model", id)\n\t\t}\n\t\tif d.MaxParallelWorkers < 0 {\n\t\t\treturn fmt.Errorf("model registry: deployment %q has invalid max_parallel_workers %d", id, d.MaxParallelWorkers)\n\t\t}\n\t\tswitch p := strings.TrimSpace(d.LegacyProfile); p {',
)
replace(
    "farm/model_registry.go",
    '\t\tControlURL: d.ControlURL, TokenEnv: d.TokenEnv, Engine: d.Engine,\n\t\tEngineVersion: d.EngineVersion, EngineConfig: d.EngineConfig,\n\t}\n}\n\n// CompatibilityProfile',
    '\t\tControlURL: d.ControlURL, TokenEnv: d.TokenEnv, Engine: d.Engine,\n\t\tEngineVersion: d.EngineVersion, EngineConfig: d.EngineConfig,\n\t\tMaxParallelWorkers: d.ParallelLimit(),\n\t}\n}\n\n// ParallelLimit returns the deployment worker cap. Zero is intentionally\n// backwards-compatible and means one active worker at a time.\nfunc (d ModelDeployment) ParallelLimit() int {\n\tif d.MaxParallelWorkers <= 0 {\n\t\treturn 1\n\t}\n\treturn d.MaxParallelWorkers\n}\n\n// CompatibilityProfile',
)

# Postgres schema/load, including migration of existing clean/new databases.
replace(
    "farm/model_registry_postgres.go",
    "\tengine_version TEXT NOT NULL DEFAULT '',\n\tengine_config TEXT NOT NULL DEFAULT '',\n\tlegacy_profile TEXT NOT NULL DEFAULT '',\n\tcreated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),",
    "\tengine_version TEXT NOT NULL DEFAULT '',\n\tengine_config TEXT NOT NULL DEFAULT '',\n\tmax_parallel_workers INTEGER NOT NULL DEFAULT 1,\n\tlegacy_profile TEXT NOT NULL DEFAULT '',\n\tcreated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),",
)
replace(
    "farm/model_registry_postgres.go",
    ");\nCREATE INDEX IF NOT EXISTS model_deployments_enabled_compute_idx",
    ");\nALTER TABLE model_deployments\n\tADD COLUMN IF NOT EXISTS max_parallel_workers INTEGER NOT NULL DEFAULT 1;\nCREATE INDEX IF NOT EXISTS model_deployments_enabled_compute_idx",
)
replace(
    "farm/model_registry_postgres.go",
    "       api_model, enabled, control_url, token_env, engine, engine_version,\n       engine_config, legacy_profile\nFROM model_deployments",
    "       api_model, enabled, control_url, token_env, engine, engine_version,\n       engine_config, max_parallel_workers, legacy_profile\nFROM model_deployments",
)
replace(
    "farm/model_registry_postgres.go",
    "\t\t\t&d.ControlURL, &d.TokenEnv, &d.Engine, &d.EngineVersion,\n\t\t\t&d.EngineConfig, &d.LegacyProfile,",
    "\t\t\t&d.ControlURL, &d.TokenEnv, &d.Engine, &d.EngineVersion,\n\t\t\t&d.EngineConfig, &d.MaxParallelWorkers, &d.LegacyProfile,",
)

# Experiment arms can select a stricter/equal worker cap. Concurrency is part of comparability.
replace(
    "farm/experiment.go",
    'type ExperimentArm struct {\n\tName       string `json:"name"`\n\tDeployment string `json:"deployment"`\n}',
    'type ExperimentArm struct {\n\tName               string `json:"name"`\n\tDeployment         string `json:"deployment"`\n\tMaxParallelWorkers int    `json:"max_parallel_workers,omitempty"`\n}',
)
replace(
    "farm/experiment.go",
    '\tReasoningEffort string `json:"reasoning_effort,omitempty"`\n\tFPS             int    `json:"fps,omitempty"`\n\tMaxRounds       int    `json:"max_rounds,omitempty"`\n\tMaxFrames       int    `json:"max_frames,omitempty"`\n}\n\n// ExperimentRunMeta',
    '\tReasoningEffort    string `json:"reasoning_effort,omitempty"`\n\tFPS                int    `json:"fps,omitempty"`\n\tMaxRounds          int    `json:"max_rounds,omitempty"`\n\tMaxFrames          int    `json:"max_frames,omitempty"`\n\tMaxParallelWorkers int    `json:"max_parallel_workers,omitempty"`\n}\n\n// ExperimentRunMeta',
)
replace(
    "farm/experiment.go",
    '\tReasoningEffort string `json:"reasoning_effort,omitempty"`\n\tFPS             int    `json:"fps,omitempty"`\n\tMaxRounds       int    `json:"max_rounds,omitempty"`\n\tMaxFrames       int    `json:"max_frames,omitempty"`\n}\n',
    '\tReasoningEffort    string `json:"reasoning_effort,omitempty"`\n\tFPS                int    `json:"fps,omitempty"`\n\tMaxRounds          int    `json:"max_rounds,omitempty"`\n\tMaxFrames          int    `json:"max_frames,omitempty"`\n\tMaxParallelWorkers int    `json:"max_parallel_workers,omitempty"`\n}\n',
)

# Model host: hard boundary for switchable hosts so a wall bug cannot oversubscribe a GPU.
replace(
    "cmd/pokemodelhost/main.go",
    '\tVersion      string            `json:"engine_version,omitempty"`\n\tCommand      string            `json:"command,omitempty"`',
    '\tVersion            string            `json:"engine_version,omitempty"`\n\tMaxParallelWorkers int               `json:"max_parallel_workers,omitempty"`\n\tCommand            string            `json:"command,omitempty"`',
)
replace(
    "cmd/pokemodelhost/main.go",
    '\tActiveLeases int        `json:"active_leases"`\n\tLeaseRunIDs  []string   `json:"lease_run_ids,omitempty"`',
    '\tActiveLeases       int        `json:"active_leases"`\n\tMaxParallelWorkers int        `json:"max_parallel_workers,omitempty"`\n\tLeaseRunIDs        []string   `json:"lease_run_ids,omitempty"`',
)
replace(
    "cmd/pokemodelhost/main.go",
    '\t\tif _, exists := models[id]; exists {\n\t\t\treturn nil, fmt.Errorf("duplicate deployment_id %q", id)\n\t\t}\n\t\tmodels[id] = model',
    '\t\tif _, exists := models[id]; exists {\n\t\t\treturn nil, fmt.Errorf("duplicate deployment_id %q", id)\n\t\t}\n\t\tif model.MaxParallelWorkers < 0 {\n\t\t\treturn nil, fmt.Errorf("deployment %q has invalid max_parallel_workers %d", id, model.MaxParallelWorkers)\n\t\t}\n\t\tmodels[id] = model',
)
replace(
    "cmd/pokemodelhost/main.go",
    'type leaseRequest struct {\n\tRunID        string `json:"run_id"`\n\tDeploymentID string `json:"deployment_id"`\n}',
    'type leaseRequest struct {\n\tRunID              string `json:"run_id"`\n\tDeploymentID       string `json:"deployment_id"`\n\tMaxParallelWorkers int    `json:"max_parallel_workers,omitempty"`\n}',
)
replace(
    "cmd/pokemodelhost/main.go",
    '\tstatus, code, err := s.requestLoad(strings.TrimSpace(in.DeploymentID), "")',
    '\tstatus, code, err := s.requestLoad(strings.TrimSpace(in.DeploymentID), "", 0)',
)
replace(
    "cmd/pokemodelhost/main.go",
    '\tstatus, code, err := s.requestLoad(in.DeploymentID, in.RunID)',
    '\tstatus, code, err := s.requestLoad(in.DeploymentID, in.RunID, in.MaxParallelWorkers)',
)
replace(
    "cmd/pokemodelhost/main.go",
    'func (s *lifecycleService) requestLoad(deploymentID, leaseRunID string) (hostStatus, int, error) {\n\ts.mu.Lock()\n\tdefer s.mu.Unlock()\n\tmodel, ok := s.models[deploymentID]\n\tif !ok {\n\t\tstatus := s.statusLocked()\n\t\treturn status, http.StatusNotFound, fmt.Errorf("deployment %q is not approved on this host", deploymentID)\n\t}\n\tif current, ok := s.leases[leaseRunID]; leaseRunID != "" && ok && current != deploymentID {',
    'func (s *lifecycleService) requestLoad(deploymentID, leaseRunID string, requestedLimit int) (hostStatus, int, error) {\n\ts.mu.Lock()\n\tdefer s.mu.Unlock()\n\tmodel, ok := s.models[deploymentID]\n\tif !ok {\n\t\tstatus := s.statusLocked()\n\t\treturn status, http.StatusNotFound, fmt.Errorf("deployment %q is not approved on this host", deploymentID)\n\t}\n\thardLimit := model.MaxParallelWorkers\n\tif hardLimit <= 0 {\n\t\thardLimit = 1\n\t}\n\tlimit := requestedLimit\n\tif limit <= 0 {\n\t\tlimit = hardLimit\n\t}\n\tif limit > hardLimit {\n\t\tstatus := s.statusLocked()\n\t\treturn status, http.StatusBadRequest, fmt.Errorf("deployment %q allows at most %d parallel worker(s)", deploymentID, hardLimit)\n\t}\n\tif current, ok := s.leases[leaseRunID]; leaseRunID != "" && ok && current != deploymentID {',
)
replace(
    "cmd/pokemodelhost/main.go",
    '\tif leaseRunID != "" {\n\t\ts.leases[leaseRunID] = deploymentID\n\t}\n\tif s.loaded == deploymentID',
    '\tif leaseRunID != "" {\n\t\tif _, already := s.leases[leaseRunID]; !already && len(s.leases) >= limit {\n\t\t\tstatus := s.statusLocked()\n\t\t\treturn status, http.StatusTooManyRequests, fmt.Errorf("deployment %q is at its %d-worker concurrency limit", deploymentID, limit)\n\t\t}\n\t\ts.leases[leaseRunID] = deploymentID\n\t}\n\tif s.loaded == deploymentID',
)
replace(
    "cmd/pokemodelhost/main.go",
    '\tif model, ok := s.models[s.loaded]; ok {\n\t\tstatus.DeploymentID, status.ModelID, status.Endpoint, status.APIModel = model.DeploymentID, model.ModelID, model.Endpoint, model.APIModel\n\t}',
    '\tif model, ok := s.models[s.loaded]; ok {\n\t\tstatus.DeploymentID, status.ModelID, status.Endpoint, status.APIModel = model.DeploymentID, model.ModelID, model.Endpoint, model.APIModel\n\t\tstatus.MaxParallelWorkers = model.MaxParallelWorkers\n\t\tif status.MaxParallelWorkers <= 0 {\n\t\t\tstatus.MaxParallelWorkers = 1\n\t\t}\n\t}',
)

# Wall/model experiment controller: skip saturated deployments instead of blocking the whole queue.
replace(
    "cmd/pokewall/model_experiments.go",
    '\tWildEncounters string                   `json:"wild_encounters,omitempty"`\n}',
    '\tWildEncounters    string `json:"wild_encounters,omitempty"`\n\tMaxParallelWorkers int    `json:"max_parallel_workers,omitempty"`\n}',
)
replace(
    "cmd/pokewall/model_experiments.go",
    '\tActiveLeases int    `json:"active_leases,omitempty"`\n\tError        string `json:"error,omitempty"`',
    '\tActiveLeases int    `json:"active_leases,omitempty"`\n\tQueued       int    `json:"queued,omitempty"`\n\tError        string `json:"error,omitempty"`',
)
replace(
    "cmd/pokewall/model_experiments.go",
    '\tActiveLeases int    `json:"active_leases,omitempty"`\n\tError        string `json:"error,omitempty"`\n}\n\n// modelExperimentHTTPHandler',
    '\tActiveLeases       int    `json:"active_leases,omitempty"`\n\tMaxParallelWorkers int    `json:"max_parallel_workers,omitempty"`\n\tError              string `json:"error,omitempty"`\n}\n\n// modelExperimentHTTPHandler',
)
replace(
    "cmd/pokewall/model_experiments.go",
    '\tviews := make([]deploymentView, 0, len(deployments))\n\tstatuses := map[string]modelHostStatus{}\n\tfor _, d := range deployments {\n\t\tview := deploymentView{ModelDeployment: d, State: "ready"}',
    '\tviews := make([]deploymentView, 0, len(deployments))\n\tstatuses := map[string]modelHostStatus{}\n\tqueued := c.queuedByDeployment()\n\tfor _, d := range deployments {\n\t\tview := deploymentView{ModelDeployment: d, State: "ready", Queued: queued[d.ID]}',
)
replace(
    "cmd/pokewall/model_experiments.go",
    '\t\tstatus, code, err := c.hostAction(meta.Inference, "/v1/leases/acquire", map[string]string{"run_id": spec.RunID, "deployment_id": meta.Deployment})\n\t\tif err != nil || code >= 300 || status.State != "ready" || status.DeploymentID != meta.Deployment {\n\t\t\twriteJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "deployment lost readiness while leasing", "deployment": meta.Deployment, "status": status})\n\t\t\treturn\n\t\t}',
    '\t\tstatus, code, err := c.hostAction(meta.Inference, "/v1/leases/acquire", map[string]any{"run_id": spec.RunID, "deployment_id": meta.Deployment, "max_parallel_workers": meta.MaxParallelWorkers})\n\t\tif err != nil || code >= 300 || status.State != "ready" || status.DeploymentID != meta.Deployment {\n\t\t\tc.requeueLease(spec.RunID)\n\t\t\twriteJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "deployment lost readiness while leasing", "deployment": meta.Deployment, "status": status})\n\t\t\treturn\n\t\t}',
)
replace(
    "cmd/pokewall/model_experiments.go",
    '\tfor _, runID := range queue {\n\t\tmeta, ok := c.runMeta(runID)\n\t\tif !ok || meta.Inference.ControlURL == "" {\n\t\t\tc.moveQueueFront(runID)\n\t\t\treturn true\n\t\t}\n\t\tstatus, err := c.hostStatusIdentity(meta.Inference)',
    '\tfor _, runID := range queue {\n\t\tmeta, ok := c.runMeta(runID)\n\t\tif !ok {\n\t\t\tc.moveQueueFront(runID)\n\t\t\treturn true\n\t\t}\n\t\tif c.deploymentAtCapacity(meta) {\n\t\t\tcontinue\n\t\t}\n\t\tif meta.Inference.ControlURL == "" {\n\t\t\tc.moveQueueFront(runID)\n\t\t\treturn true\n\t\t}\n\t\tstatus, err := c.hostStatusIdentity(meta.Inference)',
)
replace(
    "cmd/pokewall/model_experiments.go",
    '\treturn false\n}\n\nfunc (c *modelExperimentController) moveQueueFront(runID string) {',
    '''\treturn false
}

func (c *modelExperimentController) deploymentAtCapacity(meta runExperimentMeta) bool {
\tlimit := meta.MaxParallelWorkers
\tif limit <= 0 {
\t\tlimit = 1
\t}
\tc.wall.mu.Lock()
\tactiveIDs := make([]string, 0)
\tfor runID, tile := range c.wall.tiles {
\t\tif tile != nil && !tile.Finished && (tile.Status == statusLeased || tile.Status == statusRunning) {
\t\t\tactiveIDs = append(activeIDs, runID)
\t\t}
\t}
\tc.wall.mu.Unlock()
\tactive := 0
\tfor _, runID := range activeIDs {
\t\tif other, ok := c.runMeta(runID); ok && other.Deployment == meta.Deployment {
\t\t\tactive++
\t\t}
\t}
\treturn active >= limit
}

func (c *modelExperimentController) queuedByDeployment() map[string]int {
\tc.wall.mu.Lock()
\tqueue := append([]string(nil), c.wall.queue...)
\tc.wall.mu.Unlock()
\tout := map[string]int{}
\tfor _, runID := range queue {
\t\tif meta, ok := c.runMeta(runID); ok {
\t\t\tout[meta.Deployment]++
\t\t}
\t}
\treturn out
}

func (c *modelExperimentController) requeueLease(runID string) {
\tc.wall.mu.Lock()
\ttile := c.wall.tiles[runID]
\tif tile != nil && !tile.Finished && tile.Status == statusLeased {
\t\talreadyQueued := false
\t\tfor _, queuedID := range c.wall.queue {
\t\t\tif queuedID == runID {
\t\t\t\talreadyQueued = true
\t\t\t\tbreak
\t\t\t}
\t\t}
\t\tif !alreadyQueued {
\t\t\tc.wall.queue = append([]string{runID}, c.wall.queue...)
\t\t}
\t\ttile.Status = statusQueued
\t\ttile.lastUpdate = time.Now()
\t}
\tc.wall.mu.Unlock()
\tc.wall.saveState()
}

func (c *modelExperimentController) moveQueueFront(runID string) {''',
)
replace(
    "cmd/pokewall/model_experiments.go",
    '\t\t\t\trun["comparable_hash"] = meta.ComparableHash\n\t\t\t}',
    '\t\t\t\trun["comparable_hash"] = meta.ComparableHash\n\t\t\t\trun["max_parallel_workers"] = meta.MaxParallelWorkers\n\t\t\t}',
)
replace(
    "cmd/pokewall/model_experiments.go",
    '\t\t\t\t"llm_deployment": arm.cfg.Deployment, "reasoning_effort": request.ReasoningEffort,\n\t\t\t\t"fps": request.FPS, "max_rounds": request.MaxRounds, "max_frames": request.MaxFrames,',
    '\t\t\t\t"llm_deployment": arm.cfg.Deployment, "reasoning_effort": request.ReasoningEffort, "max_parallel_workers": arm.cfg.MaxParallelWorkers,\n\t\t\t\t"fps": request.FPS, "max_rounds": request.MaxRounds, "max_frames": request.MaxFrames,',
)
replace(
    "cmd/pokewall/model_experiments.go",
    '\tif strings.TrimSpace(runID) == "" {\n\t\treturn runExperimentMeta{}, fmt.Errorf("run_id is required")\n\t}\n\tcomparable := farm.ComparableRunConfig{',
    '\tif strings.TrimSpace(runID) == "" {\n\t\treturn runExperimentMeta{}, fmt.Errorf("run_id is required")\n\t}\n\tparallel := intNumber(raw["max_parallel_workers"])\n\tif parallel < 0 {\n\t\treturn runExperimentMeta{}, fmt.Errorf("max_parallel_workers may not be negative")\n\t}\n\tif parallel == 0 {\n\t\tparallel = d.ParallelLimit()\n\t}\n\tcomparable := farm.ComparableRunConfig{',
)
replace(
    "cmd/pokewall/model_experiments.go",
    '\t\tFPS: intNumber(raw["fps"]), MaxRounds: intNumber(raw["max_rounds"]), MaxFrames: intNumber(raw["max_frames"]),\n\t}\n\tblob, _ := json.Marshal(comparable)',
    '\t\tFPS: intNumber(raw["fps"]), MaxRounds: intNumber(raw["max_rounds"]), MaxFrames: intNumber(raw["max_frames"]), MaxParallelWorkers: parallel,\n\t}\n\tblob, _ := json.Marshal(comparable)',
)
replace(
    "cmd/pokewall/model_experiments.go",
    '\treturn runExperimentMeta{RunID: runID, Deployment: deployment, Inference: d.Identity(), ExperimentID: experimentID, ExperimentArm: arm, ExperimentCase: caseID, Comparable: comparable, ComparableHash: hex.EncodeToString(hash[:]), PlayStyle: comparable.PlayStyle, RiskTolerance: comparable.RiskTolerance, WildEncounters: comparable.WildEncounters}, nil',
    '\treturn runExperimentMeta{RunID: runID, Deployment: deployment, Inference: d.Identity(), ExperimentID: experimentID, ExperimentArm: arm, ExperimentCase: caseID, Comparable: comparable, ComparableHash: hex.EncodeToString(hash[:]), PlayStyle: comparable.PlayStyle, RiskTolerance: comparable.RiskTolerance, WildEncounters: comparable.WildEncounters, MaxParallelWorkers: parallel}, nil',
)

# Web types + operator visibility + paired experiment controls.
replace(
    "web/src/shared/api/types.ts",
    "  engine_config?: string\n  legacy_profile?: string\n  state: DeploymentState",
    "  engine_config?: string\n  max_parallel_workers?: number\n  legacy_profile?: string\n  state: DeploymentState",
)
replace(
    "web/src/shared/api/types.ts",
    "  active_leases?: number\n  error?: string",
    "  active_leases?: number\n  queued?: number\n  error?: string",
)
replace(
    "web/src/shared/api/types.ts",
    "export interface ExperimentArm {\n  name: string\n  deployment: string\n}",
    "export interface ExperimentArm {\n  name: string\n  deployment: string\n  max_parallel_workers?: number\n}",
)
replace(
    "web/src/shared/api/types.ts",
    "  p95_strategic_call_seconds?: number\n  prompt_tokens?: number",
    "  p95_strategic_call_seconds?: number\n  avg_prefill_tps?: number\n  avg_decode_tps?: number\n  prompt_tokens?: number",
)
replace(
    "web/src/operator/LLMDeploymentsPanel.vue",
    '''function loadedNote(deployment: (typeof deployments.value)[number]): string {
  if (deployment.loaded_deployment && deployment.loaded_deployment !== deployment.id) {
    return `currently loaded: ${deployment.loaded_deployment}`
  }
  if (deployment.active_leases) {
    const noun = deployment.active_leases === 1 ? 'lease' : 'leases'
    return `${deployment.active_leases} active ${noun}`
  }
  return ''
}''',
    '''function loadedNote(deployment: (typeof deployments.value)[number]): string {
  if (deployment.loaded_deployment && deployment.loaded_deployment !== deployment.id) {
    return `currently loaded: ${deployment.loaded_deployment}`
  }
  const active = Number(deployment.active_leases || 0)
  const limit = Number(deployment.max_parallel_workers || 1)
  const queued = Number(deployment.queued || 0)
  const queueNote = queued ? ` · ${queued} queued` : ''
  return `${active}/${limit} workers${queueNote}`
}''',
)
replace(
    "web/src/operator/LLMDeploymentsPanel.vue",
    '<p class="mt-3 text-[11px] text-slate-600">Busy hosts keep incompatible runs queued. Loading finishes before a worker lease is handed out.</p>',
    '<p class="mt-3 text-[11px] text-slate-600">Each deployment queues above its worker cap instead of oversubscribing inference. Free capacity on other models keeps leasing independently.</p>',
)
replace(
    "web/src/operator/PairedExperimentPanel.vue",
    "  arm_a: '',\n  arm_b: '',\n  goal:",
    "  arm_a: '',\n  arm_b: '',\n  arm_a_workers: 1,\n  arm_b_workers: 1,\n  goal:",
)
replace(
    "web/src/operator/PairedExperimentPanel.vue",
    "      arm_a: { name: 'A', deployment: form.arm_a },\n      arm_b: { name: 'B', deployment: form.arm_b },",
    "      arm_a: { name: 'A', deployment: form.arm_a, max_parallel_workers: Number(form.arm_a_workers || 1) },\n      arm_b: { name: 'B', deployment: form.arm_b, max_parallel_workers: Number(form.arm_b_workers || 1) },",
)
replace(
    "web/src/operator/PairedExperimentPanel.vue",
    "    { label: 'Avg / p50 / p95', a: `${seconds(a.avg_strategic_call_seconds)} / ${seconds(a.p50_strategic_call_seconds)} / ${seconds(a.p95_strategic_call_seconds)}`, b: `${seconds(b.avg_strategic_call_seconds)} / ${seconds(b.p50_strategic_call_seconds)} / ${seconds(b.p95_strategic_call_seconds)}` },\n    { label: 'Prompt / completion',",
    "    { label: 'Avg / p50 / p95', a: `${seconds(a.avg_strategic_call_seconds)} / ${seconds(a.p50_strategic_call_seconds)} / ${seconds(a.p95_strategic_call_seconds)}`, b: `${seconds(b.avg_strategic_call_seconds)} / ${seconds(b.p50_strategic_call_seconds)} / ${seconds(b.p95_strategic_call_seconds)}` },\n    { label: 'Prefill / decode TPS', a: `${Number(a.avg_prefill_tps || 0).toFixed(1)} / ${Number(a.avg_decode_tps || 0).toFixed(1)}`, b: `${Number(b.avg_prefill_tps || 0).toFixed(1)} / ${Number(b.avg_decode_tps || 0).toFixed(1)}` },\n    { label: 'Prompt / completion',",
)
replace(
    "web/src/operator/PairedExperimentPanel.vue",
    '''      <label class="block sm:col-span-2 xl:col-span-3">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Goal</span>''',
    '''      <label class="block">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Arm A workers</span>
        <input v-model.number="form.arm_a_workers" type="number" min="1" max="16" :class="fieldClass" />
        <span class="mt-1 block text-[10px] text-slate-600">1 keeps this arm at maximum per-run model speed.</span>
      </label>
      <label class="block">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Arm B workers</span>
        <input v-model.number="form.arm_b_workers" type="number" min="1" max="16" :class="fieldClass" />
        <span class="mt-1 block text-[10px] text-slate-600">Runs above the cap remain queued.</span>
      </label>
      <div class="hidden xl:block"></div>
      <label class="block sm:col-span-2 xl:col-span-3">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Goal</span>''',
)
replace(
    "web/src/operator/PairedExperimentPanel.vue",
    '<p v-else class="text-[11px] text-slate-600">Each seed queues one Arm A run and one Arm B run with the same gameplay spec.</p>',
    '<p v-else class="text-[11px] text-slate-600">Each seed queues one run per arm. 1 vs 1 is the fair benchmark default; higher caps trade per-run TPS for aggregate throughput.</p>',
)

# Safe production baseline. Raise these values only when the corresponding inference server has matching parallel capacity.
for config_path in ["deploy/models.json", "deploy/models.example.json"]:
    p = Path(config_path)
    doc = json.loads(p.read_text())
    for deployment in doc.get("deployments", []):
        deployment["max_parallel_workers"] = 1
    p.write_text(json.dumps(doc, indent=2) + "\n")

for config_path in ["deploy/modelhost-4090.json", "deploy/modelhost-4090.example.json"]:
    p = Path(config_path)
    doc = json.loads(p.read_text())
    for model in doc.get("models", []):
        model["max_parallel_workers"] = 1
    p.write_text(json.dumps(doc, indent=2) + "\n")

# Regression tests.
p = Path("cmd/pokewall/model_experiments_test.go")
p.write_text(
    p.read_text()
    + r'''

func TestDeploymentWorkerCapQueuesAndSkipsToFreeModel(t *testing.T) {
	registry := writeModelRegistry(t, []farm.ModelDeployment{
		{ID: "model-a", ModelID: "a", Compute: "gpu-a", Endpoint: "http://a/v1", APIModel: "a", Enabled: true, MaxParallelWorkers: 1},
		{ID: "model-b", ModelID: "b", Compute: "gpu-b", Endpoint: "http://b/v1", APIModel: "b", Enabled: true, MaxParallelWorkers: 1},
	})
	t.Setenv("POKEPILOT_MODEL_REGISTRY", registry)
	w := NewWall("")
	h := modelExperimentHTTPHandler(w, w.Handler())
	for _, spec := range []map[string]any{
		{"run_id": "a-1", "planner": "llm", "llm_deployment": "model-a"},
		{"run_id": "a-2", "planner": "llm", "llm_deployment": "model-a"},
		{"run_id": "b-1", "planner": "llm", "llm_deployment": "model-b"},
	} {
		res := requestJSON(t, h, http.MethodPost, "/v1/specs", spec)
		if res.Code != http.StatusOK {
			t.Fatalf("enqueue = %d %s", res.Code, res.Body.String())
		}
	}
	leaseA := requestJSON(t, h, http.MethodPost, "/v1/lease", map[string]any{})
	var first farm.Spec
	if leaseA.Code != http.StatusOK || json.Unmarshal(leaseA.Body.Bytes(), &first) != nil || first.RunID != "a-1" {
		t.Fatalf("first lease = %d %s", leaseA.Code, leaseA.Body.String())
	}
	leaseB := requestJSON(t, h, http.MethodPost, "/v1/lease", map[string]any{})
	var second farm.Spec
	if leaseB.Code != http.StatusOK || json.Unmarshal(leaseB.Body.Bytes(), &second) != nil || second.RunID != "b-1" {
		t.Fatalf("second lease should skip saturated model-a: %d %s", leaseB.Code, leaseB.Body.String())
	}
	blocked := requestJSON(t, h, http.MethodPost, "/v1/lease", map[string]any{})
	if blocked.Code != http.StatusNoContent {
		t.Fatalf("third lease = %d %s, want queued model-a run to wait", blocked.Code, blocked.Body.String())
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.queue) != 1 || w.queue[0] != "a-2" {
		t.Fatalf("queue = %#v", w.queue)
	}
}

func TestExperimentArmConcurrencyIsPartOfComparability(t *testing.T) {
	registry := writeModelRegistry(t, []farm.ModelDeployment{
		{ID: "a", ModelID: "a", Compute: "a", Endpoint: "http://a/v1", APIModel: "a", Enabled: true},
		{ID: "b", ModelID: "b", Compute: "b", Endpoint: "http://b/v1", APIModel: "b", Enabled: true},
	})
	t.Setenv("POKEPILOT_MODEL_REGISTRY", registry)
	w := NewWall("")
	h := modelExperimentHTTPHandler(w, w.Handler())
	created := requestJSON(t, h, http.MethodPost, "/v1/experiments", farm.ExperimentRequest{
		ArmA: farm.ExperimentArm{Deployment: "a", MaxParallelWorkers: 1},
		ArmB: farm.ExperimentArm{Deployment: "b", MaxParallelWorkers: 2},
		Seeds: []int64{1}, Goal: "Earn the Boulder Badge.",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	var view struct{ Pairs []pairResult `json:"pairs"` }
	if err := json.Unmarshal(created.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Pairs) != 1 || view.Pairs[0].Comparable {
		t.Fatalf("different concurrency must not be marked comparable: %#v", view.Pairs)
	}
}
'''
)

p = Path("cmd/pokemodelhost/main_test.go")
p.write_text(
    p.read_text()
    + r'''

func TestLifecycleEnforcesParallelWorkerLimit(t *testing.T) {
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer health.Close()
	service, err := newLifecycleService(hostConfig{
		HostID: "gpu", Compute: "test", PollEvery: "1ms", LoadTimeout: "1s",
		Models: []hostModel{{DeploymentID: "m", ModelID: "m", Endpoint: health.URL, HealthURL: health.URL, APIModel: "m", MaxParallelWorkers: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(service.handler())
	defer server.Close()
	acquire := func(run string) int {
		data, _ := json.Marshal(leaseRequest{RunID: run, DeploymentID: "m", MaxParallelWorkers: 2})
		resp, err := http.Post(server.URL+"/v1/leases/acquire", "application/json", bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := acquire("one"); code != http.StatusAccepted {
		t.Fatalf("first acquire = %d", code)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && service.status().State != "ready" {
		time.Sleep(time.Millisecond)
	}
	if code := acquire("two"); code != http.StatusOK {
		t.Fatalf("second acquire = %d", code)
	}
	if code := acquire("three"); code != http.StatusTooManyRequests {
		t.Fatalf("third acquire = %d, want 429", code)
	}
}
'''
)

Path("farm/model_registry_concurrency_test.go").write_text(
    r'''package farm

import "testing"

func TestModelDeploymentParallelLimitDefaultsToOne(t *testing.T) {
	if got := (ModelDeployment{}).ParallelLimit(); got != 1 {
		t.Fatalf("default parallel limit = %d, want 1", got)
	}
	if got := (ModelDeployment{MaxParallelWorkers: 3}).ParallelLimit(); got != 3 {
		t.Fatalf("explicit parallel limit = %d, want 3", got)
	}
}
'''
)
