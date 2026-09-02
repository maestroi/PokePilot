package emu

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/thelolagemann/gomeboy/pkg/gomeboy"
)

// Watch serves the emulator's screen over HTTP at addr so a human can see
// what the agent is doing. It returns the address actually listened on,
// which is useful when addr uses port 0.
//
// Frames are captured on the goroutine that steps the emulator, once every
// everyFrames frames, so nothing races with emulation and the caller pays a
// predictable encoding cost. everyFrames <= 0 disables capture.
//
// This is a debugging and demonstration surface only. Nothing in PokePilot
// may read gameplay truth from it — see docs/DESIGN.md.
func (m *Emu) Watch(addr string, everyFrames int) (string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	m.spec = gomeboy.NewSpectator()
	m.specEvery = everyFrames
	m.trace = newTraceBuf()

	specHandler := m.spec.Handler()
	mux := http.NewServeMux()
	mux.HandleFunc("/frame.png", specHandler.ServeHTTP)
	mux.HandleFunc("/trace.json", m.trace.serveHTTP)
	mux.HandleFunc("/", serveWatchPage)
	go http.Serve(ln, mux) //nolint:errcheck // serves until the process exits
	return ln.Addr().String(), nil
}

// serveWatchPage serves PokePilot's own spectator page: the screen on the
// left at the same 4x scale as gomeboy's own page, and a scrolling trace
// panel on the right polling /trace.json.
func serveWatchPage(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, watchPage)
}

const watchPage = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>PokePilot</title>
<style>
  html,body { margin:0; height:100%; background:#111; color:#ddd;
              font:14px system-ui,sans-serif; }
  body { display:flex; overflow:hidden; }
  /* The screen column shrinks; the image scales to whatever fits and keeps
     the Game Boy's 160x144 ratio, so a short window scales it down rather
     than cropping it. min-width/min-height:0 is what lets a flex child
     actually shrink below its content size. */
  #screen { flex:1 1 auto; min-width:0; min-height:0; padding:8px;
            display:flex; align-items:center; justify-content:center; }
  #f { display:none; width:640px; height:auto; aspect-ratio:160/144;
       max-width:100%; max-height:100%; image-rendering:pixelated; }
  #w { color:#888; }
  #side { flex:0 0 clamp(280px, 32%, 460px); min-height:0; height:100vh;
          display:flex; flex-direction:column; border-left:1px solid #333; }
  #head { flex:0 0 auto; padding:8px 12px; border-bottom:1px solid #333;
          color:#6cf; background:#181818; white-space:pre-wrap; }
  /* The statistics panel: pinned like the header, capped at a third of the
     column so a long choice list never squeezes the trace out of view. */
  #stats { flex:0 0 auto; display:none; max-height:34vh; overflow-y:auto;
           box-sizing:border-box; padding:8px 12px; background:#181818;
           border-bottom:1px solid #333; }
  #nums { display:grid; grid-template-columns:1fr 1fr; gap:2px 12px; }
  #nums div { display:flex; justify-content:space-between; gap:8px; }
  #nums span:first-child { color:#888; }
  .warn { color:#f96; }
  #goal { margin-top:6px; color:#fc9; white-space:pre-wrap; }
  #intent { margin-top:6px; color:#9c9; white-space:pre-wrap; }
  #choices { margin-top:6px; }
  #choices div { position:relative; padding:2px 4px; margin-top:1px;
                 display:flex; justify-content:space-between; gap:8px; }
  /* The bar is the row's own background, so a choice picked six times reads
     at a glance without a chart library. */
  .bar { position:absolute; left:0; top:0; bottom:0; background:#2a3a4a;
         z-index:0; border-radius:2px; }
  #choices span { position:relative; z-index:1; }
  .n { color:#6cf; }
  #trace { flex:1 1 auto; min-height:0; overflow-y:auto; box-sizing:border-box;
           padding:8px 12px; }
  #trace div { padding:2px 0; border-bottom:1px solid #222; white-space:pre-wrap; }
  .frame { color:#888; margin-right:8px; }
  .kind { color:#6cf; margin-right:8px; }
</style>
</head>
<body>
<div id="screen">
  <p id="w">waiting for the first frame...</p>
  <img id="f" alt="">
</div>
<div id="side">
  <div id="head"></div>
  <div id="stats">
    <div id="nums"></div>
    <div id="goal"></div>
    <div id="intent"></div>
    <div id="choices"></div>
  </div>
  <div id="trace"></div>
</div>
<script>
const img = document.getElementById('f'), wait = document.getElementById('w');
const trace = document.getElementById('trace');
const head = document.getElementById('head');
const stats = document.getElementById('stats'), nums = document.getElementById('nums');
const goal = document.getElementById('goal'), intent = document.getElementById('intent');
const choices = document.getElementById('choices');
const esc = s => String(s).replace(/[&<>]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;'}[c]));
let inFlight = false;
async function tickFrame() {
  if (inFlight) return;
  inFlight = true;
  try {
    const r = await fetch('/frame.png', { cache: 'no-store' });
    if (r.ok) {
      const url = URL.createObjectURL(await r.blob());
      const old = img.src;
      img.src = url;
      if (old.startsWith('blob:')) URL.revokeObjectURL(old);
      img.style.display = 'block';
      wait.style.display = 'none';
    }
  } catch (e) { /* server gone; keep showing the last frame */ }
  finally { inFlight = false; }
}

// Page on seq, not on array length: the buffer is a ring, so once it wraps
// the length stops growing while entries keep arriving. Reset on a new run,
// because seq restarts and the old lines belong to a dead process.
let lastSeq = 0, run = null;
async function tickTrace() {
  try {
    const r = await fetch('/trace.json', { cache: 'no-store' });
    if (!r.ok) return;
    const payload = await r.json();
    head.textContent = payload.header || '';
    renderStats(payload.stats);
    if (payload.run !== run) {
      run = payload.run;
      lastSeq = 0;
      trace.replaceChildren();
    }
    for (const e of payload.entries) {
      if (e.seq <= lastSeq) continue;
      lastSeq = e.seq;
      const line = document.createElement('div');
      line.innerHTML = '<span class="frame">#' + e.frame + '</span>' +
                        '<span class="kind">' + e.kind + '</span>' +
                        e.text.replace(/[&<>]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;'}[c]));
      trace.appendChild(line);
    }
    trace.scrollTop = trace.scrollHeight;
  } catch (e) { /* server gone */ }
}

// renderStats draws the planner tally the run pushes with TraceStats. The
// server decides what a statistic is; this only lays it out, so a new field
// needs a row here and nothing else.
function renderStats(s) {
  if (!s) { stats.style.display = 'none'; return; }
  stats.style.display = 'block';
  const row = (label, value, warn) =>
    '<div><span>' + label + '</span><span' + (warn ? ' class="warn"' : '') +
    '>' + esc(value) + '</span></div>';
  nums.innerHTML =
    row('round', s.round + (s.rounds_left ? ' (' + s.rounds_left + ' left)' : '')) +
    row('repeat picks', s.repeats + ' of ' + s.rounds, s.rounds > 3 && s.repeats * 2 >= s.rounds) +
    row('think', s.last_seconds.toFixed(1) + 's / ' + s.avg_seconds.toFixed(1) + 's avg') +
    row('offered', s.avg_offered.toFixed(1) + ' avg') +
    row('tokens', s.prompt_tokens + ' / ' + s.completion_tokens) +
    row('rejected', s.rejected, s.rejected > 0) +
    row('transport', s.transport, s.transport > 0) +
    row('fallbacks', s.fallbacks, s.fallbacks > 0);
  goal.textContent = s.goal_summary ? 'goal: ' + s.goal_summary + (s.goal_complete ? ' (complete)' : '') : '';
  intent.textContent = s.intent ? '"' + s.intent + '" (' + s.intent_age + ' rounds)' : '';
  const top = (s.choices && s.choices[0]) ? s.choices[0].count : 1;
  choices.innerHTML = (s.choices || []).map(c =>
    '<div><div class="bar" style="width:' + (100 * c.count / top) + '%"></div>' +
    '<span>' + esc(c.objective) + '</span><span class="n">' + c.count + '</span></div>').join('');
}

setInterval(tickFrame, 100);
setInterval(tickTrace, 500);
tickFrame();
tickTrace();
</script>
</body>
</html>
`

// capture refreshes what Watch serves, if watching is on and enough frames
// have passed. Errors are ignored: a dropped preview frame must never affect
// emulation.
func (m *Emu) capture() {
	if m.spec == nil || m.specEvery <= 0 {
		return
	}
	if m.e.FrameCount()-m.lastCapture < uint64(m.specEvery) {
		return
	}
	m.lastCapture = m.e.FrameCount()
	_ = m.spec.Capture(m.e)
	m.sampleTrace()
}

// Pace throttles emulation to about fps frames per second, so a human can
// follow what the agent is doing. Zero or negative runs as fast as the CPU
// allows, which is the default and what tests use.
//
// Pacing is wall-clock only. It cannot change what the game does, because
// every skill waits on RAM predicates rather than on elapsed time.
func (m *Emu) Pace(fps int) {
	if fps <= 0 {
		m.frameDur = 0
		return
	}
	m.frameDur = time.Second / time.Duration(fps)
	m.nextFrame = time.Time{}
}

// throttle sleeps until n frames' worth of wall clock has passed.
func (m *Emu) throttle(n int) {
	if m.frameDur <= 0 {
		return
	}
	if m.nextFrame.IsZero() {
		m.nextFrame = time.Now()
	}
	m.nextFrame = m.nextFrame.Add(time.Duration(n) * m.frameDur)
	d := time.Until(m.nextFrame)
	switch {
	case d > 0:
		time.Sleep(d)
	case d < -time.Second:
		// ponytail: fell more than a second behind (a slow capture, a
		// descheduled process). Resync instead of sprinting to catch up,
		// which would look worse than the hitch we already took.
		m.nextFrame = time.Now()
	}
}
