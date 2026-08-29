(function () {
  const pollMs = 2000;
  const frameMs = 50; // 20 fps; a tight /frame loop burned the Chrome tab
  let snap = { now: 0, runs: [], workers: [] };
  let selected = "";
  let cardErr = "";
  let wallDown = false;
  const pumps = new Map();

  const $ = (id) => document.getElementById(id);
  const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;"
  }[c]));
  const hexMap = (n) => "0x" + Number(n).toString(16).padStart(2, "0");
  const routeLine = (r) => r.planner === "llm"
    ? ((r.starter || "squirtle") + " · play the game")
    : ((r.starter || "—") + " → " + (r.dest || "—"));

  function newRunId() {
    const n = new Uint32Array(2);
    crypto.getRandomValues(n);
    return "run-" + n[0].toString(36) + n[1].toString(36);
  }
  function syncPlannerFields() {
    const scripted = $("spec-form").planner.value === "scripted";
    document.querySelectorAll(".scripted-only").forEach((el) => { el.hidden = !scripted; });
  }
  function fillDefaults() {
    const f = $("spec-form");
    f.run_id.value = newRunId();
    f.planner.value = "llm";
    f.starter.value = "squirtle";
    f.dest.value = "viridian pokemon center";
    f.seed.value = "0";
    f.fps.value = "60";
    f.max_rounds.value = "32";
    f.max_frames.value = "0";
    syncPlannerFields();
  }

  function liveRuns() { return (snap.runs || []).filter((r) => r.status !== "done"); }
  function doneRuns() { return (snap.runs || []).filter((r) => r.status === "done"); }

  function fillLcd(lcd, run) {
    if (run.status === "queued" || run.status === "leased") {
      lcd.dataset.frameRun = "";
      lcd.innerHTML = `<span class="idle">${esc(run.status)}</span>`;
      return;
    }
    lcd.dataset.frameRun = run.run_id;
    if (!lcd.querySelector("img")) {
      lcd.innerHTML = `<span class="idle">live</span>`;
    }
  }

  function paintFrame(id, url) {
    document.querySelectorAll(".lcd").forEach((lcd) => {
      if (lcd.dataset.frameRun !== id) return;
      let img = lcd.querySelector("img");
      if (!img) {
        lcd.replaceChildren();
        img = new Image(160, 144);
        img.alt = "";
        img.src = url;
        lcd.appendChild(img);
        return;
      }
      img.src = url;
    });
  }

  function sleep(ms) {
    return new Promise((ok) => setTimeout(ok, ms));
  }
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

  function ensurePump(id) {
    if (pumps.has(id)) return;
    let stop = false;
    pumps.set(id, () => { stop = true; });
    (async function loop() {
      let blobUrl = "";
      while (!stop) {
        await whenVisible();
        if (stop) break;
        const started = Date.now();
        try {
          const r = await fetch("/frame?run=" + encodeURIComponent(id), { cache: "no-store" });
          if (r.ok) {
            const url = URL.createObjectURL(await r.blob());
            paintFrame(id, url);
            if (blobUrl) URL.revokeObjectURL(blobUrl);
            blobUrl = url;
          }
        } catch (e) { /* next tick */ }
        const wait = frameMs - (Date.now() - started);
        if (wait > 0) await sleep(wait);
      }
    })();
  }

  const lastOnce = new Set();
  function fetchLast(id) {
    if (lastOnce.has(id)) return;
    const has = [...document.querySelectorAll(".lcd")].some(
      (lcd) => lcd.dataset.frameRun === id && lcd.querySelector("img")
    );
    if (has) return;
    lastOnce.add(id);
    (async () => {
      try {
        const r = await fetch("/frame?run=" + encodeURIComponent(id), { cache: "no-store" });
        if (!r.ok) {
          lastOnce.delete(id);
          return;
        }
        paintFrame(id, URL.createObjectURL(await r.blob()));
      } catch (e) {
        lastOnce.delete(id);
      }
    })();
  }

  function syncPumps() {
    const want = new Set();
    for (const r of liveRuns()) {
      if (r.status === "running") want.add(r.run_id);
    }
    const sel = (snap.runs || []).find((r) => r.run_id === selected);
    if (sel && sel.status === "running") want.add(sel.run_id);
    for (const id of want) ensurePump(id);
    for (const [id, stop] of pumps) {
      if (want.has(id)) continue;
      stop();
      pumps.delete(id);
    }
    if (sel && sel.status === "done") fetchLast(sel.run_id);
  }

  function renderLive() {
    const runs = liveRuns();
    const el = $("live");
    if (!runs.length) {
      el.innerHTML = `<p class="empty">No runs yet</p>`;
      return;
    }
    const empty = el.querySelector(".empty");
    if (empty) empty.remove();
    const seen = new Set();
    for (const r of runs) {
      seen.add(r.run_id);
      let art = el.querySelector('article[data-run="' + CSS.escape(r.run_id) + '"]');
      if (!art) {
        art = document.createElement("article");
        art.className = "bezel";
        art.tabIndex = 0;
        art.setAttribute("role", "button");
        art.dataset.run = r.run_id;
        art.innerHTML = `<div class="lcd"></div>
          <div class="meta">
            <span class="chip"></span>
            <div class="rid"></div>
            <div class="route"></div>
            <div class="stats"></div>
            <div class="pos"></div>
            <button type="button" class="cancel"></button>
            <div class="card-err" hidden></div>
          </div>`;
        el.appendChild(art);
      }
      art.classList.toggle("selected", r.run_id === selected);
      fillLcd(art.querySelector(".lcd"), r);
      art.querySelector(".chip").className = "chip " + r.status;
      art.querySelector(".chip").textContent = r.status;
      art.querySelector(".rid").textContent = r.run_id;
      art.querySelector(".route").textContent = routeLine(r);
      art.querySelector(".stats").textContent = "seed " + r.seed + " · frame " + r.frame;
      art.querySelector(".pos").textContent = hexMap(r.map) + " (" + r.x + "," + r.y + ") · attempt " + r.attempts;
      const cancel = art.querySelector(".cancel");
      cancel.hidden = r.status === "done";
      cancel.dataset.cancel = r.run_id;
      cancel.textContent = "Cancel run";
      const err = art.querySelector(".card-err");
      if (cardErr && cardErr.id === r.run_id) {
        err.hidden = false;
        err.textContent = cardErr.text;
      } else {
        err.hidden = true;
        err.textContent = "";
      }
    }
    el.querySelectorAll("article").forEach((art) => {
      if (!seen.has(art.dataset.run)) art.remove();
    });
  }

  function renderWorkers() {
    const ws = snap.workers || [];
    const el = $("workers");
    if (!ws.length) {
      el.innerHTML = `<p class="empty">No workers</p>`;
      return;
    }
    el.innerHTML = ws.map((w) => {
      const st = w.run_id ? `running <b>${esc(w.run_id)}</b>` : "<b>idle</b>";
      return `<div class="worker">${esc(w.addr)} · ${st} · ${esc(w.seen_ago)} ago</div>`;
    }).join("");
  }

  function renderHistory() {
    const runs = doneRuns();
    const el = $("history");
    if (!runs.length) {
      el.innerHTML = `<p class="empty">Nothing finished yet</p>`;
      return;
    }
    el.innerHTML = runs.map((r) => {
      const sel = r.run_id === selected ? " selected" : "";
      return `<button type="button" class="hist${sel}" data-run="${esc(r.run_id)}">
        <span>${esc(r.run_id)}</span>
        <span>${esc(routeLine(r))}</span>
        <span>attempt ${esc(r.attempts)}</span>
        <span class="reason">${esc(r.reason || "")}</span>
      </button>`;
    }).join("");
  }

  function renderDetail() {
    const pane = $("detail");
    const stage = $("stage");
    const run = (snap.runs || []).find((r) => r.run_id === selected);
    if (!run) {
      pane.hidden = true;
      stage.classList.remove("has-detail");
      return;
    }
    pane.hidden = false;
    stage.classList.add("has-detail");
    $("detail-title").textContent = run.run_id;
    fillLcd($("detail-lcd"), run);
    const body = run.status === "done"
      ? `<p>${esc(run.planner || "")} · seed ${esc(run.seed)}</p>
         <p>${esc(run.reason || "done")}${run.detail ? " — " + esc(run.detail) : ""}</p>
         <pre class="trace">${esc(run.trace || "")}</pre>`
      : `<p>${esc(run.planner || "")} · seed ${esc(run.seed)}</p>
         <p>${esc(hexMap(run.map))} (${esc(run.x)},${esc(run.y)}) · frame ${esc(run.frame)}</p>
         <p>${esc(run.stop_so_far || "")}</p>
         <pre class="trace">${esc(run.trace || "")}</pre>`;
    $("detail-body").innerHTML = body;
  }

  function renderCounts() {
    const runs = snap.runs || [];
    const workers = snap.workers || [];
    $("n-running").textContent = runs.filter((r) => r.status === "running").length;
    $("n-queued").textContent = runs.filter((r) => r.status === "queued" || r.status === "leased").length;
    $("n-idle").textContent = workers.filter((w) => !w.run_id).length;
  }

  function render() {
    $("banner").hidden = !wallDown;
    $("queue-toggle").disabled = wallDown;
    $("spec-form").querySelector(".submit").disabled = wallDown;
    renderCounts();
    renderLive();
    renderWorkers();
    renderHistory();
    renderDetail();
    syncPumps();
  }

  async function refresh() {
    try {
      const res = await fetch("/v1/dashboard", { cache: "no-store" });
      if (!res.ok) throw new Error("bad");
      snap = await res.json();
      wallDown = false;
    } catch (e) {
      wallDown = true;
    }
    render();
  }

  $("queue-toggle").addEventListener("click", () => {
    const q = $("queue");
    q.hidden = !q.hidden;
    $("queue-toggle").setAttribute("aria-expanded", String(!q.hidden));
    if (!q.hidden) fillDefaults();
  });
  $("spec-form").planner.addEventListener("change", syncPlannerFields);

  $("spec-form").addEventListener("submit", async (ev) => {
    ev.preventDefault();
    const err = $("form-error");
    err.textContent = "";
    const f = ev.target;
    const planner = f.planner.value;
    const spec = {
      run_id: f.run_id.value.trim(),
      planner: planner,
      starter: f.starter.value,
      dest: planner === "scripted" ? f.dest.value.trim() : "",
      seed: Number(f.seed.value || 0),
      fps: Number(f.fps.value || 0),
      max_rounds: Number(f.max_rounds.value || 0),
      max_frames: Number(f.max_frames.value || 0)
    };
    try {
      const res = await fetch("/v1/specs", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(spec)
      });
      const body = await res.json().catch(() => ({}));
      if (res.status === 409) {
        err.textContent = "run already active";
        return;
      }
      if (!res.ok) {
        err.textContent = body.error || "could not queue";
        return;
      }
      fillDefaults();
      $("queue").hidden = true;
      $("queue-toggle").setAttribute("aria-expanded", "false");
      await refresh();
    } catch (e) {
      err.textContent = "wall unreachable";
    }
  });

  document.body.addEventListener("keydown", (ev) => {
    if (ev.key !== "Enter" && ev.key !== " ") return;
    const pick = ev.target.closest("article[data-run]");
    if (pick && ev.target === pick) {
      ev.preventDefault();
      selected = pick.getAttribute("data-run");
      render();
    }
  });

  document.body.addEventListener("click", async (ev) => {
    const cancel = ev.target.closest("[data-cancel]");
    if (cancel) {
      ev.preventDefault();
      ev.stopPropagation();
      const id = cancel.getAttribute("data-cancel");
      cardErr = "";
      try {
        const res = await fetch("/v1/runs/" + encodeURIComponent(id) + "/cancel", { method: "POST" });
        const body = await res.json().catch(() => ({}));
        if (res.status === 409) cardErr = { id, text: "already finished" };
        else if (!res.ok) cardErr = { id, text: body.error || "could not cancel" };
      } catch (e) {
        cardErr = { id, text: "wall unreachable" };
      }
      await refresh();
      return;
    }
    const pick = ev.target.closest("[data-run]");
    if (pick && !ev.target.closest("[data-cancel]")) {
      selected = pick.getAttribute("data-run");
      render();
    }
  });

  refresh();
  setInterval(refresh, pollMs);
})();
