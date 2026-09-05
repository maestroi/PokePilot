(() => {
  const root = document.getElementById("run-inspector") || document.createElement("section");
  if (!root.id) {
    root.id = "run-inspector";
    const host = document.querySelector(".history .ops-inner") || document.querySelector("main") || document.body;
    host.appendChild(root);
  }
  root.className = "inspect";
  root.innerHTML = `
    <div class="inspect-head">
      <h3>Selected run</h3>
      <div class="inspect-controls">
        <label>Run <select id="pp-inspector-run"><option value="">Loading…</option></select></label>
        <button id="pp-inspector-refresh" type="button" class="pager-btn">Refresh</button>
      </div>
    </div>
    <p id="pp-inspector-empty" class="empty">Select a run from the list to replay it and browse artifacts.</p>
    <div id="pp-inspector-content" class="inspect-grid" hidden>
      <div class="block">
        <h3>Run</h3>
        <dl id="pp-inspector-meta" class="kv"></dl>
        <div class="inspect-actions">
          <button id="pp-inspector-replay" type="button" class="pager-btn">Replay recording</button>
          <span id="pp-inspector-replay-status" class="inspect-status"></span>
        </div>
        <video id="pp-inspector-video" controls preload="metadata" hidden></video>
        <p id="pp-inspector-art-empty" class="inspect-status">No artifacts recorded for this run.</p>
        <details class="plan-raw"><summary>Debug bundle</summary><pre id="pp-inspector-debug"></pre></details>
      </div>
      <div id="pp-inspector-art-table" class="block inspect-arts" hidden>
        <h3>Artifacts</h3>
        <div class="inspect-table-wrap">
          <table>
            <thead><tr><th>Name</th><th>Type</th><th>Size</th><th>Storage</th><th>SHA-256</th><th></th></tr></thead>
            <tbody id="pp-inspector-artifacts"></tbody>
          </table>
        </div>
      </div>
    </div>`;

  const $ = (id) => root.querySelector(id);
  const runSelect = $("#pp-inspector-run");
  const refreshButton = $("#pp-inspector-refresh");
  const empty = $("#pp-inspector-empty");
  const content = $("#pp-inspector-content");
  const meta = $("#pp-inspector-meta");
  const debugPre = $("#pp-inspector-debug");
  const artifactBody = $("#pp-inspector-artifacts");
  const artifactEmpty = $("#pp-inspector-art-empty");
  const artifactTable = $("#pp-inspector-art-table");
  const replayButton = $("#pp-inspector-replay");
  const replayStatus = $("#pp-inspector-replay-status");
  const video = $("#pp-inspector-video");
  let selectedRun = "";
  let selectedDebug = null;
  let replayPoll = 0;
  const esc = encodeURIComponent;
  const text = (v) => v === undefined || v === null || v === "" ? "—" : String(v);
  const short = (s, n = 16) => !s ? "—" : (s.length > n ? `${s.slice(0, n)}…` : s);
  const fmtTime = (unix) => unix ? new Date(Number(unix) * 1000).toLocaleString() : "—";
  const fmtSize = (raw) => {
    let n = Number(raw || 0);
    if (!Number.isFinite(n) || n <= 0) return "—";
    const u = ["B", "KiB", "MiB", "GiB"];
    let i = 0;
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
    return `${n >= 10 || i === 0 ? n.toFixed(0) : n.toFixed(1)} ${u[i]}`;
  };

  async function json(url, options) {
    const res = await fetch(url, { cache: "no-store", ...options });
    let body = null;
    try { body = await res.json(); } catch (_) {}
    if (!res.ok) throw new Error((body && body.error) || `${res.status} ${res.statusText}`);
    return body;
  }
  function stopReplayPoll() {
    if (replayPoll) clearTimeout(replayPoll);
    replayPoll = 0;
  }
  function kv(label, value, title) {
    const dt = document.createElement("dt");
    dt.textContent = label;
    const dd = document.createElement("dd");
    dd.textContent = text(value);
    if (title) dd.title = title;
    return [dt, dd];
  }
  function ensureOption(id, label) {
    if (!id) return;
    if ([...runSelect.options].some((o) => o.value === id)) return;
    const o = document.createElement("option");
    o.value = id;
    o.textContent = label || id;
    runSelect.append(o);
  }
  function showEmpty(message) {
    empty.textContent = message;
    empty.hidden = false;
    content.hidden = true;
  }
  function renderDebug(debug) {
    const run = debug.run || {};
    const finish = debug.finish || {};
    const summary = debug.summary || {};
    const stats = run.stats || {};
    meta.replaceChildren();
    const rows = [
      ["status", run.status],
      ["attempt", finish.attempt || run.attempts || 1],
      ["goal", run.goal, run.goal],
      ["reason", finish.reason || run.reason, finish.detail || run.detail],
      ["map", `${run.map ?? "—"} · ${run.x ?? "—"},${run.y ?? "—"}`],
      ["frame", run.frame],
      ["queued", fmtTime(run.queued_at)],
      ["ended", fmtTime(run.ended_at)],
      ["runner", short(finish.runner_version, 18), finish.runner_version],
      ["model", stats.model || run.llm_profile],
      ["rounds", stats.rounds ?? stats.round],
      ["progress", summary.progress_known ? (summary.progressed ? "yes" : "no") : "unknown"],
    ];
    for (const [label, value, title] of rows) {
      if (value === "" || value == null) continue;
      meta.append(...kv(label, value, title));
    }
    debugPre.textContent = JSON.stringify(debug, null, 2);
  }
  function renderArtifacts(list) {
    artifactBody.replaceChildren();
    const artifacts = Array.isArray(list.artifacts) ? list.artifacts : [];
    if (!artifacts.length) {
      artifactEmpty.hidden = false;
      artifactTable.hidden = true;
      return;
    }
    artifactEmpty.hidden = true;
    artifactTable.hidden = false;
    for (const a of artifacts) {
      const row = document.createElement("tr");
      [a.name, a.media_type || "application/octet-stream", fmtSize(a.size), a.store || "inline", short(a.sha256, 12)].forEach((v, i) => {
        const c = document.createElement("td");
        c.textContent = v;
        if (i === 4 && a.sha256) c.title = a.sha256;
        row.append(c);
      });
      const action = document.createElement("td");
      const link = document.createElement("a");
      link.textContent = "Download";
      link.href = `/v1/runs/${esc(selectedRun)}/artifacts/${esc(a.name)}/content`;
      link.setAttribute("download", a.name);
      action.append(link);
      row.append(action);
      artifactBody.append(row);
    }
  }
  function setReplayState(status) {
    const state = status && status.state ? status.state : "missing";
    replayButton.disabled = false;
    video.hidden = true;
    if (state === "ready") {
      replayStatus.textContent = status.size ? `Ready · ${fmtSize(status.size)}` : "Ready";
      replayButton.hidden = true;
      replayButton.disabled = true;
      const src = `/v1/runs/${esc(selectedRun)}/replay/video`;
      if (video.dataset.run !== selectedRun) {
        video.src = src;
        video.dataset.run = selectedRun;
        video.load();
      }
      video.hidden = false;
      return;
    }
    if (state === "generating") {
      replayStatus.textContent = "Rendering deterministic replay…";
      replayButton.textContent = "Rendering…";
      replayButton.disabled = true;
      stopReplayPoll();
      replayPoll = setTimeout(() => loadReplayStatus(selectedRun), 1500);
      return;
    }
    if (state === "disabled") {
      replayStatus.textContent = status.error || "Replay service disabled";
      replayButton.hidden = true;
      replayButton.disabled = true;
      return;
    }
    if (state === "error") {
      replayStatus.textContent = status.error || "Replay render failed";
      replayButton.textContent = "Retry replay";
      return;
    }
    replayStatus.textContent = "Recording available; video will be generated and cached on first replay.";
    replayButton.textContent = "Replay recording";
  }
  async function loadReplayStatus(runID) {
    if (!runID || runID !== selectedRun) return;
    try {
      const status = await json(`/v1/runs/${esc(runID)}/replay/status`);
      if (runID === selectedRun) setReplayState(status);
    } catch (err) {
      if (runID !== selectedRun) return;
      replayStatus.textContent = err.message;
      replayButton.textContent = "Replay unavailable";
      replayButton.disabled = true;
      video.hidden = true;
    }
  }
  async function selectRun(runID) {
    stopReplayPoll();
    selectedRun = runID;
    selectedDebug = null;
    video.pause();
    video.removeAttribute("src");
    video.dataset.run = "";
    video.hidden = true;
    if (!runID) {
      showEmpty("Select a run from the list to replay it and browse artifacts.");
      return;
    }
    empty.hidden = true;
    content.hidden = false;
    replayStatus.textContent = "Loading…";
    replayButton.hidden = false;
    replayButton.disabled = true;
    try {
      const [debug, artifacts] = await Promise.all([
        json(`/v1/runs/${esc(runID)}/debug`),
        json(`/v1/runs/${esc(runID)}/artifacts`),
      ]);
      if (runID !== selectedRun) return;
      selectedDebug = debug;
      renderDebug(debug);
      renderArtifacts(artifacts);
      const replayable = Array.isArray(artifacts.artifacts) && artifacts.artifacts.some((a) => a.replayable);
      if (!replayable) {
        replayStatus.textContent = "No run.gbrun recording for this run.";
        replayButton.hidden = true;
        replayButton.disabled = true;
      } else {
        replayButton.hidden = false;
        await loadReplayStatus(runID);
      }
    } catch (err) {
      showEmpty(`Could not inspect ${runID}: ${err.message}`);
    }
  }
  async function refreshRuns() {
    const previous = selectedRun || runSelect.value;
    try {
      const dashboard = await json("/v1/dashboard");
      const runs = Array.isArray(dashboard.runs) ? dashboard.runs : [];
      runSelect.replaceChildren();
      if (!runs.length && !previous) {
        const o = document.createElement("option");
        o.value = "";
        o.textContent = "No runs yet";
        runSelect.append(o);
        selectedRun = "";
        showEmpty("No runs are available yet.");
        return;
      }
      for (const run of runs) {
        const o = document.createElement("option");
        o.value = run.run_id;
        o.textContent = [run.run_id, run.status].filter(Boolean).join(" · ");
        runSelect.append(o);
      }
      if (previous && !runs.some((r) => r.run_id === previous)) ensureOption(previous, previous);
      if (!previous) {
        const blank = document.createElement("option");
        blank.value = "";
        blank.textContent = "Select a run";
        runSelect.insertBefore(blank, runSelect.firstChild);
        runSelect.value = "";
        showEmpty("Select a run from the list to replay it and browse artifacts.");
        return;
      }
      runSelect.value = previous;
      if (previous !== selectedRun) await selectRun(previous);
    } catch (err) {
      showEmpty(`Run inspector unavailable: ${err.message}`);
    }
  }

  replayButton.addEventListener("click", async () => {
    if (!selectedRun || !selectedDebug) return;
    replayButton.disabled = true;
    replayStatus.textContent = "Starting replay render…";
    try {
      setReplayState(await json(`/v1/runs/${esc(selectedRun)}/replay/render`, { method: "POST" }));
    } catch (err) {
      replayStatus.textContent = err.message;
      replayButton.textContent = "Retry replay";
      replayButton.disabled = false;
    }
  });
  runSelect.addEventListener("change", () => {
    window.dispatchEvent(new CustomEvent("pokefarm-select-run", { detail: { runId: runSelect.value } }));
  });
  refreshButton.addEventListener("click", async () => {
    refreshButton.disabled = true;
    try {
      await refreshRuns();
      if (selectedRun) await selectRun(selectedRun);
    } finally {
      refreshButton.disabled = false;
    }
  });
  window.addEventListener("pokefarm-select-run", (ev) => {
    const id = (ev.detail && ev.detail.runId) || "";
    if (id) ensureOption(id, id);
    runSelect.value = id;
    if (id !== selectedRun) selectRun(id);
  });
  window.addEventListener("beforeunload", stopReplayPoll, { once: true });
  refreshRuns();
  setInterval(refreshRuns, 8000);
})();
