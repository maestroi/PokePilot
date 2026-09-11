(()=>{
  "use strict";

  const nativeFetch=window.fetch.bind(window);
  const PAGE=25;
  const shared={now:0,runs:[],workers:[]};
  let activeRuns=[];
  let historyRows=[];
  let pinnedRun=null;
  let historyTotal=0;
  let historyPage=0;
  let historyLoaded=false;
  let historyFacets={outcomes:[],hows:[],starters:[]};
  let selected="";
  let loadSerial=0;
  const filters={outcome:"",how:"",starter:""};

  const esc=(v)=>String(v??"").replace(/[&<>"']/g,(c)=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));
  const activeURL="/v1/dashboard?active=1";

  function runID(run){return run&&run.run_id||""}
  function rebuildShared(){
    const out=[];
    const seen=new Set();
    for(const run of [...activeRuns,...historyRows,pinnedRun]){
      const id=runID(run);
      if(!id||seen.has(id))continue;
      seen.add(id);out.push(run);
    }
    shared.runs=out;
  }

  function nativeJSON(url,init){
    return nativeFetch(url,init).then(async(res)=>{
      if(!res.ok)throw new Error(`HTTP ${res.status}`);
      return res.json();
    });
  }

  function historyURL(page){
    const q=new URLSearchParams();
    q.set("status","done");
    q.set("limit",String(PAGE));
    q.set("offset",String(Math.max(0,page)*PAGE));
    q.set("facets","1");
    if(filters.outcome)q.set("outcome",filters.outcome);
    if(filters.how)q.set("how",filters.how);
    if(filters.starter)q.set("starter",filters.starter);
    return "/v1/dashboard?"+q.toString();
  }

  async function loadHistory(page=historyPage){
    const serial=++loadSerial;
    const data=await nativeJSON(historyURL(page),{cache:"no-store"});
    if(serial!==loadSerial)return;
    historyTotal=Number(data.total||0);
    const pages=Math.max(1,Math.ceil(historyTotal/PAGE));
    historyPage=Math.max(0,Math.min(page,pages-1));
    if(historyPage!==page&&historyTotal>0){
      return loadHistory(historyPage);
    }
    historyRows=Array.isArray(data.runs)?data.runs:[];
    historyFacets=data.history_facets||historyFacets;
    historyLoaded=true;
    if(selected){
      const hit=historyRows.find((run)=>runID(run)===selected);
      if(hit)pinnedRun=hit;
    }
    rebuildShared();
    renderHistoryOwner();
  }

  async function ensurePinned(id){
    id=String(id||"").trim();
    if(!id)return;
    if(activeRuns.some((run)=>runID(run)===id)||historyRows.some((run)=>runID(run)===id))return;
    if(pinnedRun&&runID(pinnedRun)===id)return;
    try{
      const data=await nativeJSON("/v1/runs/"+encodeURIComponent(id),{cache:"no-store"});
      if(data&&data.run){pinnedRun=data.run;rebuildShared();}
    }catch(_){ }
  }

  function asDashboardResponse(){
    return {ok:true,status:200,json:async()=>shared};
  }

  async function liveDashboard(){
    const previous=new Set(activeRuns.map(runID));
    const response=await nativeFetch(activeURL,{cache:"no-store"});
    if(!response.ok)return response;
    const data=await response.json();
    activeRuns=Array.isArray(data.runs)?data.runs:[];
    shared.now=data.now||0;
    shared.wall_version=data.wall_version||"";
    shared.workers=Array.isArray(data.workers)?data.workers:[];

    if(!historyLoaded)await loadHistory(0);
    const requested=new URL(location.href).searchParams.get("run")||localStorage.getItem("pokefarm-selected-run")||"";
    if(requested)await ensurePinned(requested);

    const current=new Set(activeRuns.map(runID));
    let settled=false;
    for(const id of previous){if(id&&!current.has(id)){settled=true;break;}}
    if(settled){
      try{await loadHistory(historyPage);}catch(_){ }
    }
    rebuildShared();
    return asDashboardResponse();
  }

  window.fetch=(input,init)=>{
    const raw=typeof input==="string"?input:(input&&input.url)||"";
    const method=String((init&&init.method)||(input&&input.method)||"GET").toUpperCase();
    try{
      const url=new URL(raw,location.origin);
      if(method==="GET"&&url.origin===location.origin&&url.pathname==="/v1/dashboard"&&!url.search){
        return liveDashboard();
      }
      if(method==="DELETE"&&url.origin===location.origin&&url.pathname.startsWith("/v1/runs/")){
        return nativeFetch(input,init).then((res)=>{
          if(res.ok){
            const id=decodeURIComponent(url.pathname.slice("/v1/runs/".length));
            historyRows=historyRows.filter((run)=>runID(run)!==id);
            if(pinnedRun&&runID(pinnedRun)===id)pinnedRun=null;
            if(historyTotal>0)historyTotal--;
            rebuildShared();
            renderHistoryOwner();
            loadHistory(historyPage).catch(()=>{});
          }
          return res;
        });
      }
    }catch(_){ }
    return nativeFetch(input,init);
  };

  function whenLabel(run){
    const sec=Number(run&&run.ended_at||0);
    if(!sec)return "—";
    try{return new Date(sec*1000).toLocaleString();}catch(_){return "—";}
  }
  function howLabel(run){return run&&run.planner==="scripted"?"walk":"play"}
  function starterLabel(run){return String(run&&run.starter|| (run&&run.planner==="scripted"?"squirtle":"LLM picks"));}
  function whereLabel(run){
    if(run&&run.planner==="scripted"&&run.dest)return run.dest;
    const map=Number(run&&run.map||0).toString(16).padStart(2,"0");
    return `map 0x${map} · ${Number(run&&run.x||0)},${Number(run&&run.y||0)}`;
  }
  function issueHTML(issue){
    if(!issue||!issue.issue_number)return "";
    const label=`Issue #${issue.issue_number}`;
    try{
      const url=new URL(issue.issue_url||"");
      if(url.protocol==="http:"||url.protocol==="https:")return `<a class="issue-a" href="${esc(url.href)}" target="_blank" rel="noopener">${esc(label)}</a>`;
    }catch(_){ }
    return `<span class="chip">${esc(label)}</span>`;
  }
  function outcomeLabel(run){return String(run&&run.detail||run&&run.reason||"done");}

  function filterButton(group,value,label){
    const on=filters[group]===value;
    return `<button type="button" class="filter" data-paged-filter-group="${group}" data-paged-filter-value="${esc(value)}" aria-pressed="${on}">${esc(label)}</button>`;
  }

  function renderFilters(){
    const el=document.getElementById("hist-filters");if(!el)return;
    const outcomes=historyFacets.outcomes||[];
    const hows=historyFacets.hows||[];
    const starters=historyFacets.starters||[];
    if(!outcomes.length&&!hows.length&&!starters.length){el.innerHTML='<i data-paged-history-owner hidden></i>';return;}
    let html='<i data-paged-history-owner hidden></i>';
    html+=`<div class="filter-group"><span>ended</span>${filterButton("outcome","","all")}`+outcomes.map((v)=>filterButton("outcome",v,v)).join("")+`</div>`;
    html+=`<div class="filter-group"><span>how</span>${filterButton("how","","all")}`+hows.map((v)=>filterButton("how",v,v)).join("")+`</div>`;
    html+=`<div class="filter-group"><span>starter</span>${filterButton("starter","","all")}`+starters.map((v)=>filterButton("starter",v,v)).join("")+`</div>`;
    el.innerHTML=html;
  }

  function renderRows(){
    const el=document.getElementById("history");if(!el)return;
    const hasFilter=Boolean(filters.outcome||filters.how||filters.starter);
    if(!historyRows.length){
      el.innerHTML=`<i data-paged-history-owner hidden></i><p class="empty">${historyTotal===0&&hasFilter?"No runs match these filters":"Nothing finished yet"}</p>`;
      return;
    }
    el.innerHTML='<i data-paged-history-owner hidden></i>'+historyRows.map((run)=>{
      const id=runID(run);const chosen=id===selected?" selected":"";const out=outcomeLabel(run);
      return `<div class="hist-row${chosen}"><button type="button" class="hist" data-run="${esc(id)}"><span class="hist-who"><span class="hist-when">${esc(whenLabel(run))}</span><span class="hist-id">${esc(id)}</span></span><span class="chips"><span class="chip">${esc(howLabel(run))}</span><span class="chip">${esc(starterLabel(run))}</span></span><span class="hist-where">${esc(whereLabel(run))}</span><span class="hist-out">${issueHTML(run.issue)}<span class="hist-outcome" title="${esc(out)}">${esc(out)}</span></span></button><button type="button" class="hist-del" data-delete="${esc(id)}">Delete</button></div>`;
    }).join("");
  }

  function renderPager(){
    const el=document.getElementById("hist-pager");if(!el)return;
    const pages=Math.max(1,Math.ceil(historyTotal/PAGE));
    const start=historyTotal?historyPage*PAGE+1:0;
    const end=Math.min(historyTotal,(historyPage+1)*PAGE);
    el.innerHTML=`<i data-paged-history-owner hidden></i><span class="pager-count">${start}–${end} of ${historyTotal}</span><button type="button" class="pager-btn" data-paged-page="prev" ${historyPage===0?"disabled":""}>← prev</button><span class="pager-page">${historyPage+1} / ${pages}</span><button type="button" class="pager-btn" data-paged-page="next" ${historyPage>=pages-1?"disabled":""}>next →</button>`;
  }

  function renderHistoryOwner(){renderFilters();renderRows();renderPager();}

  function restoreIfOverwritten(){
    for(const id of ["hist-filters","history","hist-pager"]){
      const el=document.getElementById(id);
      if(el&&!el.querySelector("[data-paged-history-owner]")){renderHistoryOwner();break;}
    }
  }

  document.addEventListener("click",async(ev)=>{
    const filter=ev.target.closest("[data-paged-filter-group]");
    if(filter){
      ev.preventDefault();
      const group=filter.getAttribute("data-paged-filter-group");
      if(Object.prototype.hasOwnProperty.call(filters,group))filters[group]=filter.getAttribute("data-paged-filter-value")||"";
      historyPage=0;
      try{await loadHistory(0);}catch(_){renderHistoryOwner();}
      return;
    }
    const pager=ev.target.closest("[data-paged-page]");
    if(pager){
      ev.preventDefault();
      const dir=pager.getAttribute("data-paged-page");
      const pages=Math.max(1,Math.ceil(historyTotal/PAGE));
      const target=dir==="prev"?Math.max(0,historyPage-1):Math.min(pages-1,historyPage+1);
      if(target!==historyPage){try{await loadHistory(target);}catch(_){ }}
    }
  });

  window.addEventListener("pokefarm-select-run",(ev)=>{
    selected=String(ev.detail&&ev.detail.runId||"");
    const hit=historyRows.find((run)=>runID(run)===selected);
    if(hit)pinnedRun=hit;
    else if(selected&&!activeRuns.some((run)=>runID(run)===selected))ensurePinned(selected).then(()=>rebuildShared());
    renderHistoryOwner();
  });

  const startObserver=()=>{
    const targets=["hist-filters","history","hist-pager"].map((id)=>document.getElementById(id)).filter(Boolean);
    if(!targets.length)return;
    const observer=new MutationObserver(()=>queueMicrotask(restoreIfOverwritten));
    for(const target of targets)observer.observe(target,{childList:true});
    restoreIfOverwritten();
  };
  if(document.readyState==="loading")document.addEventListener("DOMContentLoaded",startObserver,{once:true});else startObserver();
})();
