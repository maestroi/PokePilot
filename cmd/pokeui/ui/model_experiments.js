(() => {
  const $ = (id) => document.getElementById(id);
  const esc = (value) => String(value == null ? "" : value).replace(/[&<>"']/g, (c) => ({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;"}[c]));
  let deployments = [];
  let selectedDeployment = "";

  function deploymentState(d) {
    const state = d.state || (d.enabled ? "available" : "disabled");
    const suffix = d.compute ? ` · ${d.compute}` : "";
    return `${d.label || d.id}${suffix} · ${state}`;
  }

  function chooseDefaults() {
    const enabled = deployments.filter((d) => d.enabled !== false);
    const a = enabled.find((d) => /27b/i.test(`${d.model_id} ${d.label}`) && /7900/i.test(`${d.compute} ${d.label}`)) || enabled[0];
    const b = enabled.find((d) => /4b/i.test(`${d.model_id} ${d.label}`) && /4090/i.test(`${d.compute} ${d.label}`)) || enabled.find((d) => d.id !== (a && a.id));
    return [a && a.id || "", b && b.id || ""];
  }

  function selectable(d) {
    return d.enabled !== false && !["unavailable", "failed", "busy"].includes(d.state);
  }

  function renderRunSelector() {
    const form = $("spec-form");
    if (!form || !deployments.length) return;
    const legacy = form.elements.llm_profile;
    if (!legacy) return;
    let select = form.elements.llm_deployment;
    if (!select) {
      const label = document.createElement("label");
      label.className = "llm-only deployment-field";
      label.innerHTML = `Deployment <select name="llm_deployment"></select><small class="deployment-help">Model identity + compute placement</small>`;
      legacy.closest("label").before(label);
      legacy.closest("label").hidden = true;
      select = form.elements.llm_deployment;
      select.addEventListener("change", () => { selectedDeployment = select.value; });
    }
    const previous = selectedDeployment || select.value;
    select.innerHTML = deployments.map((d) => `<option value="${esc(d.id)}" ${selectable(d) ? "" : "disabled"}>${esc(deploymentState(d))}</option>`).join("");
    const fallback = deployments.find(selectable);
    select.value = deployments.some((d) => d.id === previous && selectable(d)) ? previous : (fallback && fallback.id || "");
    selectedDeployment = select.value;
  }

  function renderRouting() {
    const bay = $("operations-llm");
    if (!bay || !deployments.length) return;
    const rows = deployments.map((d) => {
      const state = d.state || "unknown";
      const identity = [d.model_id, d.revision, d.quantization].filter(Boolean).join(" · ");
      const loaded = d.loaded_deployment && d.loaded_deployment !== d.id ? `loaded: ${d.loaded_deployment}` : (d.active_leases ? `${d.active_leases} active lease${d.active_leases === 1 ? "" : "s"}` : "");
      return `<div class="operation-row deployment-row"><span><strong>${esc(d.label || d.id)}</strong><small>${esc(identity || d.id)}</small></span><span><strong>${esc(d.compute || "compute")}</strong><small>${esc(d.endpoint || "")}</small></span><span class="deployment-state deployment-${esc(state)}"><strong>${esc(state)}</strong><small>${esc(loaded || d.error || "")}</small></span></div>`;
    }).join("");
    bay.innerHTML = `<header><h3 id="operations-llm-title">LLM deployments</h3><p>First-class model identity, compute placement, and host lifecycle</p></header>${rows}<p class="empty">Busy hosts keep incompatible runs queued. Loading finishes before a worker lease is handed out.</p>`;
  }

  function experimentMarkup() {
    const [a, b] = chooseDefaults();
    const options = (selected) => deployments.map((d) => `<option value="${esc(d.id)}" ${d.id === selected ? "selected" : ""} ${d.enabled === false ? "disabled" : ""}>${esc(deploymentState(d))}</option>`).join("");
    return `<section id="operations-experiments" class="ops-bay experiment-bay"><header><h3>Paired LLM experiment</h3><p>Matched A/B runs share seed and gameplay policy; only deployment differs</p></header>
      <form id="experiment-form"><div class="fields experiment-fields">
        <label>Name <input name="name" value="Brock · 27B vs 4B"></label>
        <label>Arm A <select name="arm_a">${options(a)}</select></label>
        <label>Arm B <select name="arm_b">${options(b)}</select></label>
        <label>Goal <input name="goal" value="Earn the Boulder Badge."></label>
        <label>Starter <select name="starter"><option value="">Let LLM decide</option><option value="squirtle">Squirtle</option><option value="charmander">Charmander</option><option value="bulbasaur">Bulbasaur</option></select></label>
        <label>Seed count <input name="seed_count" type="number" min="1" max="1000" value="20"></label>
        <label class="experiment-seeds">Seed list <input name="seeds" placeholder="optional: 1, 7, 42"></label>
        <label>Play style <select name="play_style"><option value="">Default</option><option value="speedrunner">Speedrunner</option><option value="adventure">Adventure</option><option value="completionist">Completionist</option></select></label>
        <label>Risk <select name="risk_tolerance"><option value="">Default</option><option value="cautious">Cautious</option><option value="balanced" selected>Balanced</option><option value="aggressive">Aggressive</option></select></label>
        <label>Wild encounters <select name="wild_encounters"><option value="">Default</option><option value="flee" selected>Flee</option><option value="fight">Fight</option></select></label>
        <label>Reasoning <select name="reasoning_effort"><option value="">Endpoint default</option><option value="off">Off</option><option value="low">Low</option><option value="medium" selected>Medium</option><option value="high">High</option></select></label>
        <label>FPS <input name="fps" type="number" value="0"></label>
        <label>Round cap <input name="max_rounds" type="number" min="0" value="0"></label>
        <label>Frame cap <input name="max_frames" type="number" min="0" value="0"></label>
      </div><div class="form-row"><button class="primary-action" type="submit">Start paired experiment</button><span id="experiment-error" role="status"></span></div></form>
      <div id="experiment-summary" class="experiment-summary"><p class="empty">No paired experiment loaded yet.</p></div></section>`;
  }

  function ensureExperimentBay() {
    const grid = document.querySelector("#view-operations .operations-grid");
    if (!grid || !deployments.length) return;
    const existing = $("operations-experiments");
    if (!existing) {
      grid.insertAdjacentHTML("beforeend", experimentMarkup());
      $("experiment-form").addEventListener("submit", startExperiment);
    } else {
      const a = existing.querySelector("select[name=arm_a]");
      const b = existing.querySelector("select[name=arm_b]");
      const currentA = a && a.value, currentB = b && b.value;
      if (a) a.innerHTML = deployments.map((d) => `<option value="${esc(d.id)}" ${d.id === currentA ? "selected" : ""}>${esc(deploymentState(d))}</option>`).join("");
      if (b) b.innerHTML = deployments.map((d) => `<option value="${esc(d.id)}" ${d.id === currentB ? "selected" : ""}>${esc(deploymentState(d))}</option>`).join("");
    }
  }

  async function startExperiment(ev) {
    ev.preventDefault();
    const f = ev.currentTarget;
    const error = $("experiment-error");
    error.textContent = "";
    const seeds = f.seeds.value.split(/[\s,]+/).map((v) => Number(v)).filter((v) => Number.isFinite(v));
    const payload = {
      name: f.name.value.trim(), arm_a: { name: "A", deployment: f.arm_a.value }, arm_b: { name: "B", deployment: f.arm_b.value },
      goal: f.goal.value.trim(), starter: f.starter.value, seeds, seed_count: seeds.length ? seeds.length : Number(f.seed_count.value || 20),
      play_style: f.play_style.value, risk_tolerance: f.risk_tolerance.value, wild_encounters: f.wild_encounters.value,
      reasoning_effort: f.reasoning_effort.value, fps: Number(f.fps.value || 0), max_rounds: Number(f.max_rounds.value || 0), max_frames: Number(f.max_frames.value || 0)
    };
    try {
      const res = await fetch("/v1/experiments", { method: "POST", headers: {"Content-Type":"application/json"}, body: JSON.stringify(payload) });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) { error.textContent = body.error || "could not create experiment"; return; }
      renderExperiment(body);
    } catch (_) { error.textContent = "wall unreachable"; }
  }

  function renderExperiment(exp) {
    const target = $("experiment-summary");
    if (!target || !exp) return;
    const a = exp.arm_a || {}, b = exp.arm_b || {}, paired = exp.paired || {};
    const pct = (v) => `${Math.round((Number(v) || 0) * 100)}%`;
    const metric = (label, left, right) => `<div class="experiment-metric"><span>${esc(label)}</span><strong>${esc(left)}</strong><strong>${esc(right)}</strong></div>`;
    target.innerHTML = `<header class="experiment-summary-head"><div><span class="section-kicker">${esc(exp.id || "experiment")}</span><h4>${esc(exp.name || "Paired experiment")}</h4></div><span>${esc(exp.total_pairs || 0)} pairs</span></header>
      <div class="experiment-arm-head"><span>Metric</span><strong>Arm A</strong><strong>Arm B</strong></div>
      ${metric("Boulder success", `${a.boulder_successes || 0}/${a.done || 0} · ${pct(a.success_rate)}`, `${b.boulder_successes || 0}/${b.done || 0} · ${pct(b.success_rate)}`)}
      ${metric("Completed runs", `${a.done || 0}/${a.runs || 0}`, `${b.done || 0}/${b.runs || 0}`)}
      ${metric("Rounds", a.rounds || 0, b.rounds || 0)}${metric("Frames", a.frames || 0, b.frames || 0)}
      ${metric("Strategic calls", a.strategic_calls || 0, b.strategic_calls || 0)}${metric("Avg strategic call", `${Number(a.avg_strategic_call_seconds || 0).toFixed(2)}s`, `${Number(b.avg_strategic_call_seconds || 0).toFixed(2)}s`)}
      ${metric("Prompt / completion tokens", `${a.prompt_tokens || 0} / ${a.completion_tokens || 0}`, `${b.prompt_tokens || 0} / ${b.completion_tokens || 0}`)}
      ${metric("Rejected / transport / fallback", `${a.rejected || 0} / ${a.transport_errors || 0} / ${a.fallbacks || 0}`, `${b.rejected || 0} / ${b.transport_errors || 0} / ${b.fallbacks || 0}`)}
      ${metric("Plan exec / skipped", `${a.plan_executions || 0} / ${a.steps_skipped || 0}`, `${b.plan_executions || 0} / ${b.steps_skipped || 0}`)}
      <div class="paired-score"><strong>A wins ${paired.a_wins || 0}</strong><strong>Ties ${paired.ties || 0}</strong><strong>B wins ${paired.b_wins || 0}</strong></div>`;
  }

  async function refreshExperiments() {
    try {
      const res = await fetch("/v1/experiments", { cache: "no-store" });
      if (!res.ok) return;
      const body = await res.json();
      const latest = body.experiments && body.experiments[0];
      if (latest) renderExperiment(latest);
    } catch (_) {}
  }

  async function refreshModels() {
    try {
      const res = await fetch("/v1/models", { cache: "no-store" });
      if (!res.ok) return;
      const body = await res.json();
      deployments = body.deployments || [];
      renderRunSelector(); renderRouting(); ensureExperimentBay();
    } catch (_) {}
  }

  // ui.js owns the legacy run form payload. Inject the first-class deployment
  // field at the fetch boundary so old form code and old llm_profile defaults
  // remain source-compatible while the wall receives explicit model identity.
  const previousFetch = window.fetch.bind(window);
  window.fetch = (input, init) => {
    const url = typeof input === "string" ? input : ((input && input.url) || "");
    if (url === "/v1/specs" && init && String(init.method || "GET").toUpperCase() === "POST" && init.body) {
      try {
        const payload = JSON.parse(init.body);
        const form = $("spec-form");
        const deployment = form && form.elements.llm_deployment && form.elements.llm_deployment.value;
        if (payload.planner === "llm" && deployment) payload.llm_deployment = deployment;
        init = { ...init, body: JSON.stringify(payload) };
      } catch (_) {}
    }
    return previousFetch(input, init);
  };

  refreshModels(); refreshExperiments();
  setInterval(refreshModels, 5000);
  setInterval(refreshExperiments, 5000);
})();
