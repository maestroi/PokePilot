"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const {
  partitionOperations,
  timelineLayout,
  replayPresentation,
  drawerTransition,
} = require("./behavior.js");

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

test("timeline layout grows and keeps dense markers apart", () => {
  const layout = timelineLayout(20, 36, 32);

  assert.equal(layout.width, "max(100%, 716px)");
  assert.equal(layout.positions.length, 20);
  for (let index = 1; index < layout.positions.length; index++) {
    assert.ok(
      layout.positions[index] - layout.positions[index - 1] >= 36,
      `markers ${index - 1} and ${index} overlap`,
    );
  }
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
  assert.deepEqual(drawerTransition({ open: false }, "select-keyboard", false), {
    open: false,
    inert: false,
    ariaHidden: "false",
    focus: "none",
  });
});
