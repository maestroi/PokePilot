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
    tools.innerHTML = `<div class="filter-group"><span>cleanup older than</span>${AGE_OPTIONS.map((option) => `<button type="button" class="filter" data-cleanup-age="${option.seconds}" aria-pressed="${option.seconds === DEFAULT_AGE_SECONDS}">${option.label}</button>`).join("")}<button type="button" class="danger-button" id="run-cleanup-delete" disabled>Delete older runs</button><span class="pager-count" id="run-cleanup-status" aria-live="polite">Open Runs to check cleanup candidates.</span></div>`;
    archive.appendChild(tools);

    const deleteButton = doc.getElementById("run-cleanup-delete");
    const status = doc.getElementById("run-cleanup-status");
    const ageButtons = [...tools.querySelectorAll("[data-cleanup-age]")];
    let selectedAge = DEFAULT_AGE_SECONDS;
    let snapshot = null;
    let busy = false;
    let message = "";

    function candidates() {
      return eligibleRuns(snapshot && snapshot.runs, snapshot && snapshot.now, selectedAge);
    }

    function render() {
      for (const button of ageButtons) button.setAttribute("aria-pressed", String(Number(button.dataset.cleanupAge) === selectedAge));
      const count = candidates().length;
      deleteButton.disabled = busy || !snapshot || count === 0;
      deleteButton.textContent = busy ? "Deleting…" : (count ? `Delete ${count} older run${count === 1 ? "" : "s"}` : "Delete older runs");
      status.textContent = message || (!snapshot
        ? "Open Runs to check cleanup candidates."
        : `${count} finished run${count === 1 ? "" : "s"} older than ${ageDescription(selectedAge)}.`);
    }

    async function refreshSnapshot() {
      const response = await root.fetch("/v1/dashboard", { cache: "no-store" });
      if (!response.ok) throw new Error(`dashboard returned ${response.status}`);
      snapshot = await response.json();
      render();
      return snapshot;
    }

    async function deleteOne(id) {
      const response = await root.fetch("/v1/runs/" + encodeURIComponent(id), { method: "DELETE" });
      const body = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(body.error || `delete returned ${response.status}`);
    }

    for (const button of ageButtons) {
      button.addEventListener("click", () => {
        selectedAge = Number(button.dataset.cleanupAge) || DEFAULT_AGE_SECONDS;
        message = "";
        render();
      });
    }

    deleteButton.addEventListener("click", async () => {
      if (busy) return;
      busy = true;
      message = "Checking finished runs…";
      render();
      try {
        await refreshSnapshot();
        const runs = candidates();
        if (!runs.length) {
          message = `No finished runs are older than ${ageDescription(selectedAge)}.`;
          return;
        }
        const confirmed = typeof root.confirm !== "function" || root.confirm(
          `Delete ${runs.length} finished run${runs.length === 1 ? "" : "s"} older than ${ageDescription(selectedAge)}?\n\nThis also permanently deletes their S3 artifacts and replay cache. This cannot be undone.`
        );
        if (!confirmed) {
          message = "Cleanup cancelled.";
          return;
        }

        const result = await deleteRuns(
          runs.map((run) => run.run_id),
          deleteOne,
          DELETE_CONCURRENCY,
          (progress) => {
            message = `Deleting ${progress.completed}/${progress.total} · ${progress.deleted} deleted${progress.failed ? ` · ${progress.failed} failed` : ""}`;
            render();
          },
        );

        const remembered = root.localStorage && root.localStorage.getItem("pokefarm-selected-run");
        if (remembered && result.deletedIds.includes(remembered)) {
          root.localStorage.removeItem("pokefarm-selected-run");
          if (typeof root.CustomEvent === "function") {
            root.dispatchEvent(new root.CustomEvent("pokefarm-select-run", { detail: { runId: "" } }));
          }
        }

        await refreshSnapshot();
        if (result.failures.length) {
          const examples = result.failures.slice(0, 3).map((failure) => failure.id).join(", ");
          message = `${result.deletedIds.length} deleted · ${result.failures.length} failed${examples ? ` (${examples}${result.failures.length > 3 ? ", …" : ""})` : ""}. Failed runs were kept so cleanup can be retried.`;
        } else {
          message = `${result.deletedIds.length} run${result.deletedIds.length === 1 ? "" : "s"} deleted with artifacts.`;
        }
      } catch (error) {
        message = `Cleanup failed: ${error instanceof Error ? error.message : String(error)}`;
      } finally {
        busy = false;
        render();
      }
    });

    root.addEventListener("pokefarm-console-view", (event) => {
      if (!event.detail || event.detail.view !== "runs") return;
      message = "";
      refreshSnapshot().catch((error) => {
        message = `Could not check cleanup candidates: ${error instanceof Error ? error.message : String(error)}`;
        render();
      });
    });

    const runsView = doc.getElementById("view-runs");
    if (runsView && !runsView.hidden) {
      refreshSnapshot().catch((error) => {
        message = `Could not check cleanup candidates: ${error instanceof Error ? error.message : String(error)}`;
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
    eligibleRuns,
    ageDescription,
    deleteRuns,
    mount,
  };
});
