(() => {
  "use strict";

  const $ = (id) => document.getElementById(id);
  const liveStatuses = new Set(["leased", "running"]);
  const frameMs = 50; // 20 fps; same cap as the operator console
  const replayStatusTTL = 10000;
  const query = new URLSearchParams(window.location.search);
  let selectedRunID = query.get("run") || "";
  let selectionPinned = query.has("run");
  let snapshot = { now: 0, runs: [], summary: { live: 0, queued: 0, completed: 0 } };
  let wallDown = false;
  let pumpingID = "";
  let pumpStop = null;
  let blobUrl = "";
  let currentReplayID = "";
  const lastOnce = new Set();
  const replayStatusCache = new Map();
  const mapAssets = new Map();
  let mapRenderSerial = 0;
  let mediaSerial = 0;

  function setText(id, value) {
    $(id).textContent = value == null || value === "" ? "—" : String(value);
  }

  function statusLabel(status) {
    if (!status) return "unknown";
    return status.replace(/_/g, " ");
  }

  function isLive(run) {
    return run && liveStatuses.has(run.status);
  }

  function newest(runs, field) {
    return [...runs].sort((a, b) => Number(b[field] || 0) - Number(a[field] || 0))[0] || null;
  }

  function preferredRun(runs) {
    if (!runs.length) return null;
    if (selectionPinned && selectedRunID) {
      const selected = runs.find((run) => run.run_id === selectedRunID);
      if (selected) return selected;
    }
    const running = newest(runs.filter((run) => run.status === "running"), "queued_at");
    if (running) return running;
    const leased = newest(runs.filter((run) => run.status === "leased"), "queued_at");
    if (leased) return leased;
    const queued = newest(runs.filter((run) => run.status === "queued"), "queued_at");
    if (queued) return queued;
    return newest(runs.filter((run) => run.status === "done"), "ended_at") || runs[runs.length - 1];
  }

  function selectRun(runID) {
    selectedRunID = runID;
    selectionPinned = Boolean(runID);
    const url = new URL(window.location.href);
    if (runID) url.searchParams.set("run", runID);
    else url.searchParams.delete("run");
    history.replaceState(null, "", url);
    render();
    syncMedia();
  }

  function runPermalink(runID) {
    const url = new URL(window.location.href);
    url.searchParams.set("run", runID);
    return url.toString();
  }

  function replayState(runID) {
    return replayStatusCache.get(runID)?.status?.state || "";
  }

  function renderStatus(run) {
    const el = $("status");
    const replayReady = run?.status === "done" && replayState(run.run_id) === "ready";
    el.classList.toggle("live", isLive(run));
    el.classList.toggle("replay", replayReady);
    const label = !run ? (wallDown ? "Offline" : "Waiting") : isLive(run) ? "Live" : replayReady ? "Replay" : statusLabel(run.status);
    el.querySelector("span:last-child").textContent = label;
  }

  function renderParty(run) {
    const wrap = $("party");
    wrap.replaceChildren();
    const party = run?.player?.party || [];
    if (!party.length) {
      const empty = document.createElement("span");
      empty.className = "sub";
      empty.textContent = run?.player ? "No party yet" : "Party data not available yet";
      wrap.appendChild(empty);
    } else {
      party.forEach((mon) => {
        const card = document.createElement("div");
        card.className = "mon";
        const name = document.createElement("strong");
        name.textContent = mon.name || "Unknown";
        const meta = document.createElement("small");
        const status = mon.status ? ` · ${mon.status}` : "";
        meta.textContent = `Lv ${mon.level || 0} · ${mon.hp || 0}/${mon.max_hp || 0} HP${status}`;
        const hp = document.createElement("div");
        hp.className = "hp";
        const fill = document.createElement("div");
        const pct = mon.max_hp > 0 ? Math.max(0, Math.min(100, (mon.hp / mon.max_hp) * 100)) : 0;
        fill.style.width = `${pct}%`;
        hp.appendChild(fill);
        card.append(name, meta, hp);
        wrap.appendChild(card);
      });
    }

    const badges = $("badges");
    badges.replaceChildren();
    (run?.player?.badges || []).forEach((badgeName) => {
      const badge = document.createElement("span");
      badge.className = "badge";
      badge.textContent = badgeName;
      badges.appendChild(badge);
    });
  }

  function makeRunButton(run, activeRun) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "run-btn";
    button.classList.toggle("active", activeRun && run.run_id === activeRun.run_id);
    button.addEventListener("click", () => selectRun(run.run_id));

    const top = document.createElement("div");
    top.className = "run-btn-top";
    const title = document.createElement("strong");
    title.textContent = run.run_id || "Untitled run";
    const status = document.createElement("span");
    status.className = "mini-status";
    const ready = run.status === "done" && replayState(run.run_id) === "ready";
    status.classList.toggle("live", isLive(run));
    status.classList.toggle("replay", ready);
    status.textContent = isLive(run) ? "live" : ready ? "replay" : statusLabel(run.status);
    top.append(title, status);

    const meta = document.createElement("small");
    const route = [run.starter, run.dest].filter(Boolean).join(" → ");
    meta.textContent = route || run.goal || "Pokémon Red run";
    button.append(top, meta);
    return button;
  }

  function fillRunList(id, runs, activeRun, emptyText) {
    const list = $(id);
    list.replaceChildren();
    if (!runs.length) {
      const empty = document.createElement("div");
      empty.className = "empty-list";
      empty.textContent = emptyText;
      list.appendChild(empty);
      return;
    }
    runs.forEach((run) => list.appendChild(makeRunButton(run, activeRun)));
  }

  function renderRunLists(runs, activeRun) {
    const live = [...runs]
      .filter((run) => run.status !== "done")
      .sort((a, b) => Number(b.queued_at || 0) - Number(a.queued_at || 0));
    const recent = [...runs]
      .filter((run) => run.status === "done")
      .sort((a, b) => Number(b.ended_at || 0) - Number(a.ended_at || 0));
    fillRunList("live-list", live, activeRun, "Nothing is running right now.");
    fillRunList("recent-list", recent, activeRun, "No completed runs are available yet.");
  }

  function renderFarmSummary() {
    const summary = snapshot.summary || {};
    setText("farm-live", summary.live || 0);
    setText("farm-queued", summary.queued || 0);
    setText("farm-completed", summary.completed || 0);
  }

  function renderStats(run) {
    const stats = run?.stats;
    if (!stats) {
      setText("stats", run?.status === "done" && run?.reason ? `Finished: ${run.reason}` : "No planner stats yet");
      return;
    }
    const rows = [];
    rows.push(`Calls ${stats.calls || 0} · repeats ${stats.repeats || 0} · rejected ${stats.rejected || 0}`);
    if (stats.last_seconds || stats.avg_seconds) rows.push(`Think time ${Number(stats.last_seconds || 0).toFixed(1)}s last · ${Number(stats.avg_seconds || 0).toFixed(1)}s avg`);
    if (run?.attempts > 1) rows.push(`Attempt ${run.attempts}`);
    if (run?.status === "done" && run?.reason) rows.push(`Finished: ${run.reason}`);
    setText("stats", rows.join("\n"));
  }

  function renderGoal(run) {
    const stats = run?.stats;
    setText("goal", stats?.goal_summary || run?.goal || run?.stop_so_far || "Waiting for a goal");
    const bar = $("goal-progress");
    let pct = 0;
    if (stats?.goal_complete) pct = 100;
    else if (stats?.goal_target > 0) pct = Math.max(0, Math.min(100, (stats.goal_current / stats.goal_target) * 100));
    bar.style.transform = `scaleX(${pct / 100})`;
  }

  function mapAssetURL(id) {
    return "/maps/" + Number(id).toString(16).padStart(2, "0") + ".json";
  }

  function loadMapAsset(id) {
    const key = Number(id);
    if (!mapAssets.has(key)) {
      mapAssets.set(key, fetch(mapAssetURL(key), { cache: "force-cache" })
        .then((r) => r.ok ? r.json() : null)
        .catch(() => null));
    }
    return mapAssets.get(key);
  }

  function paintMap(canvas, asset, run) {
    if (!asset || !asset.width || !asset.height || typeof asset.cells !== "string") return false;
    const scroll = canvas.closest(".map-scroll");
    let availW = 280, availH = 260;
    if (scroll) {
      availW = Math.max(1, scroll.clientWidth - 8);
      availH = Math.max(1, scroll.clientHeight - 8);
    }
    if (availW < 8 || availH < 8) return false;
    const px = Math.max(4, Math.floor(Math.min(availW / asset.width, availH / asset.height)));
    canvas.width = asset.width * px;
    canvas.height = asset.height * px;
    const ctx = canvas.getContext("2d");
    if (!ctx) return false;
    ctx.imageSmoothingEnabled = false;
    for (let y = 0; y < asset.height; y++) {
      for (let x = 0; x < asset.width; x++) {
        const ch = asset.cells[y * asset.width + x] || "#";
        ctx.fillStyle = ch === "#" ? "#1a2438" : ch === "g" ? "#2f6b45" : ch === "~" ? "#2a4a78" : "#12192c";
        ctx.fillRect(x * px, y * px, px, px);
        if (ch === "W") {
          ctx.strokeStyle = "#ffd84a";
          ctx.lineWidth = Math.max(1, Math.floor(px / 4));
          ctx.strokeRect(x * px + 1, y * px + 1, Math.max(1, px - 2), Math.max(1, px - 2));
        }
      }
    }
    const trail = Array.isArray(run.trail) ? run.trail : [];
    if (trail.length > 1) {
      ctx.strokeStyle = "#8b9cc4";
      ctx.lineWidth = Math.max(1, Math.floor(px / 3));
      ctx.globalAlpha = 0.55;
      ctx.beginPath();
      trail.forEach((p, i) => {
        const x = (Number(p[0]) + 0.5) * px;
        const y = (Number(p[1]) + 0.5) * px;
        if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
      });
      ctx.stroke();
      ctx.globalAlpha = 1;
    }
    for (const sp of (run.sprites || [])) {
      const sx = Number(sp.x), sy = Number(sp.y);
      if (sx < 0 || sy < 0 || sx >= asset.width || sy >= asset.height) continue;
      ctx.fillStyle = "#ffd84a";
      const pad = Math.max(1, Math.floor(px / 4));
      ctx.fillRect(sx * px + pad, sy * px + pad, Math.max(2, px - pad * 2), Math.max(2, px - pad * 2));
    }
    const pxX = Number(run.x), pxY = Number(run.y);
    if (pxX >= 0 && pxY >= 0 && pxX < asset.width && pxY < asset.height) {
      ctx.fillStyle = "#5ee6a8";
      ctx.beginPath();
      ctx.arc((pxX + 0.5) * px, (pxY + 0.5) * px, Math.max(2, px * 0.42), 0, Math.PI * 2);
      ctx.fill();
    }
    return true;
  }

  function renderMap(run) {
    const panel = $("map-panel");
    const status = $("map-status");
    const canvas = $("live-map");
    const serial = ++mapRenderSerial;
    if (!run || (run.status !== "running" && run.status !== "done")) {
      panel.hidden = true;
      return;
    }
    panel.hidden = false;
    status.textContent = `0x${Number(run.map || 0).toString(16).padStart(2, "0").toUpperCase()} · ${run.x ?? 0},${run.y ?? 0}`;
    loadMapAsset(run.map).then((asset) => {
      if (serial !== mapRenderSerial) return;
      requestAnimationFrame(() => {
        if (serial !== mapRenderSerial) return;
        if (!asset || !paintMap(canvas, asset, run)) panel.hidden = true;
      });
    });
  }

  function render() {
    const runs = Array.isArray(snapshot.runs) ? snapshot.runs : [];
    const run = preferredRun(runs);
    if (run && !selectionPinned) selectedRunID = run.run_id;

    renderRunLists(runs, run);
    renderFarmSummary();
    renderStatus(run);
    $("copy-link").hidden = !run;

    if (!run) {
      setText("run-title", wallDown ? "Spectator feed unavailable" : "Nothing is live right now");
      setText("run-sub", wallDown ? "The public read-only endpoint cannot reach the wall." : "Recent completed runs will appear here when recordings are available.");
      ["map", "position", "round", "badges-count", "goal", "decision", "stats"].forEach((id) => setText(id, "—"));
      $("goal-progress").style.transform = "scaleX(0)";
      renderParty(null);
      $("map-panel").hidden = true;
      return;
    }

    setText("run-title", run.run_id);
    const route = [run.starter, run.dest].filter(Boolean).join(" → ");
    setText("run-sub", route || (isLive(run) ? "Pokémon Red autonomous run" : "Completed run · replay when available"));
    setText("map", `0x${Number(run.map || 0).toString(16).padStart(2, "0").toUpperCase()}`);
    setText("position", `${run.x ?? 0}, ${run.y ?? 0}`);
    setText("round", run.stats ? `${run.stats.round || 0}${run.stats.rounds_left >= 0 ? ` · ${run.stats.rounds_left} left` : ""}` : "—");
    setText("badges-count", run.player?.badges?.length ?? 0);
    setText("decision", run.decision || (run.status === "done" ? run.reason : "Waiting for the next decision"));
    renderGoal(run);
    renderParty(run);
    renderStats(run);
    renderMap(run);
  }

  async function refreshSnapshot() {
    try {
      const response = await fetch("/v1/watch", { cache: "no-store" });
      if (!response.ok) throw new Error(`watch ${response.status}`);
      snapshot = await response.json();
      wallDown = false;
      $("connection").textContent = "Public spectator feed · live state refreshes automatically";
      $("connection").classList.remove("offline");
    } catch (error) {
      wallDown = true;
      $("connection").textContent = "Spectator feed temporarily unavailable";
      $("connection").classList.add("offline");
    }
    render();
    syncMedia();
  }

  function setMediaLabel(text, kind) {
    const label = $("media-label");
    if (!text) {
      label.hidden = true;
      label.textContent = "";
      label.classList.remove("live", "replay");
      return;
    }
    label.hidden = false;
    label.textContent = text;
    label.classList.toggle("live", kind === "live");
    label.classList.toggle("replay", kind === "replay");
  }

  function hideReplay() {
    const video = $("replay");
    if (currentReplayID) {
      video.pause();
      video.removeAttribute("src");
      video.load();
      currentReplayID = "";
    }
    video.hidden = true;
    $("replay-tools").hidden = true;
  }

  function showEmpty(message) {
    const image = $("frame");
    const empty = $("screen-empty");
    hideReplay();
    image.hidden = true;
    empty.hidden = false;
    if (message) empty.textContent = message;
  }

  function completedSummary(run) {
    const goal = run?.stats?.goal_summary || run?.goal || "Pokémon Red run";
    const badges = run?.player?.badges?.length ?? 0;
    const calls = run?.stats?.calls || 0;
    const result = run?.reason ? `Finished: ${run.reason}` : "Run completed";
    return `${result}\n${goal}\n${badges} badge${badges === 1 ? "" : "s"} · ${calls} model call${calls === 1 ? "" : "s"}\nNo public replay or final frame is available for this run.`;
  }

  function paintFrame(url) {
    const image = $("frame");
    const empty = $("screen-empty");
    hideReplay();
    image.onload = () => {
      image.hidden = false;
      empty.hidden = true;
    };
    image.onerror = () => showEmpty("Waiting for the live game screen…");
    image.src = url;
  }

  function showReplay(run) {
    stopPump();
    const image = $("frame");
    const empty = $("screen-empty");
    const video = $("replay");
    image.hidden = true;
    empty.hidden = true;
    video.hidden = false;
    $("replay-tools").hidden = false;
    setMediaLabel("Replay", "replay");
    if (currentReplayID !== run.run_id) {
      currentReplayID = run.run_id;
      video.src = `/v1/watch/runs/${encodeURIComponent(run.run_id)}/replay/video`;
      video.load();
    }
    video.playbackRate = Number($("playback-rate").value || 1);
    video.onerror = () => showEmpty(completedSummary(run));
  }

  function sleep(ms) { return new Promise((ok) => setTimeout(ok, ms)); }

  function whenVisible() {
    if (!document.hidden) return Promise.resolve();
    return new Promise((ok) => {
      const on = () => {
        if (document.hidden) return;
        document.removeEventListener("visibilitychange", on);
        ok();
      };
      document.addEventListener("visibilitychange", on);
    });
  }

  function stopPump() {
    if (!pumpStop) return;
    pumpStop();
    pumpStop = null;
    pumpingID = "";
  }

  function ensurePump(id) {
    if (pumpingID === id) return;
    stopPump();
    hideReplay();
    setMediaLabel("● Live", "live");
    pumpingID = id;
    let stop = false;
    pumpStop = () => { stop = true; };
    (async function loop() {
      while (!stop) {
        await whenVisible();
        if (stop) break;
        const started = Date.now();
        try {
          const r = await fetch("/frame?run=" + encodeURIComponent(id), { cache: "no-store" });
          if (r.ok) {
            const url = URL.createObjectURL(await r.blob());
            paintFrame(url);
            setMediaLabel("● Live", "live");
            if (blobUrl) URL.revokeObjectURL(blobUrl);
            blobUrl = url;
          }
        } catch (e) {}
        const wait = frameMs - (Date.now() - started);
        if (wait > 0) await sleep(wait);
      }
    })();
  }

  function fetchLast(run) {
    const id = run.run_id;
    if (lastOnce.has(id)) return;
    lastOnce.add(id);
    (async () => {
      try {
        const r = await fetch("/frame?run=" + encodeURIComponent(id), { cache: "no-store" });
        if (!r.ok) {
          showEmpty(completedSummary(run));
          return;
        }
        paintFrame(URL.createObjectURL(await r.blob()));
        setMediaLabel("Final frame", "");
      } catch (e) {
        lastOnce.delete(id);
        showEmpty(completedSummary(run));
      }
    })();
  }

  async function getReplayStatus(runID) {
    const cached = replayStatusCache.get(runID);
    const now = Date.now();
    if (cached && now - cached.at < replayStatusTTL) return cached.status;
    try {
      const response = await fetch(`/v1/watch/runs/${encodeURIComponent(runID)}/replay/status`, { cache: "no-store" });
      if (!response.ok) {
        const status = { run_id: runID, state: "unavailable" };
        replayStatusCache.set(runID, { at: now, status });
        return status;
      }
      const status = await response.json();
      replayStatusCache.set(runID, { at: now, status });
      return status;
    } catch (e) {
      const status = { run_id: runID, state: "unavailable" };
      replayStatusCache.set(runID, { at: now, status });
      return status;
    }
  }

  async function syncMedia() {
    const serial = ++mediaSerial;
    const run = preferredRun(Array.isArray(snapshot.runs) ? snapshot.runs : []);
    if (!run || !run.run_id) {
      stopPump();
      setMediaLabel("", "");
      showEmpty(wallDown ? "Spectator feed unavailable." : "Nothing is live right now.\nChoose a recent run to inspect its public summary.");
      return;
    }
    if (run.status === "running") {
      ensurePump(run.run_id);
      return;
    }
    stopPump();
    if (run.status === "done") {
      const status = await getReplayStatus(run.run_id);
      if (serial !== mediaSerial) return;
      renderStatus(run);
      renderRunLists(Array.isArray(snapshot.runs) ? snapshot.runs : [], run);
      if (status.state === "ready") {
        showReplay(run);
        return;
      }
      setMediaLabel("Completed", "");
      fetchLast(run);
      return;
    }
    setMediaLabel("Waiting", "");
    showEmpty(run.status === "queued" || run.status === "leased" ? "Waiting for the live game screen…" : "No captured frame is available for this run.");
  }

  $("playback-rate").addEventListener("change", () => {
    $("replay").playbackRate = Number($("playback-rate").value || 1);
  });

  $("copy-link").addEventListener("click", async () => {
    const run = preferredRun(Array.isArray(snapshot.runs) ? snapshot.runs : []);
    if (!run) return;
    const link = runPermalink(run.run_id);
    try {
      await navigator.clipboard.writeText(link);
      const button = $("copy-link");
      const before = button.textContent;
      button.textContent = "Copied";
      setTimeout(() => { button.textContent = before; }, 1200);
    } catch (e) {
      window.prompt("Copy this run link", link);
    }
  });

  refreshSnapshot();
  setInterval(refreshSnapshot, 2000);
})();
