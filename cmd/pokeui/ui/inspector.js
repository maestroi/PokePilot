(() => {
  "use strict";

  const root = document.getElementById("run-inspector");
  if (!root) return;
  root.innerHTML = `
    <section id="run-timeline" class="timeline-bay" aria-labelledby="timeline-title">
      <header><div><span class="section-kicker">Playback</span><h2 id="timeline-title">Run timeline</h2></div><div class="timeline-toolbar"><button id="pp-return-live" class="quiet-button" type="button" hidden>Return to live</button><button id="pp-replay" class="quiet-button" type="button">Generate replay</button><span id="pp-replay-status" class="inspect-status"></span></div></header>
      <video id="pp-video" controls preload="metadata" hidden></video><label id="pp-scrubber-wrap" class="replay-scrubber" hidden><span>Replay position</span><input id="pp-scrubber" type="range" min="0" max="1000" value="0"></label>
      <div id="pp-timeline-track" class="timeline-track" role="group" aria-label="Recorded run events"></div>
      <div class="timeline-legend"><span>○ Decision</span><span>□ Checkpoint</span><span>○ Progress</span><span>○ Failure</span></div>
    </section>
    <section id="run-story" class="story-bay" aria-labelledby="story-title">
      <header><div><span class="section-kicker">Chronology</span><h2 id="story-title">Run Story</h2></div><span class="inspect-status">Recorded events</span></header>
      <div id="pp-story" class="story-list"><p class="empty">Select a run to browse its recorded events.</p></div>
    </section>
    <aside id="evidence-drawer" class="evidence-drawer" aria-labelledby="evidence-title" hidden>
      <header class="evidence-head"><div><span class="section-kicker">Debug</span><h2 id="evidence-title">Evidence</h2></div><button id="pp-close-evidence" class="icon-button" type="button" aria-label="Close evidence">×</button></header>
      <div class="evidence-grid"><section><h3>Run facts</h3><dl id="pp-meta" class="kv"></dl><div class="inspect-actions"><button id="pp-investigate" class="primary-action" type="button">Investigate with AI</button><span id="pp-investigate-status" class="inspect-status"></span></div></section><section><h3>Artifacts</h3><p id="pp-art-empty" class="inspect-status">No artifacts recorded.</p><div id="pp-art-table" class="inspect-table-wrap" hidden><table><thead><tr><th>Name</th><th>Type</th><th>Size</th><th>SHA-256</th><th></th></tr></thead><tbody id="pp-artifacts"></tbody></table></div><details class="plan-raw"><summary>Raw debug bundle</summary><pre id="pp-debug"></pre></details></section></div>
    </aside>`;

  const byID = (id) => document.getElementById(id);
  const track = byID("pp-timeline-track");
  const story = byID("pp-story");
  const replayButton = byID("pp-replay");
  const replayStatus = byID("pp-replay-status");
  const returnLive = byID("pp-return-live");
  const video = byID("pp-video");
  const scrubber = byID("pp-scrubber");
  const scrubberWrap = byID("pp-scrubber-wrap");
  const evidence = byID("evidence-drawer");
  const meta = byID("pp-meta");
  const debugPre = byID("pp-debug");
  const artifactBody = byID("pp-artifacts");
  const artifactTable = byID("pp-art-table");
  const artifactEmpty = byID("pp-art-empty");
  const investigate = byID("pp-investigate");
  const investigateStatus = byID("pp-investigate-status");
  let runID = "";
  let debug = null;
  let reproSource = null;
  let checkpoints = [];
  let events = [];
  let selectedEvent = -1;
  let replayPoll = 0;
  let followingLive = true;
  const escURL = encodeURIComponent;
  const text = (v) => v === undefined || v === null || v === "" ? "—" : String(v);
  const html = (v) => String(v ?? "").replace(/[&<>"']/g, (c) => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));
  const fmtSize = (raw) => { let n=Number(raw||0); if(!n)return "—"; const u=["B","KiB","MiB","GiB"]; let i=0; while(n>=1024&&i<u.length-1){n/=1024;i++} return `${n>=10||i===0?n.toFixed(0):n.toFixed(1)} ${u[i]}`; };

  async function json(url, options) {
    const res = await fetch(url, { cache: "no-store", ...options });
    let body = null; try { body = await res.json(); } catch (_) {}
    if (!res.ok) throw new Error((body && body.error) || `${res.status} ${res.statusText}`);
    return body;
  }
  function stopReplayPoll() { if (replayPoll) clearTimeout(replayPoll); replayPoll = 0; }
  function eventFrame(event) { return Number(event.frame || 0); }
  function eventRound(event) { return Number(event.round || 0); }
  function eventKind(event) {
    const value = String(event.kind || event.type || "event").toLowerCase();
    if (value.includes("fail") || value.includes("lost") || value.includes("error")) return "failure";
    if (value.includes("checkpoint")) return "checkpoint";
    if (value.includes("progress") || value.includes("finish")) return "progress";
    if (value.includes("decision")) return "decision";
    return "event";
  }
  function eventTitle(event) {
    if (event.checkpoint) return `Checkpoint${event.round ? ` · round ${event.round}` : ""}`;
    return event.decision || event.message || event.progress || event.question || event.type || "Recorded event";
  }
  function eventDetail(event) {
    const values = [event.question && event.decision ? event.question : "", event.message, event.progress].filter(Boolean);
    return values.join(" · ") || (event.checkpoint ? (event.replayable ? "Paired agent knowledge is available." : "This checkpoint cannot start an LLM run.") : "No additional semantic detail was persisted.");
  }
  function normalizeEvents(debugView, checkpointView) {
    const timeline = Array.isArray(debugView && debugView.timeline) ? debugView.timeline.map((event) => ({...event})) : [];
    const cps = Array.isArray(checkpointView && checkpointView.checkpoints) ? checkpointView.checkpoints : [];
    for (const cp of cps) timeline.push({...cp, kind:"checkpoint", type:"checkpoint", checkpoint:cp.name});
    return timeline.sort((a,b) => eventFrame(a)-eventFrame(b) || eventRound(a)-eventRound(b) || eventKind(a).localeCompare(eventKind(b)));
  }
  function markerSymbol(kind) { return kind === "checkpoint" ? "□" : kind === "failure" ? "!" : kind === "progress" ? "✓" : "○"; }
  function renderTimeline() {
    if (!events.length) { track.innerHTML = '<p class="empty">No semantic events were persisted for this run.</p>'; return; }
    const max = Math.max(1, ...events.map(eventFrame));
    track.innerHTML = events.map((event, index) => {
      const kind = eventKind(event), at = eventFrame(event) ? Math.max(2, Math.min(98, 100 * eventFrame(event) / max)) : Math.max(2, 100 * index / Math.max(1, events.length - 1));
      return `<button type="button" class="timeline-marker ${kind}" style="left:${at}%" data-event-index="${index}" aria-label="${html(eventTitle(event))}" aria-current="${index===selectedEvent}">${markerSymbol(kind)}</button>`;
    }).join("");
  }
  function renderStory() {
    if (!events.length) { story.innerHTML = '<p class="empty">No recorded events. Raw run evidence may still be available.</p>'; return; }
    story.innerHTML = events.map((event,index) => {
      const kind=eventKind(event), when=eventFrame(event)?`frame ${eventFrame(event).toLocaleString()}`:(eventRound(event)?`round ${eventRound(event)}`:"recorded"), cp=event.checkpoint;
      const restart = cp ? `<button type="button" class="quiet-button" data-repro="${html(cp)}" ${event.replayable?"":"disabled"}>Start a new run from here</button>` : "";
      const fail = kind === "failure" ? '<button type="button" class="primary-action" data-investigate-event>Investigate with AI</button>' : "";
      return `<div class="story-entry" tabindex="0" role="button" data-event-index="${index}" aria-current="${index===selectedEvent}"><span class="story-when">${html(when)}</span><span class="story-symbol">${markerSymbol(kind)}</span><span class="story-copy"><strong>${html(eventTitle(event))}</strong><span>${html(eventDetail(event))}</span></span><span class="story-actions">${restart}${fail}<button type="button" class="quiet-button" data-evidence>Evidence</button></span>${index===selectedEvent?`<div class="story-detail">${html(eventDetail(event))}</div>`:""}</div>`;
    }).join("");
  }
  function selectEvent(index) {
    if (!Number.isInteger(index) || index < 0 || index >= events.length) return;
    selectedEvent = index; followingLive = false; returnLive.hidden = false;
    const maxFrame = Math.max(0, ...events.map(eventFrame));
    if (!video.hidden && video.duration > 0 && maxFrame > 0) video.currentTime = video.duration * eventFrame(events[index]) / maxFrame;
    renderTimeline(); renderStory();
    story.querySelector(`[data-event-index="${index}"]`)?.scrollIntoView({block:"nearest",behavior:"smooth"});
  }
  function renderMeta() {
    const run = (debug && debug.run) || {}, finish=(debug&&debug.finish)||{}, summary=(debug&&debug.summary)||{};
    const rows=[["Run ID",runID],["Status",run.status],["Goal",run.goal],["Outcome",finish.reason||run.reason],["Detail",finish.detail||run.detail],["Location",run.map!=null?`${run.map} · ${run.x},${run.y}`:""],["Frame",run.frame],["Runner",finish.runner_version],["Progress",summary.progress_known?(summary.progressed?"yes":"no"):"unknown"],["repro source",reproSource&&`${reproSource.source_run_id} · attempt ${reproSource.source_attempt} · ${reproSource.checkpoint}`]];
    meta.innerHTML=rows.filter(([,v])=>v!==""&&v!=null).map(([k,v])=>`<dt>${html(k)}</dt><dd>${html(v)}</dd>`).join("");
    debugPre.textContent=JSON.stringify(debug||{},null,2);
  }
  function renderArtifacts(data) {
    const list=Array.isArray(data&&data.artifacts)?data.artifacts:[]; artifactEmpty.hidden=Boolean(list.length); artifactTable.hidden=!list.length;
    artifactBody.innerHTML=list.map((a)=>`<tr><td>${html(a.name)}</td><td>${html(a.media_type||"binary")}</td><td>${fmtSize(a.size)}</td><td title="${html(a.sha256)}">${html((a.sha256||"—").slice(0,12))}</td><td><a href="/v1/runs/${escURL(runID)}/artifacts/${escURL(a.name)}/content" download="${html(a.name)}">Download</a></td></tr>`).join("");
  }
  function renderReplay(status) {
    const state=(status&&status.state)||"missing"; replayButton.hidden=false; replayButton.disabled=false; video.hidden=true; scrubberWrap.hidden=true;
    if(state==="ready"){replayStatus.textContent=status.size?`Ready · ${fmtSize(status.size)}`:"Ready"; replayButton.hidden=true; const src=`/v1/runs/${escURL(runID)}/replay/video`; if(video.dataset.run!==runID){video.src=src;video.dataset.run=runID;video.load()} video.hidden=false;scrubberWrap.hidden=false;return}
    if(state==="generating"){replayStatus.textContent="Generating deterministic replay…";replayButton.disabled=true;stopReplayPoll();replayPoll=setTimeout(()=>loadReplayStatus(runID),1500);return}
    if(state==="disabled"){replayStatus.textContent=status.error||"Replay service unavailable";replayButton.hidden=true;return}
    if(state==="error"){replayStatus.textContent=status.error||"Replay generation failed";replayButton.textContent="Retry replay";return}
    replayStatus.textContent="Ready to generate from the recorded run.";replayButton.textContent="Generate replay";
  }
  async function loadReplayStatus(id) { try { const status=await json(`/v1/runs/${escURL(id)}/replay/status`); if(id===runID)renderReplay(status); } catch(err){if(id===runID){replayStatus.textContent=err.message;replayButton.hidden=true}} }
  async function selectRun(id) {
    stopReplayPoll(); runID=id; debug=null; reproSource=null; checkpoints=[]; events=[]; selectedEvent=-1; followingLive=true; returnLive.hidden=true; evidence.hidden=true; video.pause();video.removeAttribute("src");video.dataset.run="";video.hidden=true;scrubber.value="0";scrubberWrap.hidden=true;
    if(!id){track.innerHTML='<p class="empty">Select a run to browse recorded events.</p>';story.innerHTML='<p class="empty">Select a run to browse its recorded events.</p>';replayButton.hidden=true;return}
    track.innerHTML='<p class="empty">Loading recorded events…</p>'; story.innerHTML='<p class="empty">Loading Run Story…</p>'; replayButton.hidden=false;replayButton.disabled=true;replayStatus.textContent="Loading…";
    try{
      const [debugView,artifactView,checkpointView,sourceView]=await Promise.all([json(`/v1/runs/${escURL(id)}/debug`),json(`/v1/runs/${escURL(id)}/artifacts`),json(`/v1/runs/${escURL(id)}/checkpoints`).catch(()=>({checkpoints:[]})),json(`/v1/runs/${escURL(id)}/repro-source`).catch(()=>null)]);
      if(id!==runID)return; debug=debugView;reproSource=sourceView;checkpoints=checkpointView.checkpoints||[];events=normalizeEvents(debugView,checkpointView);selectedEvent=events.length-1;renderTimeline();renderStory();renderMeta();renderArtifacts(artifactView);
      const replayable=(artifactView.artifacts||[]).some((a)=>a.replayable); if(replayable)await loadReplayStatus(id); else{replayButton.hidden=true;replayStatus.textContent="No run recording is available."}
    }catch(err){if(id===runID){track.innerHTML=`<p class="empty">Timeline unavailable: ${html(err.message)}</p>`;story.innerHTML=`<p class="empty">Run Story unavailable: ${html(err.message)}</p>`;replayButton.hidden=true}}
  }
  async function startFromCheckpoint(name, button) {
    // The wall marks a checkpoint replayable only when paired agent knowledge exists.
    if(!runID||!name)return; button.disabled=true;button.textContent="Starting…";
    try{const queued=await json(`/v1/runs/${escURL(runID)}/repro`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({checkpoint:name})});button.textContent=`Queued ${queued.run_id}`;window.dispatchEvent(new CustomEvent("pokefarm-select-run",{detail:{runId:queued.run_id}}));}catch(err){button.textContent=err.message;button.disabled=false}
  }
  async function investigateRun(button) {
    if(!runID)return;button.disabled=true;investigateStatus.textContent="Finding this run's failure group…";
    try{const groups=await json("/v1/triage");const group=(Array.isArray(groups)?groups:[]).find((g)=>(g.run_ids||[]).includes(runID));if(!group)throw new Error("No actionable failure group is linked to this run.");await json(`/v1/triage/${escURL(group.key)}/investigate`,{method:"POST"});investigateStatus.textContent="Investigation queued with this run and its evidence.";}catch(err){investigateStatus.textContent=err.message;button.disabled=false}
  }

  root.addEventListener("click",(event)=>{const marker=event.target.closest("[data-event-index]");if(marker&&!event.target.closest("[data-repro],[data-evidence],[data-investigate-event]")){selectEvent(Number(marker.dataset.eventIndex));return}const repro=event.target.closest("[data-repro]");if(repro){startFromCheckpoint(repro.dataset.repro,repro);return}if(event.target.closest("[data-evidence]")){evidence.hidden=false;return}if(event.target.closest("[data-investigate-event]")){evidence.hidden=false;investigateRun(investigate);}});
  root.addEventListener("keydown",(event)=>{if((event.key==="Enter"||event.key===" ")&&event.target.matches(".story-entry")){event.preventDefault();selectEvent(Number(event.target.dataset.eventIndex));}});
  replayButton.addEventListener("click",async()=>{replayButton.disabled=true;replayStatus.textContent="Starting replay generation…";try{renderReplay(await json(`/v1/runs/${escURL(runID)}/replay/render`,{method:"POST"}))}catch(err){replayStatus.textContent=err.message;replayButton.disabled=false}});
  scrubber.addEventListener("input",()=>{if(video.duration>0)video.currentTime=video.duration*Number(scrubber.value)/1000});
  video.addEventListener("timeupdate",()=>{if(video.duration>0)scrubber.value=String(Math.round(1000*video.currentTime/video.duration))});
  returnLive.addEventListener("click",()=>{followingLive=true;returnLive.hidden=true;if(events.length){selectedEvent=events.length-1;renderTimeline();renderStory()}});
  investigate.addEventListener("click",()=>investigateRun(investigate));
  byID("pp-close-evidence").addEventListener("click",()=>{evidence.hidden=true});
  window.addEventListener("pokefarm-select-run",(event)=>{const id=(event.detail&&event.detail.runId)||"";if(id!==runID)selectRun(id)});
  window.addEventListener("beforeunload",stopReplayPoll,{once:true});
  if (document.documentElement.dataset.selectedRun) selectRun(document.documentElement.dataset.selectedRun);
})();
