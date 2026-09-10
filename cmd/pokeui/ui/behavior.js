(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  else root.PokeConsoleBehavior = api;
})(typeof globalThis === "object" ? globalThis : this, function () {
  "use strict";

  function partitionOperations(runs, recentLimit) {
    const list = Array.isArray(runs) ? runs : [];
    const waiting = list
      .filter((run) => run.status === "queued" || run.status === "leased")
      .sort((a, b) => Number(a.queued_at || 0) - Number(b.queued_at || 0));
    const active = list
      .filter((run) => run.status === "running")
      .sort((a, b) => Number(a.queued_at || 0) - Number(b.queued_at || 0));
    const recent = list
      .filter((run) => run.status === "done")
      .sort((a, b) => Number(b.ended_at || 0) - Number(a.ended_at || 0))
      .slice(0, Math.max(0, Number(recentLimit) || 0));
    return { waiting, active, recent };
  }

  function timelineLayout(events, totalFrames) {
    const list = Array.isArray(events) ? events : [];
    const maximum = Math.max(1, Number(totalFrames) || 0, ...list.map((event) => Number(event && event.frame) || 0));
    const positions = list.map((event) => Math.max(0, Math.min(100, 100 * (Number(event && event.frame) || 0) / maximum)));
    const lanes = [];
    const laneEnds = [];
    const collisionDistance = 1.75;
    for (const position of positions) {
      let lane = laneEnds.findIndex((end) => position - end >= collisionDistance);
      if (lane < 0) lane = laneEnds.length;
      lanes.push(lane);
      laneEnds[lane] = position;
    }
    return { positions, lanes, laneCount: Math.max(1, laneEnds.length), totalFrames: maximum };
  }

  function replayPresentation(state) {
    if (state === "playing") {
      return { lcdHidden: true, videoHidden: false, panelHidden: true, retry: false };
    }
    if (state === "error") {
      return { lcdHidden: false, videoHidden: true, panelHidden: false, retry: true };
    }
    return { lcdHidden: false, videoHidden: true, panelHidden: true, retry: false };
  }

  function drawerPresentation(open, mobile, restoreFocus) {
    const visible = Boolean(open);
    const offCanvas = Boolean(mobile) && !visible;
    return {
      open: visible,
      inert: offCanvas,
      ariaHidden: offCanvas ? "true" : "false",
      focus: !mobile ? "none" : (visible ? "selected-or-close" : (restoreFocus ? "restore-trigger" : "none")),
    };
  }

  function drawerTransition(current, action, mobile, focusInside) {
    if (!mobile) return drawerPresentation(false, false, false);
    const wasOpen = Boolean(current && current.open);
    if (action === "open") return drawerPresentation(true, true, false);
    const closes = action === "close"
      || action === "escape"
      || action === "select-keyboard"
      || action === "select-click";
    if (closes) return drawerPresentation(false, true, wasOpen);
    if (action === "breakpoint") return drawerPresentation(false, true, Boolean(focusInside));
    return drawerPresentation(wasOpen, true, false);
  }

  function applyTabView(tabs, panels, view) {
    for (const panel of panels) panel.hidden = panel.dataset.consoleView !== view;
    for (const tab of tabs) {
      const active = tab.dataset.view === view;
      tab.setAttribute("aria-selected", String(active));
      tab.tabIndex = active ? 0 : -1;
    }
  }

  function wireTabNavigation(tabs, activate) {
    tabs.forEach((tab, index) => {
      tab.addEventListener("click", () => activate(tab.dataset.view));
      tab.addEventListener("keydown", (event) => {
        let next;
        if (event.key === "ArrowLeft") next = (index - 1 + tabs.length) % tabs.length;
        else if (event.key === "ArrowRight") next = (index + 1) % tabs.length;
        else if (event.key === "Home") next = 0;
        else if (event.key === "End") next = tabs.length - 1;
        else return;
        event.preventDefault();
        tabs[next].focus();
        activate(tabs[next].dataset.view);
      });
    });
  }

  return {
    partitionOperations,
    timelineLayout,
    replayPresentation,
    drawerPresentation,
    drawerTransition,
    applyTabView,
    wireTabNavigation,
  };
});
