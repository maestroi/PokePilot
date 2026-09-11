(function () {
  const thumbnailMs = 500; // 2 fps for unselected live cards; selected stays at ui.js's 20 fps.
  const originalFetch = window.fetch.bind(window);
  const nextFrameAt = new Map();

  window.fetch = async function (input, init) {
    let url;
    try {
      url = new URL(typeof input === "string" ? input : input.url, location.href);
    } catch (e) {
      return originalFetch(input, init);
    }
    if (url.origin === location.origin && url.pathname === "/frame") {
      const runID = url.searchParams.get("run") || "";
      const selected = document.documentElement.dataset.selectedRun || "";
      if (runID && runID !== selected) {
        const now = performance.now();
        const wait = Math.max(0, (nextFrameAt.get(runID) || 0) - now);
        if (wait > 0) await new Promise((resolve) => setTimeout(resolve, wait));
        nextFrameAt.set(runID, performance.now() + thumbnailMs);
      }
    }
    return originalFetch(input, init);
  };
})();
