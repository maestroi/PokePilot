package main

import "bytes"

// The persisted llm_profile wire values are older than the current hardware
// policy. Keep those values stable for queued/history compatibility while the
// operator console names the physical routes they now select. This is kept as
// a small build-time patch instead of rewriting the large embedded UI assets;
// remove it when the next UI cleanup edits those assets directly.
func init() {
	oldSelect := []byte(`<label class="llm-only">Model <select name="llm_profile"><option value="auto" selected>Auto · 7900 XTX → 4090 → LAN</option><option value="gpu">Reserve 4090 · 7900 XTX only</option><option value="default">Reserve all GPUs · LAN only</option></select></label>`)
	newSelect := []byte(`<label class="llm-only">Model <select name="llm_profile"><option value="auto" selected>7900 XTX · default · CPU after 120s</option><option value="gpu">RTX 4090 · manual</option><option value="default">CPU only · manual</option></select></label>`)
	indexHTML = bytes.Replace(indexHTML, oldSelect, newSelect, 1)

	oldPolicies := []byte(`  const policies = {
    auto: { label: "Auto", route: "7900 XTX → 4090 → LAN", detail: "Preferred. Uses the dedicated GPU first, borrows the fast GPU only on failure, then LAN." },
    gpu: { label: "Reserve 4090", route: "7900 XTX only", detail: "Keeps the 4090 free for coding. No LAN fallback for this explicit GPU-only mode." },
    default: { label: "Reserve all GPUs", route: "LAN CPU only", detail: "Coding mode. New PokePilot runs do not touch either GPU." }
  };`)
	newPolicies := []byte(`  const policies = {
    auto: { label: "7900 XTX", route: "7900 XTX → CPU after 120s", detail: "Default. Calls the 7900 XTX directly; CPU is used only after a GPU transport failure or request timeout. LiteLLM is bypassed." },
    gpu: { label: "RTX 4090", route: "4090 only", detail: "Manual 4090 route. It is never borrowed automatically by the default route." },
    default: { label: "CPU only", route: "LAN CPU only", detail: "Manual CPU/LAN route. No GPU is used." }
  };`)
	indexHTML = bytes.Replace(indexHTML, oldPolicies, newPolicies, 1)

	oldFacts := []byte(`<div><span>7900 XTX</span><strong>qwen3.8-27B · preferred</strong></div><div><span>4090</span><strong>qwen3.8-27B · overflow</strong></div><div><span>LAN CPU</span><strong>qwen 4B · final fallback</strong></div>`)
	newFacts := []byte(`<div><span>7900 XTX</span><strong>qwen3.8-27B · direct default</strong></div><div><span>4090</span><strong>qwen3.8-27B · manual only</strong></div><div><span>LAN CPU</span><strong>qwen 4B · 120s fallback / manual</strong></div>`)
	indexHTML = bytes.Replace(indexHTML, oldFacts, newFacts, 1)

	oldLabels := []byte(`    switch ((r.llm_profile || "").toLowerCase()) {
      case "gpu": return "GPU";
      case "auto": return "Auto (GPU → LAN)";
      case "default": return "Default (LAN)";
      default: return r.planner === "llm" ? "Default (LAN)" : "";
    }`)
	newLabels := []byte(`    switch ((r.llm_profile || "").toLowerCase()) {
      case "gpu": return "RTX 4090";
      case "auto": return "7900 XTX → CPU";
      case "default": return "CPU only";
      default: return r.planner === "llm" ? "7900 XTX → CPU" : "";
    }`)
	uiJS = bytes.Replace(uiJS, oldLabels, newLabels, 1)

	oldStats := []byte(`    const modelLine = (s.model ? s.model : "—") + (s.backend ? " · " + s.backend : "");
    const nums = row("round", s.round + (s.rounds_left ? ` + "` (${s.rounds_left} left)`" + ` : ""))
      + row("model", modelLine)
      + row("repeat picks", ` + "`${s.repeats} of ${s.rounds}`" + `, s.rounds > 3 && s.repeats * 2 >= s.rounds)
      + row("call latency", ` + "`${seconds(s.last_seconds)} last / ${seconds(s.avg_seconds)} all avg`" + `)
      + row("latency avg", ` + "`${average(s.successful_avg_seconds)} ok / ${average(s.rejected_avg_seconds)} rejected / ${average(s.strategic_avg_seconds)} strategist`" + `)
      + row("offered", ` + "`${s.avg_offered.toFixed(1)} avg`" + `)
      + row("tokens", ` + "`${s.prompt_tokens} / ${s.completion_tokens}`" + `)`)
	newStats := []byte(`    const modelLine = (s.model ? s.model : "—") + (s.backend ? " · " + s.backend : "");
    const servedBy = s.endpoint ? row("served by", ` + "`${s.response_model || s.model || "—"} @ ${s.endpoint}`" + `) : "";
    const cached = s.last_cached_prompt_tokens ? ` + "` · ${s.last_cached_prompt_tokens} cached`" + ` : "";
    const timingRows = row("last call tokens", ` + "`${s.last_prompt_tokens || 0} prompt / ${s.last_completion_tokens || 0} completion${cached}`" + `)
      + (s.timing_source ? row("prefill", ` + "`${(Number(s.prefill_ms || 0) / 1000).toFixed(2)}s · ${Number(s.prefill_tps || 0).toFixed(0)} tok/s`" + `) : "")
      + (s.timing_source ? row("decode", ` + "`${(Number(s.decode_ms || 0) / 1000).toFixed(2)}s · ${Number(s.decode_tps || 0).toFixed(1)} tok/s`" + `) : "")
      + (s.timing_source ? row("server overhead", ` + "`${(Number(s.overhead_ms || 0) / 1000).toFixed(2)}s · ${s.timing_source}`" + `) : "");
    const nums = row("round", s.round + (s.rounds_left ? ` + "` (${s.rounds_left} left)`" + ` : ""))
      + row("model", modelLine)
      + servedBy
      + row("repeat picks", ` + "`${s.repeats} of ${s.rounds}`" + `, s.rounds > 3 && s.repeats * 2 >= s.rounds)
      + row("call latency", ` + "`${seconds(s.last_seconds)} last / ${seconds(s.avg_seconds)} all avg`" + `)
      + row("latency avg", ` + "`${average(s.successful_avg_seconds)} ok / ${average(s.rejected_avg_seconds)} rejected / ${average(s.strategic_avg_seconds)} strategist`" + `)
      + timingRows
      + row("offered", ` + "`${s.avg_offered.toFixed(1)} avg`" + `)
      + row("tokens", ` + "`${s.prompt_tokens} / ${s.completion_tokens}`" + `)`)
	uiJS = bytes.Replace(uiJS, oldStats, newStats, 1)
}
