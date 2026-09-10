"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const behavior = require("./behavior.js");
const {
  partitionOperations,
  timelineLayout,
  replayPresentation,
  normalizePlaybackRate,
  readStoredPlaybackRate,
  writeStoredPlaybackRate,
  drawerTransition,
  applyTabView,
  wireTabNavigation,
} = behavior;

class FakeTarget {
  constructor(id = "") {
    this.id = id;
    this.listeners = new Map();
    this.dataset = {};
    this.style = { setProperty(name, value) { this[name] = String(value); } };
    this.hidden = false;
    this.disabled = false;
    this.textContent = "";
    this.innerHTML = "";
    this.tabIndex = 0;
    this.duration = 0;
    this.currentTime = 0;
    this.parent = null;
    this.attributes = new Map();
    this.classList = { contains: () => false };
  }
  addEventListener(type, listener, options = {}) {
    const entries = this.listeners.get(type) || [];
    entries.push({ listener, once: Boolean(options.once) });
    this.listeners.set(type, entries);
  }
  removeEventListener(type, listener) {
    this.listeners.set(type, (this.listeners.get(type) || []).filter((entry) => entry.listener !== listener));
  }
  emit(type, properties = {}, target = this) {
    const event = {
      type,
      target,
      currentTarget: this,
      defaultPrevented: false,
      preventDefault() { this.defaultPrevented = true; },
      ...properties,
    };
    this.#deliver(event);
    return event;
  }
  #deliver(event) {
    const entries = [...(this.listeners.get(event.type) || [])];
    for (const entry of entries) {
      entry.listener.call(this, event);
      if (entry.once) this.removeEventListener(event.type, entry.listener);
    }
    if (this.parent) this.parent.#deliver(event);
  }
  dispatchEvent(event) {
    this.emit(event.type, event, this);
    return true;
  }
  setAttribute(name, value) { this.attributes.set(name, String(value)); }
  getAttribute(name) { return this.attributes.get(name) || null; }
  removeAttribute(name) {
    this.attributes.delete(name);
    if (name === "src") this.src = "";
  }
  insertAdjacentHTML() {}
  pause() { this.paused = true; }
  load() { this.loadCount = (this.loadCount || 0) + 1; }
  focus() { this.focused = true; }
  scrollIntoView() {}
  querySelector(selector) {
    if (selector.startsWith('[data-event-index="')) return new FakeTarget("story-event");
    return null;
  }
  closest(selector) {
    if (selector === "[data-event-index]" && this.dataset.eventIndex !== undefined) return this;
    if (selector === "article[data-run]" && this.dataset.run !== undefined) return this;
    return null;
  }
  matches(selector) { return selector === ".story-entry" && this.storyEntry === true; }
}

function flushAsync() {
  return new Promise((resolve) => setImmediate(resolve));
}

function createInspectorHarness() {
  const ids = [
    "run-inspector", "detail-game-media", "detail-lcd", "pp-timeline-track", "pp-story",
    "pp-replay", "pp-replay-status", "pp-replay-panel", "pp-transport-status",
    "pp-return-live", "pp-game-video", "evidence-drawer", "pp-meta", "pp-debug",
    "pp-artifacts", "pp-art-table", "pp-art-empty", "pp-investigate",
    "pp-investigate-status", "pp-story-actions", "pp-timeline-action",
    "pp-timeline-selection", "pp-close-evidence",
  ];
  const elements = Object.fromEntries(ids.map((id) => [id, new FakeTarget(id)]));
  const window = new FakeTarget("window");
  window.PokeConsoleBehavior = behavior;
  const documentElement = new FakeTarget("html");
  const document = {
    documentElement,
    getElementById: (id) => elements[id] || null,
  };
  const timers = [];
  const requests = [];
  const semanticEvents = [];
  const replayStatuses = [{ state: "missing" }, { state: "ready", size: 42 }];
  const runViews = {
    finished: {
      run: { status: "done", frame: 100 },
      finish: { reason: "done" },
      timeline: [
        { frame: 10, decision: "first" },
        { frame: 80, decision: "latest" },
      ],
    },
    active: {
      run: { status: "running", frame: 25 },
      timeline: [
        { frame: 5, decision: "active first" },
        { frame: 20, decision: "active latest" },
      ],
    },
  };
  let selectedRun = "";
  const fetch = async (url, options = {}) => {
    requests.push({ url, method: options.method || "GET" });
    const run = url.includes("/active/") ? "active" : "finished";
    let body;
    if (url.endsWith("/debug")) body = runViews[run];
    else if (url.endsWith("/artifacts")) body = {
      artifacts: run === "finished" ? [{ name: "run.gbrun", replayable: true }] : [],
    };
    else if (url.endsWith("/checkpoints")) body = { checkpoints: [] };
    else if (url.endsWith("/repro-source")) body = null;
    else if (url.endsWith("/replay/status")) body = replayStatuses.shift() || { state: "ready", size: 42 };
    else if (url.endsWith("/replay/render")) body = { state: "generating", progress: "rendering" };
    else throw new Error(`unexpected request: ${url}`);
    return { ok: true, status: 200, statusText: "OK", json: async () => body };
  };
  window.addEventListener("pokefarm-semantic-event", (event) => semanticEvents.push(event.detail));
  const context = {
    window,
    document,
    fetch,
    console,
    encodeURIComponent,
    URL,
    CustomEvent: class {
      constructor(type, options = {}) { this.type = type; this.detail = options.detail; }
    },
    setTimeout: (callback) => { timers.push(callback); return timers.length; },
    clearTimeout: () => {},
  };
  vm.runInNewContext(
    fs.readFileSync(path.join(__dirname, "inspector.js"), "utf8"),
    context,
    { filename: "inspector.js" },
  );
  return {
    elements,
    requests,
    semanticEvents,
    timers,
    select(runID) {
      selectedRun = runID;
      window.emit("pokefarm-select-run", { detail: { runId: runID } });
    },
    get selectedRun() { return selectedRun; },
  };
}

test("operations partition queued, active, and recent attempts", () => {
  const runs = [
    { run_id: "done-old", status: "done", ended_at: 10 },
    { run_id: "active", status: "running", queued_at: 30 },
    { run_id: "leased", status: "leased", queued_at: 20 },
    { run_id: "queued", status: "queued", queued_at: 10 },
    { run_id: "done-new", status: "done", ended_at: 40 },
  ];

  const groups = partitionOperations(runs, 1);

  assert.deepEqual(groups.waiting.map((run) => run.run_id), ["queued", "leased"]);
  assert.deepEqual(groups.active.map((run) => run.run_id), ["active"]);
  assert.deepEqual(groups.recent.map((run) => run.run_id), ["done-new"]);
});

test("timeline layout uses run frames and separates colliding markers into lanes", () => {
  const layout = timelineLayout([
    { frame: 0 },
    { frame: 25 },
    { frame: 25 },
    { frame: 100 },
  ], 100);

  assert.deepEqual(layout.positions, [0, 25, 25, 100]);
  assert.deepEqual(layout.lanes, [0, 0, 1, 0]);
  assert.equal(layout.laneCount, 2);
  assert.equal(layout.totalFrames, 100);
});

test("ready replay preserves LCD until media can play and restores it on error", () => {
  assert.deepEqual(replayPresentation("loading"), {
    lcdHidden: false,
    videoHidden: true,
    panelHidden: true,
    retry: false,
  });
  assert.deepEqual(replayPresentation("playing"), {
    lcdHidden: true,
    videoHidden: false,
    panelHidden: true,
    retry: false,
  });
  assert.deepEqual(replayPresentation("error"), {
    lcdHidden: false,
    videoHidden: true,
    panelHidden: false,
    retry: true,
  });
});

test("playback rate helpers normalize, persist, and ignore storage failures", () => {
  assert.equal(normalizePlaybackRate(8), 8);
  assert.equal(normalizePlaybackRate("16"), 16);
  assert.equal(normalizePlaybackRate(3), 1);
  assert.equal(normalizePlaybackRate("nope"), 1);

  const store = new Map();
  const storage = {
    getItem: (key) => (store.has(key) ? store.get(key) : null),
    setItem: (key, value) => { store.set(key, String(value)); },
  };
  assert.equal(readStoredPlaybackRate(storage), 1);
  assert.equal(writeStoredPlaybackRate(storage, 4), 4);
  assert.equal(storage.getItem("pokepilot.replayPlaybackRate"), "4");
  assert.equal(readStoredPlaybackRate(storage), 4);
  assert.equal(writeStoredPlaybackRate(storage, 99), 1);

  const exploding = {
    getItem() { throw new Error("blocked"); },
    setItem() { throw new Error("blocked"); },
  };
  assert.equal(readStoredPlaybackRate(exploding), 1);
  assert.equal(writeStoredPlaybackRate(exploding, 8), 8);
});

test("drawer open and Escape transitions carry focus intent", () => {
  assert.equal(typeof drawerTransition, "function");
  const opened = drawerTransition({ open: false }, "open", true);
  assert.deepEqual(opened, {
    open: true,
    inert: false,
    ariaHidden: "false",
    focus: "selected-or-close",
  });
  assert.deepEqual(drawerTransition(opened, "escape", true), {
    open: false,
    inert: true,
    ariaHidden: "true",
    focus: "restore-trigger",
  });
});

test("keyboard and click selection share the mobile close transition", () => {
  assert.equal(typeof drawerTransition, "function");
  const opened = drawerTransition({ open: false }, "open", true);
  const keyboard = drawerTransition(opened, "select-keyboard", true);
  const click = drawerTransition(opened, "select-click", true);

  assert.deepEqual(keyboard, click);
  assert.deepEqual(keyboard, {
    open: false,
    inert: true,
    ariaHidden: "true",
    focus: "restore-trigger",
  });
});

test("drawer breakpoint transition toggles inert without closing desktop rail", () => {
  assert.equal(typeof drawerTransition, "function");
  const mobile = drawerTransition({ open: false }, "breakpoint", true);
  assert.deepEqual(mobile, {
    open: false,
    inert: true,
    ariaHidden: "true",
    focus: "none",
  });
  assert.deepEqual(drawerTransition(mobile, "breakpoint", false), {
    open: false,
    inert: false,
    ariaHidden: "false",
    focus: "none",
  });
  const focusedMobile = drawerTransition({ open: false }, "breakpoint", true, true);
  assert.deepEqual(focusedMobile, {
    open: false,
    inert: true,
    ariaHidden: "true",
    focus: "restore-trigger",
  });
  assert.deepEqual(drawerTransition(focusedMobile, "breakpoint", false), {
    open: false,
    inert: false,
    ariaHidden: "false",
    focus: "none",
  });
  assert.deepEqual(drawerTransition({ open: false }, "select-keyboard", false), {
    open: false,
    inert: false,
    ariaHidden: "false",
    focus: "none",
  });
});

test("finished selection and generate click execute replay request and poll wiring", async () => {
  const harness = createInspectorHarness();
  harness.select("finished");
  await flushAsync();
  await flushAsync();

  assert.ok(harness.requests.some(({ url }) => url.endsWith("/finished/debug")));
  assert.ok(harness.requests.some(({ url }) => url.endsWith("/finished/replay/status")));
  assert.equal(harness.elements["pp-replay"].textContent, "Generate replay");

  harness.elements["pp-replay"].emit("click");
  await flushAsync();
  assert.ok(harness.requests.some(({ url, method }) => url.endsWith("/finished/replay/render") && method === "POST"));
  assert.equal(harness.elements["pp-replay-status"].textContent, "rendering");
  assert.equal(harness.timers.length, 1);

  harness.timers.shift()();
  await flushAsync();
  await flushAsync();
  assert.ok(harness.requests.filter(({ url }) => url.endsWith("/finished/replay/status")).length >= 2);
  assert.equal(harness.elements["pp-game-video"].src, "/v1/runs/finished/replay/video");
  assert.equal(harness.elements["detail-lcd"].hidden, false);
});

test("video readiness and failure execute LCD fallback DOM wiring", async () => {
  const harness = createInspectorHarness();
  harness.select("finished");
  await flushAsync();
  await flushAsync();
  harness.elements["pp-replay"].emit("click");
  await flushAsync();
  harness.timers.shift()();
  await flushAsync();
  await flushAsync();

  const video = harness.elements["pp-game-video"];
  video.emit("canplay");
  assert.equal(harness.elements["detail-lcd"].hidden, true);
  assert.equal(video.hidden, false);

  video.emit("error");
  assert.equal(harness.elements["detail-lcd"].hidden, false);
  assert.equal(video.hidden, true);
  assert.equal(harness.elements["pp-replay-panel"].hidden, false);
  assert.equal(harness.elements["pp-replay"].textContent, "Reload replay");
  assert.match(harness.elements["pp-replay-status"].textContent, /could not be played/);
});

test("timeline click, video seek, and Return to live execute inspector event wiring", async () => {
  const harness = createInspectorHarness();
  harness.select("finished");
  await flushAsync();
  await flushAsync();
  harness.elements["pp-replay"].emit("click");
  await flushAsync();
  harness.timers.shift()();
  await flushAsync();
  await flushAsync();

  const video = harness.elements["pp-game-video"];
  video.duration = 100;
  video.emit("canplay");
  const firstMarker = new FakeTarget("first-marker");
  firstMarker.dataset.eventIndex = "0";
  firstMarker.parent = harness.elements["run-inspector"];
  firstMarker.emit("click");
  assert.equal(video.currentTime, 10);
  assert.equal(harness.semanticEvents.at(-1).event.decision, "first");

  video.currentTime = 79;
  video.emit("seeked");
  assert.equal(harness.semanticEvents.at(-1).event.decision, "latest");
  assert.equal(harness.semanticEvents.at(-1).nearest, true);

  harness.select("active");
  await flushAsync();
  await flushAsync();
  const activeFirst = new FakeTarget("active-first");
  activeFirst.dataset.eventIndex = "0";
  activeFirst.parent = harness.elements["run-inspector"];
  activeFirst.emit("click");
  assert.equal(harness.elements["pp-return-live"].hidden, false);
  harness.elements["pp-return-live"].emit("click");
  assert.equal(harness.elements["pp-return-live"].hidden, true);
  assert.equal(harness.elements["pp-transport-status"].textContent, "Following live");
  assert.equal(harness.semanticEvents.at(-1).event, null);
  assert.match(harness.elements["pp-timeline-track"].innerHTML, /data-event-index="1"[^>]+aria-current="true"/);
});

test("Operations and Analytics tabs execute shared click and keyboard DOM wiring", () => {
  assert.equal(typeof applyTabView, "function");
  assert.equal(typeof wireTabNavigation, "function");
  const analytics = new FakeTarget("tab-analytics");
  analytics.dataset.view = "analytics";
  const operations = new FakeTarget("tab-operations");
  operations.dataset.view = "operations";
  const tabs = [analytics, operations];
  const panels = tabs.map((tab) => {
    const panel = new FakeTarget(`view-${tab.dataset.view}`);
    panel.dataset.consoleView = tab.dataset.view;
    return panel;
  });
  const activate = (view) => applyTabView(tabs, panels, view);
  wireTabNavigation(tabs, activate);

  operations.emit("click");
  assert.equal(operations.getAttribute("aria-selected"), "true");
  assert.equal(panels[1].hidden, false);
  assert.equal(panels[0].hidden, true);

  operations.emit("keydown", { key: "ArrowLeft" });
  assert.equal(analytics.focused, true);
  assert.equal(analytics.getAttribute("aria-selected"), "true");
  assert.equal(panels[0].hidden, false);
  assert.equal(panels[1].hidden, true);
});
