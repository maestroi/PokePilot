(() => {
  "use strict";

  const root = document.getElementById("run-inspector");
  if (!root) return;
  root.innerHTML = `
    <section id="run-timeline" class="timeline-bay" aria-labelledby="timeline-title">
      <header><div><span class="section-kicker">Playback</span><h2 id="timeline-title">Run timeline</h2></div><div class="timeline-toolbar"><button id="pp-return-live" class="quiet-button" type="button" hidden>Return to live</button><span id="pp-transport-status" class="inspect-status"></span></div></header>
      <div id="pp-timeline-track" class="timeline-track" role="group" aria-label="Recorded run events"></div>
      <div class="timeline-legend"><span>○ Decision</span><span>□ Checkpoint</span><span>○ Progress</span><span>○ Failure</span></div>
    </section>
    <section id="run-story" class="story-bay" aria-labelledby="story-title">
      <header><div><span class="section-kicker">Chronology</span><h2 id="story-title">Run Story <small>Recorded events</small></h2></div><div id="pp-story-actions" class="story-header-actions"><button type="button" class="quiet-button" data-evidence>Evidence</button></div></header>
      <div id="pp-story" class="story-list"><p class="empty">Select a run to browse its recorded events.</p></div>
    </section>
    <aside id="evidence-drawer" class="evidence-drawer" aria-labelledby="evidence-title" hidden>
      <header class="evidence-head"><div><span class="section-kicker">Debug</span><h2 id="evidence-title">Evidence</h2></div><button id="pp-close-evidence" class="icon-button" type="button" aria-label="Close evidence">×</button></header>
      <div class="evidence-grid"><section><h3>Run facts</h3><dl id="pp-meta" class="kv"></dl><div class="inspect-actions"><button id="pp-investigate" class="primary-action" type="button">Investigate with AI</button><span id="pp-investigate-status" class="inspect-status"></span></div></section><section><h3>Artifacts</h3><p id="pp-art-empty" class="inspect-status">No artifacts recorded.</p><div id="pp-art-table" class="inspect-table-wrap" hidden><table><thead><tr><th>Name</th><th>Type</th><th>Size</th><th>SHA-256</th><th></th></tr></thead><tbody id="pp-artifacts"></tbody></table></div><details class="plan-raw"><summary>Raw debug bundle</summary><pre id="pp-debug"></pre></details></section></div>
    </aside>`;

  const byID = (id) => document.getElementById(id);
  const mediaHost = byID("detail-game-media");
  const lcd = byID("detail-lcd");
  if (!mediaHost || !lcd) return;
  mediaHost.insertAdjacentHTML("beforeend", `
    <video id="pp-game-video" controls preload="metadata" aria-label="Finished run replay" hidden></video>
    <div id="pp-replay-panel" class="game-replay-panel" hidden>
      <span id="pp-replay-status" class="inspect-status" role="status" aria-live="polite"></span>
      <button id="pp-replay" class="quiet-button" type="button">Generate replay</button>
    </div>`);
  const track = byID("pp-timeline-track");
  const story = byID("pp-story");
  const replayButton = byID("pp-replay");
  const replayStatus = byID("pp-replay-status");
  const replayPanel = byID("pp-replay-panel");
  const transportStatus = byID("pp-transport-status");
  const returnLive = byID("pp-return-live");
  const video = byID("pp-game-video");
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
  let runFinished = false;
  let runTotalFrame = 0;
  let lifecycleStatus = "";
  let runLoadInFlight = false;
  let finishedReloadPending = false;
  let runLoadSerial = 0;
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
  function timelineFrameTotal() {
    return Math.max(1,runTotalFrame,...events.map(eventFrame));
  }
  function markerSymbol(kind) { return kind === "checkpoint" ? "□" : kind === "failure" ? "!" : kind === "progress" ? "✓" : "○"; }
  function renderTimeline() {
    if (!events.length) { track.innerHTML = '<p class="empty">No semantic events were persisted for this run.</p>'; return; }
    const max = timelineFrameTotal();
    track.innerHTML = events.map((event, index) => {
      const kind = eventKind(event), at = eventFrame(event) ? Math.max(2, Math.min(98, 100 * eventFrame(event) / max)) : Math.max(2, 100 * index / Math.max(1, events.length - 1));
      return `<button type="button" class="timeline-marker ${kind}" style="left:${at}%" data-event-index="${index}" aria-label="${html(eventTitle(event))}" aria-current="${index===selectedEvent}">${markerSymbol(kind)}</button>`;
    }).join("");
  }
  function renderStory() {
    if (!events.length) { story.innerHTML = '<p class="empty">No recorded events. Raw run evidence may still be available.</p>'; return; }
    story.innerHTML = events.map((event,index) => {
      const kind=eventKind(event), when=eventFrame(event)?`frame ${eventFrame(event).toLocaleString()}`:(eventRound(event)?`round ${eventRound(event)}`:"recorded");
      return `<div class="story-entry" tabindex="0" role="button" data-event-index="${index}" aria-current="${index===selectedEvent}"><span class="story-when">${html(when)}</span><span class="story-symbol">${markerSymbol(kind)}</span><span class="story-copy"><strong>${html(eventTitle(event))}</strong><span>${html(eventDetail(event))}</span></span>${index===selectedEvent?`<div class="story-detail">${html(eventDetail(event))}</div>`:""}</div>`;
    }).join("");
  }
  function renderStoryActions() {
    const host=byID("pp-story-actions"), event=events[selectedEvent];
    if(!host)return;
    const restart=event&&event.checkpoint?`<button type="button" class="quiet-button" data-repro="${html(event.checkpoint)}" ${event.replayable?"":"disabled"}>Start a new run from here</button>`:"";
    const failure=event&&eventKind(event)==="failure"?'<button type="button" class="primary-action" data-investigate-event>Investigate with AI</button>':"";
    host.innerHTML=`${failure}${restart}<button type="button" class="quiet-button" data-evidence>Evidence</button>`;
  }
  function publishSemanticEvent(index, nearest) {
    const event = events[index];
    if (!event) return;
    window.dispatchEvent(new CustomEvent("pokefarm-semantic-event", {detail:{
      runId:runID,
      event,
      nearest,
      label:nearest ? "Semantic state from nearest persisted event" : "Persisted semantic event"
    }}));
  }
  function selectEvent(index, seekVideo=true, nearest=false) {
    if (!Number.isInteger(index) || index < 0 || index >= events.length) return;
    selectedEvent = index; followingLive = false; returnLive.hidden = runFinished;
    const maxFrame = timelineFrameTotal();
    if (seekVideo && !video.hidden && video.duration > 0 && maxFrame > 0) video.currentTime = video.duration * eventFrame(events[index]) / maxFrame;
    publishSemanticEvent(index, nearest);
    transportStatus.textContent = nearest ? "Semantic state from nearest persisted event" : (runFinished ? "Replay event" : "Browsing recorded event");
    renderTimeline(); renderStory(); renderStoryActions();
    story.querySelector(`[data-event-index="${index}"]`)?.scrollIntoView({block:"nearest",behavior:"smooth"});
  }
  function nearestEventIndex(time) {
    if (!events.length || !(video.duration > 0)) return -1;
    const maxFrame = timelineFrameTotal();
    const frame = maxFrame * time / video.duration;
    let nearest = 0;
    for (let i=1;i<events.length;i++) {
      if (Math.abs(eventFrame(events[i])-frame) < Math.abs(eventFrame(events[nearest])-frame)) nearest=i;
    }
    return nearest;
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
  function clearReplayVideo() {
    video.pause();
    video.removeAttribute("src");
    video.dataset.run="";
    video.hidden=true;
    video.load();
  }
  function resetGameMedia() {
    stopReplayPoll();
    clearReplayVideo();
    replayPanel.hidden=true;
    replayStatus.textContent="";
    replayButton.hidden=false;
    replayButton.disabled=false;
    replayButton.textContent="Generate replay";
    lcd.hidden=false;
  }
  function renderReplay(status) {
    const state=(status&&status.state)||"missing";
    stopReplayPoll();
    if(state!=="ready") clearReplayVideo();
    lcd.hidden=false;
    replayPanel.hidden=false;
    replayButton.hidden=false;
    replayButton.disabled=false;
    if(state==="ready"){
      replayPanel.hidden=true;
      replayStatus.textContent=status.size?`Ready · ${fmtSize(status.size)}`:"Ready";
      replayButton.hidden=true;
      const src=`/v1/runs/${escURL(runID)}/replay/video`;
      if(video.dataset.run!==runID){video.src=src;video.dataset.run=runID;video.load()}
      lcd.hidden=true;
      video.hidden=false;
      transportStatus.textContent="Replay · native video controls";
      return;
    }
    if(state==="generating"){
      replayStatus.textContent=status.progress||"Generating deterministic replay…";
      replayButton.disabled=true;
      replayPoll=setTimeout(()=>loadReplayStatus(runID),1500);
      return;
    }
    if(state==="disabled"){
      replayStatus.textContent=status.error||"Replay generation is unavailable for this console.";
      replayButton.textContent="Generate replay";
      replayButton.disabled=true;
      return;
    }
    if(state==="error"){
      replayStatus.textContent=status.error||"Replay generation failed";
      replayButton.textContent="Retry replay";
      return;
    }
    replayStatus.textContent=status.error||"Ready to generate from the recorded run.";
    replayButton.textContent="Generate replay";
  }
  async function loadReplayStatus(id) { try { const status=await json(`/v1/runs/${escURL(id)}/replay/status`); if(id===runID)renderReplay(status); } catch(err){if(id===runID)renderReplay({state:"error",error:err.message})} }
  async function selectRun(id, reload=false) {
    if(id===runID&&runLoadInFlight){if(reload)finishedReloadPending=true;return}
    const changed=id!==runID;
    const serial=++runLoadSerial;
    runLoadInFlight=true;
    resetGameMedia(); runID=id; debug=null; reproSource=null; checkpoints=[]; events=[]; selectedEvent=-1; followingLive=true; runFinished=false; returnLive.hidden=true; evidence.hidden=true; transportStatus.textContent="";investigate.disabled=false;investigateStatus.textContent="";
    if(changed){lifecycleStatus="";runTotalFrame=0}
    window.dispatchEvent(new CustomEvent("pokefarm-semantic-event",{detail:{runId:id,event:null}}));
    if(!id){track.innerHTML='<p class="empty">Select a run to browse recorded events.</p>';story.innerHTML='<p class="empty">Select a run to browse its recorded events.</p>';replayButton.hidden=true;runLoadInFlight=false;return}
    track.innerHTML='<p class="empty">Loading recorded events…</p>'; story.innerHTML='<p class="empty">Loading Run Story…</p>'; replayButton.hidden=false;replayButton.disabled=true;replayStatus.textContent="Loading…";
    try{
      const [debugView,artifactView,checkpointView,sourceView]=await Promise.all([json(`/v1/runs/${escURL(id)}/debug`),json(`/v1/runs/${escURL(id)}/artifacts`),json(`/v1/runs/${escURL(id)}/checkpoints`).catch(()=>({checkpoints:[]})),json(`/v1/runs/${escURL(id)}/repro-source`).catch(()=>null)]);
      if(id!==runID||serial!==runLoadSerial)return; debug=debugView;reproSource=sourceView;checkpoints=checkpointView.checkpoints||[];events=normalizeEvents(debugView,checkpointView);selectedEvent=events.length-1;const debugStatus=String(debugView.run&&debugView.run.status||"");runFinished=debugStatus==="done";if(!lifecycleStatus)lifecycleStatus=debugStatus;runTotalFrame=Math.max(runTotalFrame,Number(debugView.run&&debugView.run.frame||0));renderTimeline();renderStory();renderStoryActions();renderMeta();renderArtifacts(artifactView);
      const replayable=(artifactView.artifacts||[]).some((a)=>a.replayable);
      if(!runFinished){resetGameMedia();transportStatus.textContent=replayable?"Following live":"Following live · Available after this run finishes and uploads its recording.";return}
      if(replayable)await loadReplayStatus(id);
      else{renderReplay({state:"missing",error:"This run has no run.gbrun recording to replay."});replayButton.disabled=true}
    }catch(err){if(id===runID){track.innerHTML=`<p class="empty">Timeline unavailable: ${html(err.message)}</p>`;story.innerHTML=`<p class="empty">Run Story unavailable: ${html(err.message)}</p>`;replayButton.hidden=false;replayButton.disabled=true;replayStatus.textContent=err.message}}
    finally{if(serial===runLoadSerial){runLoadInFlight=false;if(finishedReloadPending&&id===runID){finishedReloadPending=false;selectRun(id,true)}}}
  }
  async function startFromCheckpoint(name, button) {
    // The wall marks a checkpoint replayable only when paired agent knowledge exists.
    if(!runID||!name)return; button.disabled=true;button.textContent="Starting…";
    try{const queued=await json(`/v1/runs/${escURL(runID)}/repro`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({checkpoint:name})});button.textContent=`Queued ${queued.run_id}`;window.dispatchEvent(new CustomEvent("pokefarm-select-run",{detail:{runId:queued.run_id}}));}catch(err){button.textContent=err.message;button.disabled=false}
  }
  async function investigateRun(button) {
    const targetRunID=runID;
    if(!targetRunID)return;button.disabled=true;investigateStatus.textContent="Finding this run's failure group…";
    try{const groups=await json("/v1/triage");if(targetRunID!==runID)return;const group=(Array.isArray(groups)?groups:[]).find((g)=>(g.run_ids||[]).includes(targetRunID));if(!group)throw new Error("No actionable failure group is linked to this run.");await json(`/v1/triage/${escURL(group.key)}/investigate`,{method:"POST"});if(targetRunID!==runID)return;investigateStatus.textContent="Investigation queued with this run and its evidence.";}catch(err){if(targetRunID!==runID)return;investigateStatus.textContent=err.message;button.disabled=false}
  }

  root.addEventListener("click",(event)=>{const marker=event.target.closest("[data-event-index]");if(marker&&!event.target.closest("[data-repro],[data-evidence],[data-investigate-event]")){selectEvent(Number(marker.dataset.eventIndex));return}const repro=event.target.closest("[data-repro]");if(repro){startFromCheckpoint(repro.dataset.repro,repro);return}if(event.target.closest("[data-evidence]")){evidence.hidden=false;return}if(event.target.closest("[data-investigate-event]")){evidence.hidden=false;investigateRun(investigate);}});
  root.addEventListener("keydown",(event)=>{if((event.key==="Enter"||event.key===" ")&&event.target.matches(".story-entry")){event.preventDefault();selectEvent(Number(event.target.dataset.eventIndex));}});
  replayButton.addEventListener("click",async()=>{
    const id=runID;
    replayButton.disabled=true;
    replayStatus.textContent="Starting replay generation…";
    try{const status=await json(`/v1/runs/${escURL(id)}/replay/render`,{method:"POST"});if(id===runID)renderReplay(status)}
    catch(err){if(id===runID){renderReplay({state:"error",error:err.message})}}
  });
  video.addEventListener("seeked",()=>{
    if(!runFinished||video.dataset.run!==runID)return;
    const index=nearestEventIndex(video.currentTime);
    if(index>=0)selectEvent(index,false,true);
  });
  returnLive.addEventListener("click",()=>{
    if(runFinished)return;
    followingLive=true;
    returnLive.hidden=true;
    transportStatus.textContent="Following live";
    window.dispatchEvent(new CustomEvent("pokefarm-semantic-event",{detail:{runId:runID,event:null}}));
    if(events.length){selectedEvent=events.length-1;renderTimeline();renderStory();renderStoryActions()}
  });
  investigate.addEventListener("click",()=>investigateRun(investigate));
  byID("pp-close-evidence").addEventListener("click",()=>{evidence.hidden=true});
  window.addEventListener("pokefarm-select-run",(event)=>{const id=(event.detail&&event.detail.runId)||"";if(id!==runID)selectRun(id)});
  window.addEventListener("pokefarm-run-lifecycle",(event)=>{const detail=event.detail||{};if(detail.runId!==runID)return;runTotalFrame=Math.max(runTotalFrame,Number(detail.frame||0));const status=String(detail.status||"");const previousStatus=lifecycleStatus;lifecycleStatus=status;if(previousStatus&&previousStatus!=="done"&&status==="done")selectRun(runID,true)});
  window.addEventListener("beforeunload",()=>{stopReplayPoll();clearReplayVideo()},{once:true});
  if (document.documentElement.dataset.selectedRun) selectRun(document.documentElement.dataset.selectedRun);
})();
