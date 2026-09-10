"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const {
  eligibleRuns,
  ageDescription,
  deleteRuns,
} = require("./run_cleanup.js");

test("eligibleRuns selects only finished runs older than the cutoff", () => {
  const now = 10_000;
  const runs = [
    { run_id: "old", status: "done", ended_at: 1_000 },
    { run_id: "edge", status: "done", ended_at: 6_400 },
    { run_id: "fresh", status: "done", ended_at: 6_401 },
    { run_id: "active", status: "running", ended_at: 1_000 },
    { run_id: "queued", status: "queued", ended_at: 1_000 },
    { run_id: "no-end", status: "done", ended_at: 0 },
  ];

  assert.deepEqual(
    eligibleRuns(runs, now, 3_600).map((run) => run.run_id),
    ["old", "edge"],
  );
});

test("eligibleRuns rejects invalid clocks and thresholds", () => {
  const runs = [{ run_id: "old", status: "done", ended_at: 1 }];
  assert.deepEqual(eligibleRuns(runs, 0, 60), []);
  assert.deepEqual(eligibleRuns(runs, 100, 0), []);
  assert.deepEqual(eligibleRuns(null, 100, 60), []);
});

test("ageDescription names the cleanup presets", () => {
  assert.equal(ageDescription(60 * 60), "1 hour");
  assert.equal(ageDescription(6 * 60 * 60), "6 hours");
  assert.equal(ageDescription(24 * 60 * 60), "24 hours");
  assert.equal(ageDescription(7 * 24 * 60 * 60), "7 days");
  assert.equal(ageDescription(30 * 24 * 60 * 60), "30 days");
});

test("deleteRuns bounds concurrency and reports partial failures", async () => {
  const ids = ["a", "b", "c", "d", "e", "f"];
  let active = 0;
  let maxActive = 0;
  const progress = [];

  const result = await deleteRuns(ids, async (id) => {
    active++;
    maxActive = Math.max(maxActive, active);
    await new Promise((resolve) => setTimeout(resolve, 5));
    active--;
    if (id === "c" || id === "f") throw new Error(`cannot delete ${id}`);
  }, 3, (state) => progress.push({ ...state }));

  assert.ok(maxActive <= 3, `maximum concurrency was ${maxActive}`);
  assert.deepEqual([...result.deletedIds].sort(), ["a", "b", "d", "e"]);
  assert.deepEqual(result.failures.map((failure) => failure.id).sort(), ["c", "f"]);
  assert.equal(progress.at(-1).completed, ids.length);
  assert.equal(progress.at(-1).deleted, 4);
  assert.equal(progress.at(-1).failed, 2);
});
