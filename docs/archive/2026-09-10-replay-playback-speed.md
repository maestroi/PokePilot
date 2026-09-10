# Replay Playback Speed Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Let operators and spectators play generated run replays at 1×, 2×, 4×, 8×, or 16×, remembering the last chosen rate.

**Architecture:** Keep playback client-side on the existing MP4. Canonical rate helpers live in `behavior.js` for the operator console. Spectator `watch.js` uses the same presets and `localStorage` key but stays a separate public bundle (it does not load `behavior.js`). Native video seeking stays; the only added control is a Speed `<select>`.

**Tech Stack:** Vanilla JS, HTML5 `<video>`, existing pokeui Go embed tests and `ui/behavior_test.js`.

---

### Task 1: Shared playback-rate helpers

**Files:**
- Modify: `cmd/pokeui/ui/behavior.js`
- Modify: `cmd/pokeui/ui/behavior_test.js`
- Test: `go test ./cmd/pokeui -run TestConsoleBehaviorJavaScript`

**Step 1: Write failing helper tests**

In `cmd/pokeui/ui/behavior_test.js`, destructure the new helpers and add:

```javascript
test("playback rate helpers normalize, persist, and ignore storage failures", () => {
  assert.equal(normalizePlaybackRate(8), 8);
  assert.equal(normalizePlaybackRate("16"), 16);
  assert.equal(normalizePlaybackRate(3), 1);
  assert.equal(normalizePlaybackRate("nope"), 1);

  const store = new Map();
  const storage = {
    getItem: (key) => (store.has(key) ? store.get(key) : null),
    setItem: (key, value) => { store.set(key, String(value)); },
  };
  assert.equal(readStoredPlaybackRate(storage), 1);
  assert.equal(writeStoredPlaybackRate(storage, 4), 4);
  assert.equal(storage.getItem("pokepilot.replayPlaybackRate"), "4");
  assert.equal(readStoredPlaybackRate(storage), 4);
  assert.equal(writeStoredPlaybackRate(storage, 99), 1);

  const exploding = {
    getItem() { throw new Error("blocked"); },
    setItem() { throw new Error("blocked"); },
  };
  assert.equal(readStoredPlaybackRate(exploding), 1);
  assert.equal(writeStoredPlaybackRate(exploding, 8), 8);
});
```

Also add `normalizePlaybackRate`, `readStoredPlaybackRate`, and
`writeStoredPlaybackRate` to the `require("./behavior.js")` destructure at
the top of the file.

**Step 2: Run tests and verify failure**

Run:

```bash
go test ./cmd/pokeui -run TestConsoleBehaviorJavaScript
```

Expected: FAIL because the helpers are not exported.

**Step 3: Implement the helpers**

In `cmd/pokeui/ui/behavior.js`, add:

```javascript
  const playbackRates = [1, 2, 4, 8, 16];
  const playbackRateKey = "pokepilot.replayPlaybackRate";

  function normalizePlaybackRate(value) {
    const n = Number(value);
    return playbackRates.includes(n) ? n : 1;
  }

  function readStoredPlaybackRate(storage) {
    try {
      return normalizePlaybackRate(storage && storage.getItem(playbackRateKey));
    } catch (_) {
      return 1;
    }
  }

  function writeStoredPlaybackRate(storage, value) {
    const rate = normalizePlaybackRate(value);
    try {
      if (storage) storage.setItem(playbackRateKey, String(rate));
    } catch (_) {}
    return rate;
  }
```

Export them on the returned API object next to `replayPresentation`.

**Step 4: Run tests and verify they pass**

Run:

```bash
go test ./cmd/pokeui -run TestConsoleBehaviorJavaScript
```

Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/pokeui/ui/behavior.js cmd/pokeui/ui/behavior_test.js
git commit -m "$(cat <<'EOF'
pokeui: remember valid replay playback rates

Shared helpers keep 1×–16× presets and localStorage failures out of the
two player surfaces.
EOF
)"
```

---

### Task 2: Spectator 8× / 16× and remembered rate

**Files:**
- Modify: `cmd/pokeui/ui/watch.html`
- Modify: `cmd/pokeui/ui/watch.js`
- Modify: `cmd/pokeui/spectator_replay_test.go`
- Test: `go test ./cmd/pokeui -run 'TestSpectator(ReplayUsesCachedVideo|PublicReplayRoutesStayAllowlisted)'`

**Step 1: Pin spectator markup and persistence**

In `cmd/pokeui/spectator_replay_test.go`, extend the `watch.js` / `watch.html`
assertions (add a `watchHTML` check if the test currently only reads `watchJS`):

```go
html := string(watchHTML)
for _, want := range []string{
    `id="playback-rate"`,
    `value="1"`, `value="2"`, `value="4"`, `value="8"`, `value="16"`,
} {
    if !strings.Contains(html, want) {
        t.Errorf("watch.html missing playback speed %q", want)
    }
}
for _, want := range []string{
    "playback-rate",
    "pokepilot.replayPlaybackRate",
    "playbackRate",
} {
    if !strings.Contains(src, want) {
        t.Errorf("watch.js missing spectator replay behavior %q", want)
    }
}
```

Keep the existing forbidden-route checks.

**Step 2: Run tests and verify failure**

Run:

```bash
go test ./cmd/pokeui -run 'TestSpectatorPublicReplayRoutesStayAllowlisted'
```

Expected: FAIL because 8× / 16× and the storage key are missing.

**Step 3: Extend spectator UI**

In `cmd/pokeui/ui/watch.html`, replace the Speed select options with:

```html
<option value="1">1×</option><option value="2">2×</option><option value="4">4×</option><option value="8">8×</option><option value="16">16×</option>
```

In `cmd/pokeui/ui/watch.js`, add compact copies of the same helpers (spectator
does not load `behavior.js`). Use the same key `pokepilot.replayPlaybackRate`.

On startup, set `$("playback-rate").value` from storage.

In `showReplay`, after assigning `src` / `load()`, set:

```javascript
video.playbackRate = Number($("playback-rate").value || 1);
```

Also set `playbackRate` on `canplay` so a new source does not reset to 1×.

On `change` of `#playback-rate`:

```javascript
const rate = writeStoredPlaybackRate(window.localStorage, $("playback-rate").value);
$("replay").playbackRate = rate;
```

Wrap storage access the same way as the shared helpers.

**Step 4: Run tests and verify they pass**

Run:

```bash
go test ./cmd/pokeui -run 'TestSpectator'
```

Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/pokeui/ui/watch.html cmd/pokeui/ui/watch.js cmd/pokeui/spectator_replay_test.go
git commit -m "$(cat <<'EOF'
pokeui: add 8× and 16× spectator replay speed

Long public replays can skip ahead without re-encoding, and the chosen
rate is kept for the next recording.
EOF
)"
```

---

### Task 3: Operator Game bay Speed control

**Files:**
- Modify: `cmd/pokeui/ui/inspector.js`
- Modify: `cmd/pokeui/ui/console.css`
- Modify: `cmd/pokeui/ui/behavior_test.js`
- Modify: `cmd/pokeui/console_test.go`
- Test: `go test ./cmd/pokeui -run 'TestUI(ReplayLivesInGameBay|ReplayTimelineIsSeekable)|TestConsoleBehaviorJavaScript'`

**Step 1: Pin console markup and wiring tests**

In `cmd/pokeui/console_test.go`, extend `TestUIReplayLivesInGameBay` (or add
`TestUIReplayHasPlaybackSpeed`) to require:

```go
for _, want := range []string{
    `id="pp-playback-rate"`,
    `id="pp-replay-tools"`,
    `value="8"`,
    `value="16"`,
    `pokepilot.replayPlaybackRate`,
    `playbackRate`,
} {
    if !strings.Contains(inspector, want) {
        t.Errorf("inspector replay speed missing %q", want)
    }
}
if strings.Contains(inspector, `type="range"`) {
    t.Error("finished replay must use native video controls without a duplicate range input")
}
```

In `cmd/pokeui/ui/behavior_test.js`:

- Add `"pp-replay-tools"` and `"pp-playback-rate"` to `createInspectorHarness` ids.
- Give the fake video a `playbackRate` field (default `1`).
- Put `localStorage` on the inspector `context` as an in-memory store.
- Initialize `pp-playback-rate.value` to `"1"`.
- Add:

```javascript
test("finished replay applies stored playback rate and remembers a new one", async () => {
  const harness = createInspectorHarness();
  harness.storage.setItem("pokepilot.replayPlaybackRate", "8");
  harness.select("finished");
  await flushAsync();
  await flushAsync();
  harness.elements["pp-replay"].emit("click");
  await flushAsync();
  harness.timers.shift()();
  await flushAsync();
  await flushAsync();

  const video = harness.elements["pp-game-video"];
  video.emit("canplay");
  assert.equal(video.hidden, false);
  assert.equal(harness.elements["pp-replay-tools"].hidden, false);
  assert.equal(video.playbackRate, 8);
  assert.equal(harness.elements["pp-playback-rate"].value, "8");

  harness.elements["pp-playback-rate"].value = "16";
  harness.elements["pp-playback-rate"].emit("change");
  assert.equal(video.playbackRate, 16);
  assert.equal(harness.storage.getItem("pokepilot.replayPlaybackRate"), "16");

  harness.select("active");
  await flushAsync();
  await flushAsync();
  assert.equal(harness.elements["pp-replay-tools"].hidden, true);
});
```

Return `storage` from the harness.

**Step 2: Run tests and verify failure**

Run:

```bash
go test ./cmd/pokeui -run 'TestUIReplayLivesInGameBay|TestConsoleBehaviorJavaScript'
```

Expected: FAIL because the Game bay has no Speed control.

**Step 3: Mount the control and apply the rate**

In `inspector.js`, destructure the new helpers from `window.PokeConsoleBehavior`.

Extend the Game bay insert to:

```javascript
  mediaHost.insertAdjacentHTML("beforeend", `
    <video id="pp-game-video" controls preload="metadata" aria-label="Finished run replay" hidden></video>
    <div id="pp-replay-tools" class="replay-tools" hidden>
      <label>Speed <select id="pp-playback-rate">
        <option value="1">1×</option>
        <option value="2">2×</option>
        <option value="4">4×</option>
        <option value="8">8×</option>
        <option value="16">16×</option>
      </select></label>
    </div>
    <div id="pp-replay-panel" class="game-replay-panel" hidden>
      <span id="pp-replay-status" class="inspect-status" role="status" aria-live="polite"></span>
      <button id="pp-replay" class="quiet-button" type="button">Generate replay</button>
    </div>`);
```

Wire `byID("pp-replay-tools")` and `byID("pp-playback-rate")`.

On load, set the select from `readStoredPlaybackRate(window.localStorage)`.

Add `applyPlaybackRate()` that sets `video.playbackRate` from the select.
Call it from `canplay`. On select `change`, `writeStoredPlaybackRate` then
`applyPlaybackRate`.

In `applyReplayPresentation`, show `#pp-replay-tools` only when
`state === "playing"`. Hide it in `clearReplayVideo` / `resetGameMedia`.

In `cmd/pokeui/ui/console.css`, add Game-bay overlay rules (square corners,
matching the console, not the spectator pill):

```css
.game-media>.replay-tools{position:absolute;z-index:3;top:8px;right:8px;display:flex;align-items:center;gap:6px;padding:5px 8px;border:1px solid var(--line-strong);background:rgba(15,20,28,.96);color:var(--muted);font-size:11px}
.game-media>.replay-tools select{color:var(--text);background:#12161d;border:0;padding:2px 4px}
```

**Step 4: Run tests and verify they pass**

Run:

```bash
go test ./cmd/pokeui -run 'TestUI(Replay|FramePump)|TestConsoleBehaviorJavaScript|TestSpectator'
```

Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/pokeui/ui/inspector.js cmd/pokeui/ui/console.css cmd/pokeui/ui/behavior_test.js cmd/pokeui/console_test.go
git commit -m "$(cat <<'EOF'
pokeui: add Game bay replay speed control

Finished operator recordings get the same 1×–16× Speed select as the
spectator player, including the remembered rate.
EOF
)"
```

---

## Verification

- `go test ./cmd/pokeui -run 'TestUI(Replay|FramePump)|TestConsoleBehaviorJavaScript|TestSpectator'`
- Manual: generate or open a finished replay on both the operator console and
  public watch page; change speed; reload and confirm the last rate returns;
  switch to a live run and confirm the control disappears.
