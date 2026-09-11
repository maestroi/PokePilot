(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  else api.mount(root);
})(typeof globalThis === "object" ? globalThis : this, function () {
  "use strict";

  const REFRESH_MS = 10_000;

  function issueNumberFromText(text) {
    const match = String(text || "").match(/Issue\s+#(\d+)/i);
    return match ? Number(match[1]) : 0;
  }

  function findGroup(groups, pattern, issueNumber) {
    const list = Array.isArray(groups) ? groups : [];
    const wantedPattern = String(pattern || "").trim();
    const wantedIssue = Number(issueNumber) || 0;
    if (wantedIssue) {
      const byIssue = list.find((group) => group && group.issue && Number(group.issue.issue_number) === wantedIssue);
      if (byIssue) return byIssue;
    }
    if (wantedPattern) {
      return list.find((group) => group && String(group.pattern || "").trim() === wantedPattern) || null;
    }
    return null;
  }

  function mount(root) {
    const doc = root && root.document;
    if (!doc || typeof root.fetch !== "function") return;
    const failures = doc.getElementById("failures");
    if (!failures) return;

    let groups = [];
    let loading = false;
    let loadedAt = 0;

    function decorate() {
      for (const card of failures.querySelectorAll(".fail-card")) {
        const meta = card.querySelector(".fail-meta");
        if (!meta) continue;
        const pattern = card.querySelector(".fail-pat")?.textContent || "";
        const issueNumber = issueNumberFromText(card.querySelector(".issue-a")?.textContent || meta.textContent || "");
        const group = findGroup(groups, pattern, issueNumber);
        if (!group || !group.key) continue;

        const existing = meta.querySelector("[data-triage-key]");
        if (existing && existing.dataset.triageKey === group.key) continue;
        if (existing) existing.remove();

        const chip = doc.createElement("span");
        chip.className = "chip";
        chip.dataset.triageKey = group.key;
        chip.textContent = `key ${group.key}`;
        chip.title = `Triage key for cleanup: ${group.key}`;
        meta.appendChild(chip);
      }
    }

    async function refresh(force) {
      if (loading) return;
      const now = Date.now();
      if (!force && groups.length && now - loadedAt < REFRESH_MS) {
        decorate();
        return;
      }
      loading = true;
      try {
        const response = await root.fetch("/v1/triage", { cache: "no-store" });
        if (!response.ok) return;
        const payload = await response.json();
        groups = Array.isArray(payload) ? payload : [];
        loadedAt = Date.now();
        decorate();
      } catch (_) {
        // The main console already reports wall availability. A missing key
        // chip should not add a second error surface.
      } finally {
        loading = false;
      }
    }

    const observer = typeof root.MutationObserver === "function"
      ? new root.MutationObserver(() => {
          decorate();
          if (Date.now() - loadedAt >= REFRESH_MS) void refresh(false);
        })
      : null;
    if (observer) observer.observe(failures, { childList: true, subtree: true });

    void refresh(true);
  }

  return { issueNumberFromText, findGroup, mount };
});
