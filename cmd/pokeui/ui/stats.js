(()=>{
  "use strict";

  const style=document.createElement("style");
  style.textContent=`
    .outcome-stats{grid-column:1/-1}
    .outcome-stats .ops-inner{display:grid;gap:14px}
    .outcome-head{display:flex;align-items:flex-start;justify-content:space-between;gap:12px;flex-wrap:wrap}
    .outcome-head h2{margin:0;color:var(--blue-dark);font-size:18px}
    .outcome-note{color:var(--muted);font-size:12px;max-width:72ch}
    .outcome-kpis{display:grid;grid-template-columns:repeat(6,minmax(110px,1fr));gap:8px}
    .outcome-kpi{min-width:0;padding:10px 12px;border:1px solid var(--line);border-radius:9px;background:var(--raised)}
    .outcome-kpi .k{color:var(--muted);font-size:11px;text-transform:uppercase;letter-spacing:.05em;font-weight:800}
    .outcome-kpi .v{margin-top:3px;font-size:22px;font-weight:850;line-height:1.15;color:var(--ink)}
    .outcome-kpi .s{margin-top:3px;color:var(--muted);font-size:11px}
    .outcome-grid{display:grid;grid-template-columns:minmax(240px,1fr) minmax(240px,1fr);gap:12px}
    .outcome-block{padding:10px 12px;border:1px solid var(--line);border-radius:9px;background:var(--raised);min-width:0}
    .outcome-block h3{margin:0 0 9px;color:var(--blue-dark);font-size:12px;text-transform:uppercase;letter-spacing:.05em}
    .outcome-bars{display:grid;gap:6px}
    .outcome-bar-row{display:grid;grid-template-columns:5.7rem minmax(0,1fr) auto;gap:8px;align-items:center;font-size:12px}
    .outcome-bar-label{color:var(--muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
    .outcome-track{height:8px;border-radius:999px;background:var(--bg-deep);overflow:hidden}
    .outcome-fill{height:100%;min-width:0;background:var(--blue);border-radius:inherit}
    .outcome-bar-n{font:12px/1 var(--mono);color:var(--ink)}
    .endless-wrap{overflow:auto;border:1px solid var(--line);border-radius:9px}
    .endless-table{width:100%;border-collapse:collapse;min-width:760px;font-size:12px}
    .endless-table th,.endless-table td{padding:8px 9px;border-bottom:1px solid var(--line);text-align:left;vertical-align:top}
    .endless-table th{position:sticky;top:0;background:var(--surface-2);color:var(--muted);text-transform:uppercase;letter-spacing:.04em;font-size:10px}
    .endless-table tr:last-child td{border-bottom:0}
    .endless-goal{max-width:36rem;white-space:normal;color:var(--ink)}
    .endless-key{font-family:var(--mono);color:var(--muted)}
    .endless-reasons{color:var(--muted);white-space:nowrap}
    .outcome-empty,.outcome-error{margin:0;color:var(--muted)}
    .outcome-error{color:var(--err-ink)}
    @media(max-width:1100px){.outcome-kpis{grid-template-columns:repeat(3,minmax(110px,1fr))}}
    @media(max-width:760px){.outcome-kpis{grid-template-columns:repeat(2,minmax(110px,1fr))}.outcome-grid{grid-template-columns:1fr}}
  `;
  document.head.appendChild(style);

  const ops=document.querySelector(".ops");
  if(!ops)return;

  const card=document.createElement("section");
  card.className="ops-card outcome-stats";
  card.innerHTML=`
    <div class="ops-inner">
      <div class="outcome-head">
        <div><h2>Run outcomes</h2><div class="outcome-note">Badge progress and objective wins are counted independently from terminal failures. Retry failures include earlier error/lost attempts that a later retry can otherwise hide.</div></div>
        <div class="outcome-note" id="outcome-status">Loading…</div>
      </div>
      <div class="outcome-kpis" id="outcome-kpis"></div>
      <div class="outcome-grid">
        <div class="outcome-block"><h3>Badge distribution</h3><div id="outcome-badges"></div></div>
        <div class="outcome-block"><h3>Terminal outcomes</h3><div id="outcome-reasons"></div></div>
      </div>
      <div class="outcome-block"><h3>Endless experiments</h3><div class="outcome-note" style="margin-bottom:8px">Successor runs with identical endless settings are grouped together, so high-goal random-seed farms can be compared as one benchmark.</div><div id="outcome-endless"></div></div>
    </div>`;
  ops.insertBefore(card,ops.firstChild);

  const esc=(v)=>String(v??"").replace(/[&<>"']/g,(c)=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));
  const pct=(n,d)=>d?`${(100*n/d).toFixed(n&&n<d?1:0)}%`:"—";
  const nfmt=(n)=>Number(n||0).toLocaleString();
  const ratio=(n,d)=>d?`${nfmt(n)} / ${nfmt(d)} · ${pct(n,d)}`:"No tracked runs";

  function renderKpis(s){
    const missing=Math.max(0,(s.settled_runs||0)-(s.usable_progress_runs||0));
    const cells=[
      ["Completed attempts",nfmt(s.completed_attempts),`${nfmt(s.settled_runs)} settled run records`],
      ["Objective wins",ratio(s.goal_wins,s.goal_tracked_runs),"structured goals with a known completion signal"],
      ["Reached ≥1 badge",ratio(s.at_least_one_badge,s.usable_progress_runs),"among runs with a final player snapshot"],
      ["Best badge count",`${nfmt(s.best_badges)} / 8`,"highest final badge count observed"],
      ["Retry failures",nfmt(s.retryable_failure_attempts),"error/lost attempts, including failures hidden by retries"],
      ["No progress data",nfmt(missing),"settled runs without a usable final player snapshot"],
    ];
    document.getElementById("outcome-kpis").innerHTML=cells.map(([k,v,sub])=>`<div class="outcome-kpi"><div class="k">${esc(k)}</div><div class="v">${esc(v)}</div><div class="s">${esc(sub)}</div></div>`).join("");
  }

  function renderBars(target,rows,label,total){
    const el=document.getElementById(target);
    if(!rows||!rows.length){el.innerHTML='<p class="outcome-empty">No data yet.</p>';return}
    const max=Math.max(1,...rows.map((r)=>Number(r.count||0)));
    el.innerHTML=`<div class="outcome-bars">${rows.map((r)=>{
      const name=label(r);
      const count=Number(r.count||0);
      const width=count?Math.max(2,100*count/max):0;
      return `<div class="outcome-bar-row"><div class="outcome-bar-label" title="${esc(name)}">${esc(name)}</div><div class="outcome-track"><div class="outcome-fill" style="width:${width}%"></div></div><div class="outcome-bar-n">${nfmt(count)}${total?` · ${pct(count,total)}`:""}</div></div>`;
    }).join("")}</div>`;
  }

  function renderEndless(rows){
    const el=document.getElementById("outcome-endless");
    if(!rows||!rows.length){el.innerHTML='<p class="outcome-empty">No endless runs recorded yet.</p>';return}
    el.innerHTML=`<div class="endless-wrap"><table class="endless-table"><thead><tr><th>Experiment</th><th>Attempts</th><th>Best</th><th>≥1 badge</th><th>Objective wins</th><th>Retry failures</th><th>Terminal outcomes</th></tr></thead><tbody>${rows.map((r)=>{
      const goal=(r.goal||r.planner||"endless run").trim();
      const config=[r.llm_profile||"",r.random_seed?"random seed":"same seed",r.max_rounds?`${r.max_rounds} rounds`:""].filter(Boolean).join(" · ");
      const reasons=(r.terminal_reasons||[]).map((x)=>`${x.name} ${x.count}`).join(" · ")||"—";
      return `<tr><td><div class="endless-goal">${esc(goal)}</div><div class="endless-key">${esc(r.key)} · ${esc(config)}</div></td><td>${nfmt(r.completed_attempts)}</td><td>${nfmt(r.best_badges)} / 8</td><td>${esc(ratio(r.at_least_one_badge,r.usable_progress_runs))}</td><td>${esc(ratio(r.goal_wins,r.goal_tracked_runs))}</td><td>${nfmt(r.retryable_failure_attempts)}</td><td class="endless-reasons">${esc(reasons)}</td></tr>`;
    }).join("")}</tbody></table></div>`;
  }

  function render(s){
    renderKpis(s);
    const badgeRows=(s.badge_distribution||[]).filter((r)=>r.count>0);
    renderBars("outcome-badges",badgeRows,(r)=>`${r.badges} badge${r.badges===1?"":"s"}`,s.usable_progress_runs||0);
    renderBars("outcome-reasons",s.terminal_reasons||[],(r)=>r.name,s.settled_runs||0);
    renderEndless(s.endless_experiments||[]);
    document.getElementById("outcome-status").textContent=`${nfmt(s.completed_attempts)} completed attempts · ${nfmt(s.settled_runs)} settled runs`;
  }

  async function refresh(){
    try{
      const res=await fetch("/v1/stats",{cache:"no-store"});
      if(!res.ok)throw new Error(`HTTP ${res.status}`);
      render(await res.json());
    }catch(err){
      document.getElementById("outcome-status").innerHTML=`<span class="outcome-error">Stats unavailable: ${esc(err.message||err)}</span>`;
    }
  }

  refresh();
  setInterval(refresh,3000);
})();
