(function () {
  const pollMs = 2000;
  const frameMs = 50; // 20 fps; a tight /frame loop burned the Chrome tab
  const slowFrameMs = 500; // phones on data: 2 fps still shows progress, 40x less traffic
  const narrow = () => window.matchMedia("(max-width: 700px)").matches;
  const short = (v) => String(v || "").slice(0, 7); // display SHAs short; JSON keeps the full one
  let snap = { now: 0, runs: [], workers: [] };
  let groups = [];
  let consoleVersion = "";
  let selected = "";
  let cardErr = "";
  let wallDown = false;
  const investigating = new Set();
  const pumps = new Map();
  const histFilter = { outcome: "", how: "", starter: "" };
  // History is paged: a farm that has run for a while has hundreds of
  // finished runs, and rendering them all at once is a long DOM and a long
  // scroll. 25 rows fills the pane without a scrollbar of its own.
  const HIST_PAGE = 25;
  let histPage = 0;

  const $ = (id) => document.getElementById(id);
  const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;"
  }[c]));
  const hexMap = (n) => "0x" + Number(n).toString(16).padStart(2, "0");
  const howLabel = (r) => r.planner === "scripted" ? "walk" : "play";
  const howText = (r) => r.planner === "scripted" ? "walk to a place" : "play the game";
  const starterOf = (r) => r.starter || "squirtle";
  const outcomeOf = (r) => (r.reason || r.status || "").toLowerCase();
  const goalOf = (r) => (r.goal || "").trim();

  function issueHref(url) {
    try {
      const u = new URL(url);
      if (u.protocol === "http:" || u.protocol === "https:") return u.href;
    } catch (e) {}
    return "";
  }
  function issueBadge(issue) {
    if (!issue || !issue.issue_number) return "";
    const href = issueHref(issue.issue_url);
    const label = `Issue #${issue.issue_number}`;
    const st = issue.status ? ` · ${issue.status}` : "";
    const n = issue.occurrence_count ? ` · ${issue.occurrence_count}×` : "";
    const fixed = issue.fixed_revision ? ` · ${short(issue.fixed_revision)}` : "";
    const stale = issue.stale ? " · stale" : "";
    if (!href) return `<span class="chip">${esc(label + st + n + fixed + stale)}</span>`;
    return `<a class="issue-a" href="${esc(href)}" target="_blank" rel="noopener">${esc(label)}</a><span class="chip">${esc((issue.status || "") + n + fixed + stale)}</span>`;
  }

  // The same tally the runner's watch page renders, compressed to one card
  // line. Nil on scripted runs and before the first ask.
  function statsLine(r) {
    const s = r.stats;
    if (!s || r.planner === "scripted") return "";
    return `round ${s.round} (${s.rounds_left} left) · rep ${s.repeats}/${s.rounds} · think ${s.avg_seconds.toFixed(1)}s avg`;
  }

  // Decomp map constants, same inventory as red/state/map_names.go.
  const MAP_NAMES = {
    0x00: "PALLET_TOWN", 0x01: "VIRIDIAN_CITY", 0x02: "PEWTER_CITY",
    0x03: "CERULEAN_CITY", 0x04: "LAVENDER_TOWN", 0x05: "VERMILION_CITY",
    0x06: "CELADON_CITY", 0x07: "FUCHSIA_CITY", 0x08: "CINNABAR_ISLAND",
    0x09: "INDIGO_PLATEAU", 0x0A: "SAFFRON_CITY", 0x0C: "ROUTE_1",
    0x0D: "ROUTE_2", 0x0E: "ROUTE_3", 0x0F: "ROUTE_4", 0x10: "ROUTE_5",
    0x11: "ROUTE_6", 0x12: "ROUTE_7", 0x13: "ROUTE_8", 0x14: "ROUTE_9",
    0x15: "ROUTE_10", 0x16: "ROUTE_11", 0x17: "ROUTE_12", 0x18: "ROUTE_13",
    0x19: "ROUTE_14", 0x1A: "ROUTE_15", 0x1B: "ROUTE_16", 0x1C: "ROUTE_17",
    0x1D: "ROUTE_18", 0x1E: "ROUTE_19", 0x1F: "ROUTE_20", 0x20: "ROUTE_21",
    0x21: "ROUTE_22", 0x22: "ROUTE_23", 0x23: "ROUTE_24", 0x24: "ROUTE_25",
    0x25: "REDS_HOUSE_1F", 0x26: "REDS_HOUSE_2F", 0x27: "BLUES_HOUSE",
    0x28: "OAKS_LAB", 0x29: "VIRIDIAN_POKECENTER", 0x2A: "VIRIDIAN_MART",
    0x2B: "VIRIDIAN_SCHOOL_HOUSE", 0x2C: "VIRIDIAN_NICKNAME_HOUSE",
    0x2D: "VIRIDIAN_GYM", 0x2E: "DIGLETTS_CAVE_ROUTE_2",
    0x2F: "VIRIDIAN_FOREST_NORTH_GATE", 0x30: "ROUTE_2_TRADE_HOUSE",
    0x31: "ROUTE_2_GATE", 0x32: "VIRIDIAN_FOREST_SOUTH_GATE",
    0x33: "VIRIDIAN_FOREST", 0x34: "MUSEUM_1F", 0x35: "MUSEUM_2F",
    0x36: "PEWTER_GYM", 0x37: "PEWTER_NIDORAN_HOUSE", 0x38: "PEWTER_MART",
    0x39: "PEWTER_SPEECH_HOUSE", 0x3A: "PEWTER_POKECENTER", 0x3B: "MT_MOON_1F",
    0x3C: "MT_MOON_B1F", 0x3D: "MT_MOON_B2F", 0x3E: "CERULEAN_TRASHED_HOUSE",
    0x3F: "CERULEAN_TRADE_HOUSE", 0x40: "CERULEAN_POKECENTER", 0x41: "CERULEAN_GYM",
    0x42: "BIKE_SHOP", 0x43: "CERULEAN_MART", 0x44: "MT_MOON_POKECENTER",
    0x46: "ROUTE_5_GATE", 0x47: "UNDERGROUND_PATH_ROUTE_5", 0x48: "DAYCARE",
    0x49: "ROUTE_6_GATE", 0x4A: "UNDERGROUND_PATH_ROUTE_6", 0x4C: "ROUTE_7_GATE",
    0x4D: "UNDERGROUND_PATH_ROUTE_7", 0x4F: "ROUTE_8_GATE",
    0x50: "UNDERGROUND_PATH_ROUTE_8", 0x51: "ROCK_TUNNEL_POKECENTER",
    0x52: "ROCK_TUNNEL_1F", 0x53: "POWER_PLANT", 0x54: "ROUTE_11_GATE_1F",
    0x55: "DIGLETTS_CAVE_ROUTE_11", 0x56: "ROUTE_11_GATE_2F",
    0x57: "ROUTE_12_GATE_1F", 0x58: "BILLS_HOUSE", 0x59: "VERMILION_POKECENTER",
    0x5A: "POKEMON_FAN_CLUB", 0x5B: "VERMILION_MART", 0x5C: "VERMILION_GYM",
    0x5D: "VERMILION_PIDGEY_HOUSE", 0x5E: "VERMILION_DOCK", 0x5F: "SS_ANNE_1F",
    0x60: "SS_ANNE_2F", 0x61: "SS_ANNE_3F", 0x62: "SS_ANNE_B1F",
    0x63: "SS_ANNE_BOW", 0x64: "SS_ANNE_KITCHEN", 0x65: "SS_ANNE_CAPTAINS_ROOM",
    0x66: "SS_ANNE_1F_ROOMS", 0x67: "SS_ANNE_2F_ROOMS", 0x68: "SS_ANNE_B1F_ROOMS",
    0x6C: "VICTORY_ROAD_1F", 0x71: "LANCES_ROOM", 0x76: "HALL_OF_FAME",
    0x77: "UNDERGROUND_PATH_NORTH_SOUTH", 0x78: "CHAMPIONS_ROOM",
    0x79: "UNDERGROUND_PATH_WEST_EAST", 0x7A: "CELADON_MART_1F",
    0x7B: "CELADON_MART_2F", 0x7C: "CELADON_MART_3F", 0x7D: "CELADON_MART_4F",
    0x7E: "CELADON_MART_ROOF", 0x7F: "CELADON_MART_ELEVATOR",
    0x80: "CELADON_MANSION_1F", 0x81: "CELADON_MANSION_2F",
    0x82: "CELADON_MANSION_3F", 0x83: "CELADON_MANSION_ROOF",
    0x84: "CELADON_MANSION_ROOF_HOUSE", 0x85: "CELADON_POKECENTER",
    0x86: "CELADON_GYM", 0x87: "GAME_CORNER", 0x88: "CELADON_MART_5F",
    0x89: "GAME_CORNER_PRIZE_ROOM", 0x8A: "CELADON_DINER",
    0x8B: "CELADON_CHIEF_HOUSE", 0x8C: "CELADON_HOTEL", 0x8D: "LAVENDER_POKECENTER",
    0x8E: "POKEMON_TOWER_1F", 0x8F: "POKEMON_TOWER_2F", 0x90: "POKEMON_TOWER_3F",
    0x91: "POKEMON_TOWER_4F", 0x92: "POKEMON_TOWER_5F", 0x93: "POKEMON_TOWER_6F",
    0x94: "POKEMON_TOWER_7F", 0x95: "MR_FUJIS_HOUSE", 0x96: "LAVENDER_MART",
    0x97: "LAVENDER_CUBONE_HOUSE", 0x98: "FUCHSIA_MART",
    0x99: "FUCHSIA_BILLS_GRANDPAS_HOUSE", 0x9A: "FUCHSIA_POKECENTER",
    0x9B: "WARDENS_HOUSE", 0x9C: "SAFARI_ZONE_GATE", 0x9D: "FUCHSIA_GYM",
    0x9E: "FUCHSIA_MEETING_ROOM", 0x9F: "SEAFOAM_ISLANDS_B1F",
    0xA0: "SEAFOAM_ISLANDS_B2F", 0xA1: "SEAFOAM_ISLANDS_B3F",
    0xA2: "SEAFOAM_ISLANDS_B4F", 0xA3: "VERMILION_OLD_ROD_HOUSE",
    0xA4: "FUCHSIA_GOOD_ROD_HOUSE", 0xA5: "POKEMON_MANSION_1F",
    0xA6: "CINNABAR_GYM", 0xA7: "CINNABAR_LAB", 0xA8: "CINNABAR_LAB_TRADE_ROOM",
    0xA9: "CINNABAR_LAB_METRONOME_ROOM", 0xAA: "CINNABAR_LAB_FOSSIL_ROOM",
    0xAB: "CINNABAR_POKECENTER", 0xAC: "CINNABAR_MART",
    0xAE: "INDIGO_PLATEAU_LOBBY", 0xAF: "COPYCATS_HOUSE_1F",
    0xB0: "COPYCATS_HOUSE_2F", 0xB1: "FIGHTING_DOJO", 0xB2: "SAFFRON_GYM",
    0xB3: "SAFFRON_PIDGEY_HOUSE", 0xB4: "SAFFRON_MART", 0xB5: "SILPH_CO_1F",
    0xB6: "SAFFRON_POKECENTER", 0xB7: "MR_PSYCHICS_HOUSE",
    0xB8: "ROUTE_15_GATE_1F", 0xB9: "ROUTE_15_GATE_2F", 0xBA: "ROUTE_16_GATE_1F",
    0xBB: "ROUTE_16_GATE_2F", 0xBC: "ROUTE_16_FLY_HOUSE",
    0xBD: "ROUTE_12_SUPER_ROD_HOUSE", 0xBE: "ROUTE_18_GATE_1F",
    0xBF: "ROUTE_18_GATE_2F", 0xC0: "SEAFOAM_ISLANDS_1F", 0xC1: "ROUTE_22_GATE",
    0xC2: "VICTORY_ROAD_2F", 0xC3: "ROUTE_12_GATE_2F", 0xC4: "VERMILION_TRADE_HOUSE",
    0xC5: "DIGLETTS_CAVE", 0xC6: "VICTORY_ROAD_3F", 0xC7: "ROCKET_HIDEOUT_B1F",
    0xC8: "ROCKET_HIDEOUT_B2F", 0xC9: "ROCKET_HIDEOUT_B3F",
    0xCA: "ROCKET_HIDEOUT_B4F", 0xCB: "ROCKET_HIDEOUT_ELEVATOR",
    0xCF: "SILPH_CO_2F", 0xD0: "SILPH_CO_3F", 0xD1: "SILPH_CO_4F",
    0xD2: "SILPH_CO_5F", 0xD3: "SILPH_CO_6F", 0xD4: "SILPH_CO_7F",
    0xD5: "SILPH_CO_8F", 0xD6: "POKEMON_MANSION_2F", 0xD7: "POKEMON_MANSION_3F",
    0xD8: "POKEMON_MANSION_B1F", 0xD9: "SAFARI_ZONE_EAST", 0xDA: "SAFARI_ZONE_NORTH",
    0xDB: "SAFARI_ZONE_WEST", 0xDC: "SAFARI_ZONE_CENTER",
    0xDD: "SAFARI_ZONE_CENTER_REST_HOUSE", 0xDE: "SAFARI_ZONE_SECRET_HOUSE",
    0xDF: "SAFARI_ZONE_WEST_REST_HOUSE", 0xE0: "SAFARI_ZONE_EAST_REST_HOUSE",
    0xE1: "SAFARI_ZONE_NORTH_REST_HOUSE", 0xE2: "CERULEAN_CAVE_2F",
    0xE3: "CERULEAN_CAVE_B1F", 0xE4: "CERULEAN_CAVE_1F", 0xE5: "NAME_RATERS_HOUSE",
    0xE6: "CERULEAN_BADGE_HOUSE", 0xE8: "ROCK_TUNNEL_B1F", 0xE9: "SILPH_CO_9F",
    0xEA: "SILPH_CO_10F", 0xEB: "SILPH_CO_11F", 0xEC: "SILPH_CO_ELEVATOR",
    0xEF: "TRADE_CENTER", 0xF0: "COLOSSEUM", 0xF5: "LORELEIS_ROOM",
    0xF6: "BRUNOS_ROOM", 0xF7: "AGATHAS_ROOM"
  };

  function titleMap(raw) {
    if (!raw) return "";
    return raw.toLowerCase().replace(/_/g, " ").replace(/\b[a-z]/g, (c) => c.toUpperCase())
      .replace(/\bSs\b/g, "S.S.")
      .replace(/\bMt\b/g, "Mt.")
      .replace(/\bPokecenter\b/g, "Pokécenter")
      .replace(/\bPokemon\b/g, "Pokémon");
  }
  function mapLabel(n) {
    const name = titleMap(MAP_NAMES[Number(n)]);
    const hex = hexMap(n);
    return name ? name + " " + hex : hex;
  }
  function tileLabel(r) {
    return mapLabel(r.map) + " (" + r.x + "," + r.y + ")";
  }
  function chip(kind, text) {
    return `<span class="chip ${kind}">${esc(text)}</span>`;
  }
  function settingChips(r) {
    let html = chip("starter", starterOf(r))
      + chip("how", howLabel(r))
      + chip("seed", "seed " + r.seed);
    if (goalOf(r)) html += chip("goal", goalOf(r));
    if (r.endless) html += chip("loop", r.random_seed ? "endless random" : "endless");
    return html;
  }
  function fmtWhen(unix) {
    const n = Number(unix);
    if (!n) return "";
    const now = snap.now || Math.floor(Date.now() / 1000);
    const sec = Math.max(0, now - n);
    let rel = "just now";
    if (sec >= 86400) rel = Math.floor(sec / 86400) + "d ago";
    else if (sec >= 3600) rel = Math.floor(sec / 3600) + "h ago";
    else if (sec >= 60) rel = Math.floor(sec / 60) + "m ago";
    else if (sec >= 5) rel = sec + "s ago";
    const d = new Date(n * 1000);
    const clock = d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
    return clock + " " + rel;
  }
  function runWhen(r) {
    return fmtWhen(r.ended_at || r.queued_at);
  }
  function statusChip(r) {
    const out = outcomeOf(r);
    if (r.status === "done") return chip("outcome-" + out, out || "done");
    return chip(r.status, r.status);
  }
  function kv(rows) {
    const body = rows.filter((row) => row[1] !== "" && row[1] != null)
      .map(([k, v]) => `<dt>${esc(k)}</dt><dd>${esc(v)}</dd>`).join("");
    return body ? `<dl class="kv">${body}</dl>` : "";
  }

  function newRunId() {
    const n = new Uint32Array(2);
    crypto.getRandomValues(n);
    return "run-" + n[0].toString(36) + n[1].toString(36);
  }
  function syncPlannerFields() {
    const f = $("spec-form");
    const scripted = f.planner.value === "scripted";
    document.querySelectorAll(".scripted-only").forEach((el) => { el.hidden = !scripted; });
    document.querySelectorAll(".llm-only").forEach((el) => { el.hidden = scripted; });
    document.querySelectorAll(".endless-only").forEach((el) => { el.hidden = !f.endless.checked; });
  }
  function fillDefaults() {
    const f = $("spec-form");
    f.run_id.value = newRunId();
    f.planner.value = "llm";
    f.starter.value = "squirtle";
    f.dest.value = "viridian pokemon center";
    f.goal.value = "Earn the Boulder Badge.";
    f.seed.value = "0";
    f.fps.value = "60";
    f.max_rounds.value = "32";
    f.max_frames.value = "0";
    f.endless.checked = false;
    f.seed_mode.value = "random";
    syncPlannerFields();
  }

  function liveRuns() { return (snap.runs || []).filter((r) => r.status !== "done"); }
  function doneRuns() { return (snap.runs || []).filter((r) => r.status === "done"); }
  function filteredHistory() {
    return doneRuns().filter((r) => {
      if (histFilter.outcome && outcomeOf(r) !== histFilter.outcome) return false;
      if (histFilter.how && howLabel(r) !== histFilter.how) return false;
      if (histFilter.starter && starterOf(r) !== histFilter.starter) return false;
      return true;
    });
  }

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
      const tick = narrow() ? slowFrameMs : frameMs; // decided per pump start
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
        const wait = tick - (Date.now() - started);
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
            <div class="chips status-chips"></div>
            <div class="rid"></div>
            <div class="chips setting-chips"></div>
            <div class="facts">
              <div class="fact-k">now</div>
              <div class="pos"></div>
              <div class="fact-k">progress</div>
              <div class="stats"></div>
              <div class="fact-k">llm</div>
              <div class="llm"></div>
            </div>
            <button type="button" class="cancel"></button>
            <div class="card-err" hidden></div>
          </div>`;
        el.appendChild(art);
      }
      art.classList.toggle("selected", r.run_id === selected);
      fillLcd(art.querySelector(".lcd"), r);
      art.querySelector(".status-chips").innerHTML = statusChip(r);
      art.querySelector(".rid").textContent = r.run_id;
      art.querySelector(".setting-chips").innerHTML = settingChips(r);
      const waiting = r.status === "queued" || r.status === "leased";
      if (waiting && r.planner === "scripted" && r.dest) {
        art.querySelector(".pos").textContent = "to " + r.dest;
        art.querySelector(".stats").textContent = "waiting for a worker";
      } else if (waiting) {
        art.querySelector(".pos").textContent = "waiting for a worker";
        art.querySelector(".stats").textContent = "not started";
      } else {
        art.querySelector(".pos").textContent = tileLabel(r);
        art.querySelector(".stats").textContent = "frame " + r.frame + " · attempt " + r.attempts;
        const line = statsLine(r);
        art.querySelector(".llm").textContent = line;
      }
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
    for (const r of runs) {
      const art = el.querySelector('article[data-run="' + CSS.escape(r.run_id) + '"]');
      if (art) el.appendChild(art);
    }
  }

  function renderWorkers() {
    const ws = snap.workers || [];
    const el = $("workers");
    if (!ws.length) {
      el.innerHTML = `<p class="empty">No workers</p>`;
      return;
    }
    const byVer = {};
    for (const w of ws) {
      const v = w.version || "unknown";
      byVer[v] = (byVer[v] || 0) + 1;
    }
    const summary = Object.entries(byVer)
      .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
      .map(([v, n]) => `${n} × ${short(v)}`)
      .join(", ");
    el.innerHTML = `<p class="ver-summary">${esc(summary)}</p>` + ws.map((w) => {
      const busy = Boolean(w.run_id);
      const job = busy ? `on <b>${esc(w.run_id)}</b>` : "waiting for a lease";
      return `<div class="worker">
        ${chip(busy ? "busy" : "idle", busy ? "busy" : "idle")}
        <span class="addr">${esc(w.addr)}</span>
        ${w.version ? `<span class="ver">${esc(short(w.version))}</span>` : ""}
        <span class="job">${job}</span>
        <span class="ago">${esc(w.seen_ago)} ago</span>
      </div>`;
    }).join("");
  }

  function filterBtn(group, value, label) {
    const on = histFilter[group] === value;
    return `<button type="button" class="filter" data-filter-group="${group}" data-filter-value="${esc(value)}" aria-pressed="${on}">${esc(label)}</button>`;
  }
  function renderHistFilters() {
    const runs = doneRuns();
    const outcomes = [...new Set(runs.map(outcomeOf).filter(Boolean))].sort();
    const hows = [...new Set(runs.map(howLabel))];
    const starters = [...new Set(runs.map(starterOf))];
    const el = $("hist-filters");
    if (!runs.length) {
      el.innerHTML = "";
      return;
    }
    let html = `<div class="filter-group"><span>ended</span>${filterBtn("outcome", "", "all")}`;
    for (const o of outcomes) html += filterBtn("outcome", o, o);
    html += `</div><div class="filter-group"><span>how</span>${filterBtn("how", "", "all")}`;
    for (const h of hows) html += filterBtn("how", h, h);
    html += `</div><div class="filter-group"><span>starter</span>${filterBtn("starter", "", "all")}`;
    for (const s of starters) html += filterBtn("starter", s, s);
    html += `</div>`;
    el.innerHTML = html;
  }

  function renderHistory() {
    renderHistFilters();
    const runs = filteredHistory();
    const el = $("history");
    if (!doneRuns().length) {
      el.innerHTML = `<p class="empty">Nothing finished yet</p>`;
      $("hist-pager").innerHTML = "";
      return;
    }
    if (!runs.length) {
      el.innerHTML = `<p class="empty">No runs match these filters</p>`;
      $("hist-pager").innerHTML = "";
      return;
    }
    const pages = Math.max(1, Math.ceil(runs.length / HIST_PAGE));
    histPage = Math.min(histPage, pages - 1);
    const start = histPage * HIST_PAGE;
    el.innerHTML = runs.slice(start, start + HIST_PAGE).map((r) => {
      const sel = r.run_id === selected ? " selected" : "";
      const where = r.planner === "scripted" && r.dest
        ? r.dest : tileLabel(r);
      const out = r.detail || r.reason || "done";
      return `<div class="hist-row${sel}">
        <button type="button" class="hist" data-run="${esc(r.run_id)}">
        <span class="hist-when">${esc(runWhen(r) || "—")}</span>
        <span class="hist-id">${esc(r.run_id)}</span>
        <span class="chips">${settingChips(r)}</span>
        <span class="hist-where">${esc(where)}</span>
        <span class="hist-out">${statusChip(r)}${issueBadge(r.issue)}<span class="hist-outcome">${esc(out)}</span></span>
        </button>
        <button type="button" class="hist-del" data-delete="${esc(r.run_id)}">Delete</button>
      </div>`;
    }).join("");
    renderHistPager(runs.length, pages);
  }

  function renderHistPager(total, pages) {
    const el = $("hist-pager");
    const start = histPage * HIST_PAGE + 1;
    const end = Math.min(total, (histPage + 1) * HIST_PAGE);
    el.innerHTML =
      `<span class="pager-count">${start}\u2013${end} of ${total}</span>` +
      `<button type="button" class="pager-btn" data-page="prev" ${histPage === 0 ? "disabled" : ""}>\u2190 prev</button>` +
      `<span class="pager-page">${histPage + 1} / ${pages}</span>` +
      `<button type="button" class="pager-btn" data-page="next" ${histPage >= pages - 1 ? "disabled" : ""}>next \u2192</button>`;
  }

  function renderDetail() {
    const pane = $("watch");
    const run = (snap.runs || []).find((r) => r.run_id === selected);
    if (!run) {
      pane.hidden = true;
      return;
    }
    pane.hidden = false;
    $("detail-title").textContent = run.run_id;
    $("detail-chips").innerHTML = statusChip(run) + settingChips(run) + issueBadge(run.issue);
    fillLcd($("detail-lcd"), run);
    const settings = kv([
      ["how", howText(run)],
      ["starter", starterOf(run)],
      ["goal", goalOf(run)],
      ["walk to", run.planner === "scripted" ? (run.dest || "—") : ""],
      ["seed", String(run.seed)],
      ["keep going", run.endless ? (run.random_seed ? "yes, random seed" : "yes, same seed") : ""],
      ["queued", fmtWhen(run.queued_at)],
      ["ended", fmtWhen(run.ended_at)],
      ["fps", run.fps ? String(run.fps) : ""],
      ["max rounds", run.max_rounds ? String(run.max_rounds) : ""],
      ["max frames", run.max_frames ? String(run.max_frames) : ""]
    ]);
    const stateRows = run.status === "done"
      ? [
          ["ended", run.reason || "done"],
          ["detail", run.detail || ""],
          ["last map", tileLabel(run)],
          ["frame", String(run.frame)],
          ["attempts", String(run.attempts)]
        ]
      : [
          ["status", run.status],
          ["map", tileLabel(run)],
          ["frame", String(run.frame)],
          ["attempt", String(run.attempts)],
          ["so far", run.stop_so_far || ""]
        ];
    $("detail-body").innerHTML =
      `<div class="block"><h3>Settings</h3>${settings}</div>`
      + `<div class="block"><h3>${run.status === "done" ? "Outcome" : "Now"}</h3>${kv(stateRows)}</div>`
      + planHTML(run)
      + playHTML(run)
      + lastEventHTML(run);
  }

  function planHTML(run) {
    const play = run.planner !== "scripted";
    if (!play && !run.question && !run.decision) return "";
    const question = run.question
      ? `<pre class="plan-q">${esc(run.question)}</pre>`
      : `<p class="plan-wait">waiting for the first plan</p>`;
    let decision = "";
    if (run.decision) {
      decision = `<div class="plan-k">decision</div><p class="plan-d">${esc(run.decision)}</p>`;
    } else if (run.question) {
      decision = `<div class="plan-k">decision</div><p class="plan-wait">waiting for reply</p>`;
    }
    return `<div class="block"><h3>Plan</h3>
      <div class="plan-k">question</div>${question}
      ${decision}</div>`;
  }

  // The full Play panel, mirroring the runner's watch page: one row per
  // number, amber when it is the warning (repeat picks, rejected replies,
  // transport errors, fallbacks), plus the last intent and the choice bars.
  function playHTML(run) {
    const s = run.stats;
    if (!s || run.planner === "scripted") return "";
    const row = (k, v, warn) =>
      `<div class="prow"><span>${esc(k)}</span><span${warn ? ' class="pwarn"' : ""}>${esc(v)}</span></div>`;
    const nums =
      row("round", s.round + (s.rounds_left ? ` (${s.rounds_left} left)` : "")) +
      row("repeat picks", `${s.repeats} of ${s.rounds}`, s.rounds > 3 && s.repeats * 2 >= s.rounds) +
      row("think", `${s.last_seconds.toFixed(1)}s / ${s.avg_seconds.toFixed(1)}s avg`) +
      row("offered", `${s.avg_offered.toFixed(1)} avg`) +
      row("tokens", `${s.prompt_tokens} / ${s.completion_tokens}`) +
      row("rejected", String(s.rejected), s.rejected > 0) +
      row("transport", String(s.transport), s.transport > 0) +
      row("fallbacks", String(s.fallbacks), s.fallbacks > 0);
    const intent = s.intent ? `<p class="pintent">"${esc(s.intent)}" (${s.intent_age} rounds)</p>` : "";
    const top = (s.choices && s.choices[0]) ? s.choices[0].count : 1;
    const choices = (s.choices || []).map((c) =>
      `<div class="pchoice"><div class="pbar" style="width:${(100 * c.count) / top}%"></div>` +
      `<span>${esc(c.objective)}</span><span class="n">${c.count}</span></div>`).join("");
    return `<div class="block"><h3>Play</h3><div class="pnums">${nums}</div>${intent}<div class="pchoices">${choices}</div></div>`;
  }

  function lastEventHTML(run) {
    if (!run.trace) return "";
    const title = run.question || run.decision ? "Last event" : "Trace";
    return `<div class="block"><h3>${title}</h3><pre class="trace">${esc(run.trace)}</pre></div>`;
  }

  function renderCounts() {
    const runs = snap.runs || [];
    const workers = snap.workers || [];
    $("n-running").textContent = runs.filter((r) => r.status === "running").length;
    $("n-queued").textContent = runs.filter((r) => r.status === "queued" || r.status === "leased").length;
    $("n-idle").textContent = workers.filter((w) => !w.run_id).length;
  }

  function renderVersions() {
    const wall = snap.wall_version || "";
    $("versions").textContent =
      ["console", short(consoleVersion), "wall", short(wall)].filter(Boolean).join(" · ");
  }

  function selectRun(id) {
    selected = id;
    render();
    if (id && narrow()) $("watch").scrollIntoView({ behavior: "smooth", block: "start" });
  }

  function renderFailures() {
    const el = $("failures");
    if (!el) return;
    const active = (groups || []).filter((g) => {
      const st = g.issue && g.issue.status;
      const res = g.issue && g.issue.resolution;
      return st !== "resolved" && st !== "fixed" && res !== "fixed";
    });
    if (!active.length) {
      el.innerHTML = `<p class="empty">No open failure groups</p>`;
      return;
    }
    el.innerHTML = active.map((g) => {
      const issue = g.issue;
      let action = "";
      if (!issue || !issue.issue_id) {
        if (g.outbox === "error") action = `<span class="chip">report failed</span>`;
        else if (g.outbox === "pending") action = `<span class="chip">pending report</span>`;
      } else if (issue.status === "open" || issue.status === "diagnosed") {
        const busy = investigating.has(g.key);
        action = `<button type="button" class="fail-act" data-investigate="${esc(g.key)}" ${busy ? "disabled" : ""}>Investigate now</button>`;
      }
      return `<div class="fail-card">
        <div class="fail-pat">${esc(g.pattern)}</div>
        <div class="fail-ex">${esc(g.example || "")} · ${g.count} run${g.count === 1 ? "" : "s"}</div>
        <div class="fail-meta">${issueBadge(issue)}${action}</div>
      </div>`;
    }).join("");
  }

  function render() {
    $("banner").hidden = !wallDown;
    $("queue-toggle").disabled = wallDown;
    $("spec-form").querySelector(".submit").disabled = wallDown;
    renderCounts();
    renderVersions();
    renderLive();
    renderFailures();
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
      try {
        const tr = await fetch("/v1/triage", { cache: "no-store" });
        if (tr.ok) groups = await tr.json();
      } catch (e) {
        groups = groups || [];
      }
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
  $("spec-form").endless.addEventListener("change", syncPlannerFields);
  $("detail-close").addEventListener("click", () => {
    selected = "";
    render();
  });

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
      goal: planner === "llm" ? f.goal.value.trim() : "",
      seed: Number(f.seed.value || 0),
      fps: Number(f.fps.value || 0),
      max_rounds: Number(f.max_rounds.value || 0),
      max_frames: Number(f.max_frames.value || 0),
      endless: f.endless.checked,
      random_seed: f.endless.checked && f.seed_mode.value === "random"
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
      selectRun(pick.getAttribute("data-run"));
    }
  });

  document.body.addEventListener("click", async (ev) => {
    const filt = ev.target.closest("[data-filter-group]");
    if (filt) {
      ev.preventDefault();
      const group = filt.getAttribute("data-filter-group");
      histFilter[group] = filt.getAttribute("data-filter-value") || "";
      histPage = 0;
      renderHistory();
      return;
    }
    const page = ev.target.closest("[data-page]");
    if (page) {
      ev.preventDefault();
      if (page.getAttribute("data-page") === "prev") histPage = Math.max(0, histPage - 1);
      else histPage += 1;
      renderHistory();
      return;
    }
    const del = ev.target.closest("[data-delete]");
    if (del) {
      ev.preventDefault();
      ev.stopPropagation();
      const id = del.getAttribute("data-delete");
      try {
        const res = await fetch("/v1/runs/" + encodeURIComponent(id), { method: "DELETE" });
        const body = await res.json().catch(() => ({}));
        if (!res.ok) cardErr = { id, text: body.error || "could not delete" };
        else if (selected === id) selected = "";
      } catch (e) {
        cardErr = { id, text: "wall unreachable" };
      }
      await refresh();
      return;
    }
    const inv = ev.target.closest("[data-investigate]");
    if (inv) {
      ev.preventDefault();
      ev.stopPropagation();
      const key = inv.getAttribute("data-investigate");
      if (!key || investigating.has(key)) return;
      investigating.add(key);
      inv.disabled = true;
      try {
        await fetch("/v1/triage/" + encodeURIComponent(key) + "/investigate", { method: "POST" });
      } catch (e) {}
      investigating.delete(key);
      await refresh();
      return;
    }
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
      selectRun(pick.getAttribute("data-run"));
    }
  });

  fetch("/v1/version", { cache: "no-store" })
    .then((r) => (r.ok ? r.json() : null))
    .then((v) => {
      if (v && v.version) {
        consoleVersion = v.version;
        renderVersions();
      }
    })
    .catch(() => {}); // version is cosmetic; never block the console on it
  refresh();
  setInterval(refresh, pollMs);
})();
