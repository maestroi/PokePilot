"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { issueNumberFromText, findGroup } = require("./triage_keys.js");

test("issueNumberFromText reads displayed issue numbers", () => {
  assert.equal(issueNumberFromText("Issue #232"), 232);
  assert.equal(issueNumberFromText("issue #7 · fixed"), 7);
  assert.equal(issueNumberFromText("no issue"), 0);
});

test("findGroup prefers issue number and falls back to exact pattern", () => {
  const groups = [
    { key: "aaa", pattern: "same", issue: { issue_number: 231 } },
    { key: "bbb", pattern: "other", issue: { issue_number: 232 } },
  ];
  assert.equal(findGroup(groups, "same", 232).key, "bbb");
  assert.equal(findGroup(groups, "same", 0).key, "aaa");
  assert.equal(findGroup(groups, "missing", 0), null);
});
