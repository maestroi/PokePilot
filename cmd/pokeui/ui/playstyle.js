(() => {
  const form = document.getElementById("spec-form");
  if (!form || form.dataset.runPolicyControls === "1") return;
  form.dataset.runPolicyControls = "1";

  const modelLabel = form.elements.llm_profile && form.elements.llm_profile.closest("label");
  if (!modelLabel) return;

  let anchor = modelLabel;
  const addPolicySelect = (name, title, help, options, defaultValue) => {
    if (form.elements[name]) {
      const existing = form.elements[name];
      anchor = existing.closest("label") || anchor;
      return existing;
    }
    const label = document.createElement("label");
    label.className = "llm-only";
    label.title = help;
    label.append(title + " ");

    const select = document.createElement("select");
    select.name = name;
    for (const [value, text] of options) {
      const option = document.createElement("option");
      option.value = value;
      option.textContent = text;
      option.selected = value === defaultValue;
      select.appendChild(option);
    }
    label.appendChild(select);
    anchor.insertAdjacentElement("afterend", label);
    anchor = label;
    return select;
  };

  const playStyle = addPolicySelect(
    "play_style",
    "Play style",
    "How the agent balances story progression, exploration, party building, and optional opportunities.",
    [
      ["adventure", "Adventure · natural play"],
      ["speedrun", "Speedrun · progression first"],
      ["completionist", "Completionist · explore and collect"],
      ["team_builder", "Team Builder · catches and training"],
    ],
    "adventure",
  );

  const riskTolerance = addPolicySelect(
    "risk_tolerance",
    "Risk tolerance",
    "How early the agent values a free recovery stop before risking more battles.",
    [
      ["aggressive", "Aggressive · push on longer"],
      ["balanced", "Balanced · protect progress"],
      ["cautious", "Cautious · heal early and often"],
    ],
    "balanced",
  );

  const wildEncounters = addPolicySelect(
    "wild_encounters",
    "Wild encounters",
    "Whether route travel may flee wild battles or must fight every encounter for extra training.",
    [
      ["planner", "Planner decides · fight or flee"],
      ["fight", "Fight every encounter · never flee"],
    ],
    "planner",
  );

  // The backend contract remains FPS (0 means flat out), but the operator
  // should choose a human play-speed multiplier instead of an arbitrary rate.
  const oldFPS = form.elements.fps;
  if (oldFPS && oldFPS.tagName !== "SELECT") {
    const speed = document.createElement("select");
    speed.name = "fps";
    const current = Number(oldFPS.value || 60);
    const speeds = [
      ["60", "1× · 60 FPS"],
      ["120", "2× · 120 FPS"],
      ["240", "4× · 240 FPS"],
      ["480", "8× · 480 FPS"],
      ["0", "Max · uncapped"],
    ];
    for (const [value, text] of speeds) {
      const option = document.createElement("option");
      option.value = value;
      option.textContent = text;
      option.selected = Number(value) === current;
      speed.appendChild(option);
    }
    const fpsLabel = oldFPS.closest("label");
    oldFPS.replaceWith(speed);
    if (fpsLabel && fpsLabel.firstChild && fpsLabel.firstChild.nodeType === Node.TEXT_NODE) {
      fpsLabel.firstChild.textContent = "Play speed ";
    }
  }

  // ui.js owns the queue form and builds its JSON explicitly rather than from
  // FormData. Keep that code unchanged and extend only the /v1/specs request.
  // The wrapper composes with the existing fetch policy installed by index.html.
  const nativeFetch = window.fetch.bind(window);
  window.fetch = (input, init) => {
    const url = typeof input === "string" ? input : (input && input.url) || "";
    const method = String((init && init.method) || "GET").toUpperCase();
    if (url === "/v1/specs" && method === "POST" && init && typeof init.body === "string") {
      try {
        const spec = JSON.parse(init.body);
        if (spec && spec.planner === "llm") {
          spec.play_style = playStyle.value || "adventure";
          spec.risk_tolerance = riskTolerance.value || "balanced";
          spec.wild_encounters = wildEncounters.value || "planner";
        } else if (spec) {
          delete spec.play_style;
          delete spec.risk_tolerance;
          delete spec.wild_encounters;
        }
        init = { ...init, body: JSON.stringify(spec) };
      } catch (_) {
        // Preserve the original request if another caller supplied non-JSON.
      }
    }
    return nativeFetch(input, init);
  };
})();
