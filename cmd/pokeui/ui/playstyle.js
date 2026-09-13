(() => {
  const form = document.getElementById("spec-form");
  if (!form || form.elements.play_style) return;

  const modelLabel = form.elements.llm_profile && form.elements.llm_profile.closest("label");
  if (!modelLabel) return;

  const label = document.createElement("label");
  label.className = "llm-only";
  label.title = "How the agent balances story progression, exploration, party building, and optional opportunities.";
  label.append("Play style ");

  const select = document.createElement("select");
  select.name = "play_style";
  const profiles = [
    ["adventure", "Adventure · natural play"],
    ["speedrun", "Speedrun · progression first"],
    ["completionist", "Completionist · explore and collect"],
    ["team_builder", "Team Builder · catches and training"],
  ];
  for (const [value, text] of profiles) {
    const option = document.createElement("option");
    option.value = value;
    option.textContent = text;
    option.selected = value === "adventure";
    select.appendChild(option);
  }
  label.appendChild(select);
  modelLabel.insertAdjacentElement("afterend", label);

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
        if (spec && spec.planner === "llm") spec.play_style = select.value || "adventure";
        else if (spec) delete spec.play_style;
        init = { ...init, body: JSON.stringify(spec) };
      } catch (_) {
        // Preserve the original request if another caller supplied non-JSON.
      }
    }
    return nativeFetch(input, init);
  };
})();
