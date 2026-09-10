(()=>{
  "use strict";

  const card=document.getElementById("analytics-outcomes");
  if(!card)return;
  function analyticsVisible(){
    const panel=card.closest("[data-console-view]");
    return !panel||!panel.hidden;
  }
  card.className="ops-card outcome-stats";
  card.innerHTML=`
    <div class="ops-inner">
      <div class="outcome-head">
        <div><h2>Run outcomes</h2><div class="outcome-note">Badge progress and objective wins are counted independently from terminal failures. Retry failures include earlier error/lost attempts that a later retry can otherwise hide.</div></div>
        <div class="outcome-note" id="outcome-status">Loading…</div>
      </div>
      <div class="outcome-summary" id="outcome-kpis"></div>
      <div class="outcome-grid">
        <div class="outcome-block"><h3>Badge distribution</h3><div id="outcome-badges"></div></div>
        <div class="outcome-block"><h3>Terminal outcomes</h3><div id="outcome-reasons"></div></div>
      </div>
      <div class="outcome-block"><h3>Endless experiments</h3><div class="outcome-note outcome-section-note">Successor runs with identical endless settings are grouped together, so high-goal random-seed farms can be compared as one benchmark.</div><div id="outcome-endless"></div></div>
    </div>`;
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
    document.getElementById("outcome-kpis").innerHTML=cells.map(([k,v,sub])=>`<div class="outcome-summary-item"><div class="k">${esc(k)}</div><div class="v">${esc(v)}</div><div class="s">${esc(sub)}</div></div>`).join("");
  }

  function renderBars(target,rows,label,total){
    const el=document.getElementById(target);
    if(!rows||!rows.length){el.innerHTML='<p class="outcome-empty">No data yet.</p>';return}
    const max=Math.max(1,...rows.map((r)=>Number(r.count||0)));
    el.innerHTML=`<div class="outcome-bars">${rows.map((r)=>{
      const name=label(r);
      const count=Number(r.count||0);
      const width=count?Math.max(2,100*count/max):0;
      return `<div class="outcome-bar-row"><div class="outcome-bar-label" title="${esc(name)}">${esc(name)}</div><progress class="outcome-track" max="100" value="${width.toFixed(1)}" aria-label="${esc(name)}">${width.toFixed(1)}%</progress><div class="outcome-bar-n">${nfmt(count)}${total?` · ${pct(count,total)}`:""}</div></div>`;
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
    if(!analyticsVisible())return;
    try{
      const res=await fetch("/v1/stats",{cache:"no-store"});
      if(!res.ok)throw new Error(`HTTP ${res.status}`);
      render(await res.json());
    }catch(err){
      document.getElementById("outcome-status").innerHTML=`<span class="outcome-error">Stats unavailable: ${esc(err.message||err)}</span>`;
    }
  }

  window.addEventListener("pokefarm-console-view",(ev)=>{
    const detail=ev.detail||{};
    if(detail.view==="analytics")refresh();
  });
  refresh();
  setInterval(refresh,3000);
})();
