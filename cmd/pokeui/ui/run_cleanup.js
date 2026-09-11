(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  else {
    root.PokeRunCleanup = api;
    api.mount(root);
  }
})(typeof globalThis === "object" ? globalThis : this, function () {
  "use strict";

  const AGE_OPTIONS = [
    { seconds: 60 * 60, label: "1h", description: "1 hour" },
    { seconds: 6 * 60 * 60, label: "6h", description: "6 hours" },
    { seconds: 24 * 60 * 60, label: "24h", description: "24 hours" },
    { seconds: 7 * 24 * 60 * 60, label: "7d", description: "7 days" },
    { seconds: 30 * 24 * 60 * 60, label: "30d", description: "30 days" },
  ];
  const DEFAULT_AGE_SECONDS = 7 * 24 * 60 * 60;
  const DELETE_CONCURRENCY = 3;
  const FAILURE_PATTERN_CAP = 128;

  // Keep this byte-for-byte equivalent to pokewall.normalizeDetail: the
  // failure-group pattern is the contract that lets cleanup select every run
  // for one bug instead of relying on the five example run ids /v1/triage
  // exposes for investigation.
  function normalizeFailureDetail(detail) {
    return String(detail || "")
      .replace(/0x[0-9a-fA-F]+/g, "<hex>")
      .replace(/\d+/g, "<n>")
      .slice(0, FAILURE_PATTERN_CAP);
  }

  function bugGroupRuns(runs, pattern) {
    const wanted = String(pattern || "");
    if (!wanted) return [];
    return (Array.isArray(runs) ? runs : [])
      .filter((run) => run
        && run.status === "done"
        && (run.reason === "error" || run.reason === "lost")
        && run.detail
        && normalizeFailureDetail(run.detail) === wanted)
      .sort((a, b) => Number(a.ended_at || 0) - Number(b.ended_at || 0));
  }

  function eligibleRuns(runs, nowSeconds, ageSeconds) {
    const now = Number(nowSeconds) || 0;
    const age = Number(ageSeconds) || 0;
    if (now <= 0 || age <= 0) return [];
    const cutoff = now - age;
    return (Array.isArray(runs) ? runs : [])
      .filter((run) => run && run.status === "done" && Number(run.ended_at || 0) > 0 && Number(run.ended_at) <= cutoff)
      .sort((a, b) => Number(a.ended_at) - Number(b.ended_at));
  }

  function ageDescription(ageSeconds) {
    const match = AGE_OPTIONS.find((option) => option.seconds === Number(ageSeconds));
    return match ? match.description : `${Math.round(Number(ageSeconds) || 0)} seconds`;
  }

  async function deleteRuns(ids, deleteOne, concurrency, onProgress) {
    const queue = Array.isArray(ids) ? ids.filter(Boolean) : [];
    const workerCount = Math.max(1, Math.min(queue.length || 1, Number(concurrency) || 1));
    const deletedIds = [];
    const failures = [];
    let cursor = 0;
    let completed = 0;

    async function worker() {
      while (true) {
        const index = cursor++;
        if (index >= queue.length) return;
        const id = queue[index];
        try {
          await deleteOne(id);
          deletedIds.push(id);
        } catch (error) {
          failures.push({ id, error: error instanceof Error ? error.message : String(error) });
        } finally {
          completed++;
          if (onProgress) onProgress({ completed, total: queue.length, deleted: deletedIds.length, failed: failures.length });
        }
      }
    }

    await Promise.all(Array.from({ length: workerCount }, () => worker()));
    return { deletedIds, failures };
  }

  function mount(root) {
    const doc = root && root.document;
    if (!doc || typeof root.fetch !== "function") return;
    const archive = doc.querySelector(".archive-tools");
    if (!archive || doc.getElementById("run-cleanup-tools")) return;

    const tools = doc.createElement("div");
    tools.id = "run-cleanup-tools";
    tools.className = "filters";
    tools.setAttribute("aria-label", "Bulk run cleanup");
    tools.innerHTML = `<div class="filter-group"><span>cleanup older than</span>${AGE_OPTIONS.map((option) => `<button type="button" class="filter" data-cleanup-age="${option.seconds}" aria-pressed="${option.seconds === DEFAULT_AGE_SECONDS}">${option.label}</button>`).join("")}<button type="button" class="danger-button" id="run-cleanup-delete" disabled>Delete older runs</button><span class="pager-count" id="run-cleanup-status" aria-live="polite">Open Runs to check cleanup candidates.</span></div><div class="filter-group"><span>cleanup bug</span><select id="run-cleanup-bug" aria-label="Failure group to clean"><option value="">Choose failure group…</option></select><button type="button" class="danger-button" id="run-cleanup-delete-bug" disabled>Delete matching runs</button><span class="pager-count" id="run-cleanup-bug-status" aria-live="polite">Choose a failure group to remove obsolete run data.</span></div>`;
    archive.appendChild(tools);

    const deleteButton = doc.getElementById("run-cleanup-delete");
    const status = doc.getElementById("run-cleanup-status");
    const bugSelect = doc.getElementById("run-cleanup-bug");
    const bugDeleteButton = doc.getElementById("run-cleanup-delete-bug");
    const bugStatus = doc.getElementById("run-cleanup-bug-status");
    const ageButtons = [...tools.querySelectorAll("[data-cleanup-age]")];
    let selectedAge = DEFAULT_AGE_SECONDS;
    let selectedBugKey = "";
    let snapshot = null;
    let triageGroups = [];
    let busyKind = "";
    let ageMessage = "";
    let bugMessage = "";

    function candidates() {
      return eligibleRuns(snapshot && snapshot.runs, snapshot && snapshot.now, selectedAge);
    }

    function selectedBug() {
      return triageGroups.find((group) => group && group.key === selectedBugKey) || null;
    }

    function bugCandidates() {
      const group = selectedBug();
      return group ? bugGroupRuns(snapshot && snapshot.runs, group.pattern) : [];
    }

    function groupLabel(group) {
      const issue = group && group.issue;
      const state = issue && String(issue.resolution || issue.status || "").trim();
      const prefix = state ? `[${state}] ` : "";
      const pattern = String((group && (group.pattern || group.example || group.key)) || "unknown failure");
      return `${prefix}${pattern} (${Number(group && group.count) || 0})`;
    }

    function renderBugOptions() {
      const wanted = selectedBugKey;
      bugSelect.replaceChildren();
      const placeholder = doc.createElement("option");
      placeholder.value = "";
      placeholder.textContent = triageGroups.length ? "Choose failure group…" : "No failure groups";
      bugSelect.appendChild(placeholder);
      for (const group of triageGroups) {
        if (!group || !group.key || !group.pattern) continue;
        const option = doc.createElement("option");
        option.value = group.key;
        option.textContent = groupLabel(group);
        bugSelect.appendChild(option);
      }
      if (wanted && triageGroups.some((group) => group && group.key === wanted)) {
        bugSelect.value = wanted;
      } else {
        selectedBugKey = "";
        bugSelect.value = "";
      }
    }

    function render() {
      for (const button of ageButtons) button.setAttribute("aria-pressed", String(Number(button.dataset.cleanupAge) === selectedAge));
      const count = candidates().length;
      const busy = busyKind !== "";
      deleteButton.disabled = busy || !snapshot || count === 0;
      deleteButton.textContent = busyKind === "age" ? "Deleting…" : (count ? `Delete ${count} older run${count === 1 ? "" : "s"}` : "Delete older runs");
      status.textContent = ageMessage || (!snapshot
        ? "Open Runs to check cleanup candidates."
        : `${count} finished run${count === 1 ? "" : "s"} older than ${ageDescription(selectedAge)}.`);

      const group = selectedBug();
      const bugCount = bugCandidates().length;
      bugSelect.disabled = busy || !snapshot || triageGroups.length === 0;
      bugDeleteButton.disabled = busy || !snapshot || !group || bugCount === 0;
      bugDeleteButton.textContent = busyKind === "bug" ? "Deleting…" : (bugCount ? `Delete ${bugCount} matching run${bugCount === 1 ? "" : "s"}` : "Delete matching runs");
      bugStatus.textContent = bugMessage || (!snapshot
        ? "Open Runs to check failure groups."
        : !group
          ? `${triageGroups.length} failure group${triageGroups.length === 1 ? "" : "s"} available.`
          : `${bugCount} finished run${bugCount === 1 ? "" : "s"} match triage key ${group.key}.`);
    }

    async function refreshData() {
      // Bulk cleanup is an explicit operator action, so it may fetch the full
      // finished catalog. The normal 2s console poll is intercepted separately
      // and stays active-only plus one paged history slice.
      const [dashboardResponse, triageResponse] = await Promise.all([
        root.fetch("/v1/dashboard?status=done", { cache: "no-store" }),
        root.fetch("/v1/triage", { cache: "no-store" }),
      ]);
      if (!dashboardResponse.ok) throw new Error(`dashboard returned ${dashboardResponse.status}`);
      if (!triageResponse.ok) throw new Error(`triage returned ${triageResponse.status}`);
      snapshot = await dashboardResponse.json();
      const groups = await triageResponse.json();
      triageGroups = Array.isArray(groups) ? groups : [];
      renderBugOptions();
      render();
      return snapshot;
    }

    async function deleteOne(id) {
      const response = await root.fetch("/v1/runs/" + encodeURIComponent(id), { method: "DELETE" });
      const body = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(body.error || `delete returned ${response.status}`);
    }

    function forgetDeletedSelection(deletedIds) {
      const remembered = root.localStorage && root.localStorage.getItem("pokefarm-selected-run");
      if (!remembered || !deletedIds.includes(remembered)) return;
      root.localStorage.removeItem("pokefarm-selected-run");
      if (typeof root.CustomEvent === "function") {
        root.dispatchEvent(new root.CustomEvent("pokefarm-select-run", { detail: { runId: "" } }));
      }
    }

    function failureSummary(result) {
      if (!result.failures.length) {
        return `${result.deletedIds.length} run${result.deletedIds.length === 1 ? "" : "s"} deleted with artifacts.`;
      }
      const examples = result.failures.slice(0, 3).map((failure) => failure.id).join(", ");
      return `${result.deletedIds.length} deleted · ${result.failures.length} failed${examples ? ` (${examples}${result.failures.length > 3 ? ", …" : ""})` : ""}. Failed runs were kept so cleanup can be retried.`;
    }

    for (const button of ageButtons) {
      button.addEventListener("click", () => {
        selectedAge = Number(button.dataset.cleanupAge) || DEFAULT_AGE_SECONDS;
        ageMessage = "";
        render();
      });
    }

    bugSelect.addEventListener("change", () => {
      selectedBugKey = bugSelect.value || "";
      bugMessage = "";
      render();
    });

    deleteButton.addEventListener("click", async () => {
      if (busyKind) return;
      busyKind = "age";
      ageMessage = "Checking finished runs…";
      render();
      try {
        await refreshData();
        const runs = candidates();
        if (!runs.length) {
          ageMessage = `No finished runs are older than ${ageDescription(selectedAge)}.`;
          return;
        }
        const confirmed = typeof root.confirm !== "function" || root.confirm(
          `Delete ${runs.length} finished run${runs.length === 1 ? "" : "s"} older than ${ageDescription(selectedAge)}?\n\nThis also permanently deletes their S3 artifacts and replay cache. This cannot be undone.`
        );
        if (!confirmed) {
          ageMessage = "Cleanup cancelled.";
          return;
        }

        const result = await deleteRuns(
          runs.map((run) => run.run_id),
          deleteOne,
          DELETE_CONCURRENCY,
          (progress) => {
            ageMessage = `Deleting ${progress.completed}/${progress.total} · ${progress.deleted} deleted${progress.failed ? ` · ${progress.failed} failed` : ""}`;
            render();
          },
        );

        forgetDeletedSelection(result.deletedIds);
        await refreshData();
        ageMessage = failureSummary(result);
      } catch (error) {
        ageMessage = `Cleanup failed: ${error instanceof Error ? error.message : String(error)}`;
      } finally {
        busyKind = "";
        render();
      }
    });

    bugDeleteButton.addEventListener("click", async () => {
      if (busyKind || !selectedBugKey) return;
      const requestedKey = selectedBugKey;
      busyKind = "bug";
      bugMessage = "Checking matching runs…";
      render();
      try {
        await refreshData();
        const group = triageGroups.find((item) => item && item.key === requestedKey);
        if (!group) {
          bugMessage = `Failure group ${requestedKey} no longer exists.`;
          return;
        }
        selectedBugKey = requestedKey;
        bugSelect.value = requestedKey;
        const runs = bugGroupRuns(snapshot && snapshot.runs, group.pattern);
        if (!runs.length) {
          bugMessage = `No finished runs still match ${requestedKey}.`;
          return;
        }
        const confirmed = typeof root.confirm !== "function" || root.confirm(
          `Delete all ${runs.length} finished run${runs.length === 1 ? "" : "s"} for failure ${requestedKey}?\n\nPattern: ${group.pattern}\n\nThis also permanently deletes their S3 artifacts and replay cache. This cannot be undone.`
        );
        if (!confirmed) {
          bugMessage = "Bug cleanup cancelled.";
          return;
        }

        const result = await deleteRuns(
          runs.map((run) => run.run_id),
          deleteOne,
          DELETE_CONCURRENCY,
          (progress) => {
            bugMessage = `Deleting ${progress.completed}/${progress.total} · ${progress.deleted} deleted${progress.failed ? ` · ${progress.failed} failed` : ""}`;
            render();
          },
        );

        forgetDeletedSelection(result.deletedIds);
        await refreshData();
        bugMessage = failureSummary(result);
      } catch (error) {
        bugMessage = `Bug cleanup failed: ${error instanceof Error ? error.message : String(error)}`;
      } finally {
        busyKind = "";
        render();
      }
    });

    root.addEventListener("pokefarm-console-view", (event) => {
      if (!event.detail || event.detail.view !== "runs") return;
      ageMessage = "";
      bugMessage = "";
      refreshData().catch((error) => {
        ageMessage = `Could not check cleanup candidates: ${error instanceof Error ? error.message : String(error)}`;
        bugMessage = `Could not check failure groups: ${error instanceof Error ? error.message : String(error)}`;
        render();
      });
    });

    const runsView = doc.getElementById("view-runs");
    if (runsView && !runsView.hidden) {
      refreshData().catch((error) => {
        ageMessage = `Could not check cleanup candidates: ${error instanceof Error ? error.message : String(error)}`;
        bugMessage = `Could not check failure groups: ${error instanceof Error ? error.message : String(error)}`;
        render();
      });
    } else {
      render();
    }
  }

  return {
    AGE_OPTIONS,
    DEFAULT_AGE_SECONDS,
    DELETE_CONCURRENCY,
    FAILURE_PATTERN_CAP,
    normalizeFailureDetail,
    bugGroupRuns,
    eligibleRuns,
    ageDescription,
    deleteRuns,
    mount,
  };
});
