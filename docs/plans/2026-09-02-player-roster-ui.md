# Player Roster UI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show a live party roster (name, level, HP, status), money, and badges on the local watch page and in pokeui's detail pane.

**Architecture:** `cmd/pokepilot` decodes RAM it already reads into a typed `farm.Player` snapshot. That object rides the existing 1 Hz heartbeat (wall → `/v1/dashboard`) and an opaque sibling blob on `/trace.json`. Both UIs render the same Party block. Cards do not change. No new HTTP routes.

**Tech Stack:** Go stdlib, existing embedded HTML/JS (`emu/watch.go`, `cmd/pokeui/ui`).

**Design doc:** `docs/plans/2026-09-02-player-roster-ui-design.md`

**Commits:** If you are run by agent-runner, leave the tree dirty — do not commit. Otherwise commit after each task with the listed paths only (never `git add -A`).

---

### Task 1: Wire type — `farm.Player` on the heartbeat

**Files:**
- Modify: `farm/spec.go` (new `PartyMon` + `Player`; `Heartbeat.Player`)
- Test: `farm/spec_test.go`

**Step 1: Write the failing test**

Append to `farm/spec_test.go`:

```go
func TestHeartbeatCarriesPlayer(t *testing.T) {
	want := Heartbeat{
		RunID: "r1", Frame: 100,
		Player: &Player{
			Money:  1840,
			Badges: []string{"Boulder"},
			Party: []PartyMon{{
				Name: "squirtle", Level: 8, HP: 12, MaxHP: 35, Status: "poisoned",
			}},
		},
	}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Heartbeat
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got.Player, want.Player) {
		t.Fatalf("player round trip = %+v, want %+v", got.Player, want.Player)
	}
	for _, field := range []string{`"player"`, `"money"`, `"badges"`, `"party"`, `"name"`, `"level"`, `"hp"`, `"max_hp"`, `"status"`} {
		if !contains(string(b), field) {
			t.Errorf("marshaled heartbeat missing %s: %s", field, b)
		}
	}

	// Pre-starter: empty party is a real snapshot, not an omitted key.
	empty := Heartbeat{RunID: "r2", Player: &Player{Money: 3000, Party: []PartyMon{}}}
	b, err = json.Marshal(empty)
	if err != nil {
		t.Fatalf("marshal empty: %v", err)
	}
	if !contains(string(b), `"party":[]`) && !contains(string(b), `"party": []`) {
		t.Errorf("empty party must encode as an array: %s", b)
	}
	if !contains(string(b), `"player"`) {
		t.Errorf("empty party must still send player: %s", b)
	}

	// Older runner: nil player omits the key.
	b, err = json.Marshal(Heartbeat{RunID: "old"})
	if err != nil {
		t.Fatalf("marshal old: %v", err)
	}
	if contains(string(b), `"player"`) {
		t.Errorf("nil player must be omitted: %s", b)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./farm -run TestHeartbeatCarriesPlayer`

Expected: FAIL — `undefined: Player` (compile error).

**Step 3: Write minimal implementation**

In `farm/spec.go`, after `MapSprite` and before `Heartbeat`, add:

```go
// PartyMon is one party member on the operator wire: named, never a ROM
// index. Status is empty when healthy.
type PartyMon struct {
	Name   string `json:"name"`
	Level  uint8  `json:"level"`
	HP     uint16 `json:"hp"`
	MaxHP  uint16 `json:"max_hp"`
	Status string `json:"status,omitempty"`
}

// Player is a live trainer snapshot the runner decodes from RAM. Nil on
// older runners and before the first sample. An empty Party is a real
// pre-starter snapshot and must still be sent.
type Player struct {
	Money  uint32     `json:"money"`
	Badges []string   `json:"badges,omitempty"`
	Party  []PartyMon `json:"party"`
}
```

On `Heartbeat`, after `Stats`:

```go
	// Player is the live party/money/badges snapshot. Nil on older
	// runners and before the first sample.
	Player *Player `json:"player,omitempty"`
```

**Step 4: Run test to verify it passes**

Run: `go test ./farm -run TestHeartbeatCarriesPlayer`

Expected: PASS

**Step 5: Commit** (skip if agent-runner)

```bash
git add farm/spec.go farm/spec_test.go
git commit -m "$(cat <<'EOF'
farm: carry a live player snapshot on the heartbeat

EOF
)"
```

---

### Task 2: emu — opaque `player` blob on `/trace.json`

**Files:**
- Modify: `emu/trace.go` (`traceBuf.player`, `tracePayload.Player`, `TracePlayer`)
- Test: `emu/trace_test.go`

**Step 1: Write the failing test**

Append to `emu/trace_test.go`:

```go
func TestTracePlayerRoundTrip(t *testing.T) {
	b := newTraceBuf()
	rec := httptest.NewRecorder()
	b.serveHTTP(rec, httptest.NewRequest(http.MethodGet, "/trace.json", nil))
	if strings.Contains(rec.Body.String(), "player") {
		t.Fatalf("unset player must be omitted, got %s", rec.Body.String())
	}

	b.player = json.RawMessage(`{"money":1840}`)
	rec = httptest.NewRecorder()
	b.serveHTTP(rec, httptest.NewRequest(http.MethodGet, "/trace.json", nil))
	var got tracePayload
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if string(got.Player) != `{"money":1840}` {
		t.Fatalf("player = %s, want the blob verbatim", got.Player)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./emu -run TestTracePlayerRoundTrip`

Expected: FAIL — `b.player undefined` (compile error).

**Step 3: Write minimal implementation**

In `emu/trace.go`:

- Add `player json.RawMessage` on `traceBuf` next to `stats`.
- Add `Player json.RawMessage \`json:"player,omitempty"\`` on `tracePayload`.
- In `serveHTTP`, copy `player` under the lock next to `stats` and pass it into the encode.
- Add `TracePlayer` mirroring `TraceStats`:

```go
// TracePlayer replaces the player-snapshot blob served alongside the
// trace. v is marshalled here and carried verbatim; a value that will
// not marshal is dropped. Safe to call at any time, from any goroutine.
func (m *Emu) TracePlayer(v any) {
	if m.trace == nil {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	m.trace.mu.Lock()
	m.trace.player = b
	m.trace.mu.Unlock()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./emu -run TestTracePlayerRoundTrip`

Expected: PASS

**Step 5: Commit** (skip if agent-runner)

```bash
git add emu/trace.go emu/trace_test.go
git commit -m "$(cat <<'EOF'
emu: carry an opaque player blob on the watch trace

EOF
)"
```

---

### Task 3: Runner — decode RAM into `farm.Player`

**Files:**
- Modify: `cmd/pokepilot/farm.go` (`playerSnapshot`, `sampleHeartbeat`)
- Modify: `cmd/pokepilot/main.go` (local `OnSample` also pushes `TracePlayer`)
- Test: `cmd/pokepilot/farm_test.go`

**Step 1: Write the failing tests**

Append to `cmd/pokepilot/farm_test.go`:

```go
func TestPlayerSnapshotNamesParty(t *testing.T) {
	g := state.GameState{
		Inventory: state.InventoryState{Money: 1840},
		Progress:  state.ProgressState{Badges: 0x01},
		Party: state.PartyState{
			Count: 1,
			Mons: []state.Mon{{
				Species: 0xB1, Level: 8, HP: 12, MaxHP: 35, Status: 1 << 3,
			}},
		},
	}
	got := playerSnapshot(g)
	if got == nil {
		t.Fatal("playerSnapshot returned nil")
	}
	if got.Money != 1840 {
		t.Fatalf("money = %d, want 1840", got.Money)
	}
	if len(got.Badges) != 1 || got.Badges[0] != "Boulder" {
		t.Fatalf("badges = %v, want [Boulder]", got.Badges)
	}
	if len(got.Party) != 1 {
		t.Fatalf("party len = %d, want 1", len(got.Party))
	}
	m := got.Party[0]
	if m.Name != "squirtle" || m.Level != 8 || m.HP != 12 || m.MaxHP != 35 || m.Status != "poisoned" {
		t.Fatalf("party[0] = %+v, want squirtle Lv8 12/35 poisoned", m)
	}

	unknown := playerSnapshot(state.GameState{
		Party: state.PartyState{Count: 1, Mons: []state.Mon{{Species: 0xFE, Level: 5, HP: 1, MaxHP: 1}}},
	})
	if unknown == nil || len(unknown.Party) != 1 || unknown.Party[0].Name != "species 0xfe" {
		t.Fatalf("unknown species = %+v, want name species 0xfe", unknown)
	}

	empty := playerSnapshot(state.GameState{Inventory: state.InventoryState{Money: 3000}})
	if empty == nil || empty.Party == nil || len(empty.Party) != 0 || empty.Money != 3000 {
		t.Fatalf("empty party = %+v, want non-nil player with party []", empty)
	}
}

func TestHeartbeatSnapTakesPlayerKeepsStats(t *testing.T) {
	s := &heartbeatSnap{}
	s.store(farm.Heartbeat{RunID: "r1"})
	s.storeStats(farm.LLMStats{Round: 2, Rounds: 2})

	next := farm.Heartbeat{
		RunID: "r1", Frame: 20,
		Player: &farm.Player{Money: 1840, Party: []farm.PartyMon{{Name: "squirtle", Level: 8, HP: 10, MaxHP: 35}}},
	}
	s.storeStatus(next)
	got := s.load()
	if got.Stats == nil || got.Stats.Round != 2 {
		t.Fatalf("storeStatus blanked stats: %+v", got.Stats)
	}
	if got.Player == nil || got.Player.Money != 1840 || len(got.Player.Party) != 1 || got.Player.Party[0].HP != 10 {
		t.Fatalf("storeStatus dropped player: %+v", got.Player)
	}

	s.store(farm.Heartbeat{RunID: "r2"})
	if got = s.load(); got.Player != nil {
		t.Fatalf("new lease kept the old player: %+v", got.Player)
	}
}
```

Add `"github.com/maestroi/pokepilot/red/state"` to the test file imports if it is not already there.

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/pokepilot -run 'TestPlayerSnapshotNamesParty|TestHeartbeatSnapTakesPlayerKeepsStats'`

Expected: FAIL — `undefined: playerSnapshot`.

**Step 3: Write minimal implementation**

In `cmd/pokepilot/farm.go`, add:

```go
func playerSnapshot(g state.GameState) *farm.Player {
	p := &farm.Player{
		Money: g.Inventory.Money,
		Party: make([]farm.PartyMon, len(g.Party.Mons)),
	}
	for b := state.BadgeBoulder; b <= state.BadgeEarth; b++ {
		if g.Progress.Has(b) {
			p.Badges = append(p.Badges, b.String())
		}
	}
	for i, mon := range g.Party.Mons {
		name, ok := agent.SpeciesName(mon.Species)
		if !ok {
			name = fmt.Sprintf("species 0x%02x", mon.Species)
		}
		p.Party[i] = farm.PartyMon{
			Name:   name,
			Level:  mon.Level,
			HP:     mon.HP,
			MaxHP:  mon.MaxHP,
			Status: mon.StatusName(),
		}
	}
	return p
}
```

`farm.go` already imports `agent`, `farm`, `fmt`, and `state`.

In `sampleHeartbeat`, after `g := state.Read(m, mem)` and when building `hb`, set:

```go
Player: playerSnapshot(g),
```

and after `snap.storeStatus(hb)`:

```go
m.TracePlayer(hb.Player)
```

In `cmd/pokepilot/main.go`, replace the local (non-farm) `OnSample` so it still traces dialogue and also pushes the player blob. Hoist a `state.Mem` the same way farm does:

```go
var watchMem state.Mem
tracer := newDialogueTracer()
m.OnSample(func(m *emu.Emu) {
	tracer.sample(m)
	m.TracePlayer(playerSnapshot(state.Read(m, &watchMem)))
})
```

`main.go` already imports `emu` and `state`.

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/pokepilot -run 'TestPlayerSnapshotNamesParty|TestHeartbeatSnapTakesPlayerKeepsStats|TestHeartbeatSnapKeepsPlan|TestStatsPlannerPushesToSnap'`

Expected: PASS

**Step 5: Commit** (skip if agent-runner)

```bash
git add cmd/pokepilot/farm.go cmd/pokepilot/main.go cmd/pokepilot/farm_test.go
git commit -m "$(cat <<'EOF'
pokepilot: publish a live player snapshot from RAM

EOF
)"
```

---

### Task 4: Wall — store, dashboard, persist, retry

**Files:**
- Modify: `cmd/pokewall/wall.go` (`Tile`, `tileRow`, `persistedTile`, heartbeat, snapshot, marshal, restore, reset, retry)
- Test: `cmd/pokewall/llmstats_test.go` (same file; same lifecycle as stats)

**Step 1: Write the failing tests**

Append to `cmd/pokewall/llmstats_test.go`:

```go
func samplePlayer(hp uint16) *farm.Player {
	return &farm.Player{
		Money:  1840,
		Badges: []string{"Boulder"},
		Party:  []farm.PartyMon{{Name: "squirtle", Level: 8, HP: hp, MaxHP: 35}},
	}
}

func TestDashboardCarriesPlayer(t *testing.T) {
	w := NewWall("")
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	ctx := context.Background()
	client := farm.NewClient(srv.URL)
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "p1", Planner: "llm"})

	spec, err := client.Lease(ctx)
	if err != nil || spec == nil {
		t.Fatalf("lease = %v, %v; want a spec", spec, err)
	}
	if _, err := client.Heartbeat(ctx, farm.Heartbeat{RunID: spec.RunID, Frame: 1}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	dash := getDashboardView(t, srv.URL)
	if len(dash.Runs) != 1 || dash.Runs[0].Player != nil {
		t.Fatalf("pre-sample player = %+v, want nil", dash.Runs)
	}

	if _, err := client.Heartbeat(ctx, farm.Heartbeat{RunID: spec.RunID, Frame: 2, Player: samplePlayer(12)}); err != nil {
		t.Fatalf("heartbeat with player: %v", err)
	}
	dash = getDashboardView(t, srv.URL)
	p := dash.Runs[0].Player
	if p == nil || p.Money != 1840 || len(p.Party) != 1 || p.Party[0].Name != "squirtle" || p.Party[0].HP != 12 {
		t.Fatalf("dashboard player = %+v", p)
	}
}

func TestWallResetsPlayerOnRetryKeptOnDone(t *testing.T) {
	w := NewWall("")
	srv := httptest.NewServer(w.Handler())
	defer srv.Close()
	ctx := context.Background()
	client := farm.NewClient(srv.URL)
	enqueueViaHTTP(t, srv.URL, farm.Spec{RunID: "flaky-party", Planner: "llm"})

	spec, err := client.Lease(ctx)
	if err != nil || spec == nil {
		t.Fatalf("lease 1 = %v, %v", spec, err)
	}
	if _, err := client.Heartbeat(ctx, farm.Heartbeat{RunID: spec.RunID, Frame: 5, Player: samplePlayer(3)}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: spec.RunID, Attempt: spec.Attempt, Reason: "error", Detail: "wild battle"}); err != nil {
		t.Fatalf("finish 1: %v", err)
	}
	w.mu.Lock()
	reset := w.tiles["flaky-party"].Player == nil && w.tiles["flaky-party"].Status == statusQueued
	w.mu.Unlock()
	if !reset {
		t.Fatal("retried run kept attempt 1's player")
	}

	spec, err = client.Lease(ctx)
	if err != nil || spec == nil || spec.Attempt != 2 {
		t.Fatalf("lease 2 = %v, %v; want attempt 2", spec, err)
	}
	if _, err := client.Heartbeat(ctx, farm.Heartbeat{RunID: spec.RunID, Frame: 9, Player: samplePlayer(35)}); err != nil {
		t.Fatalf("heartbeat 2: %v", err)
	}
	if err := client.Finish(ctx, farm.FinishReport{RunID: spec.RunID, Attempt: spec.Attempt, Reason: "done"}); err != nil {
		t.Fatalf("finish 2: %v", err)
	}
	w.mu.Lock()
	t2 := w.tiles["flaky-party"]
	kept := t2.Finished && t2.Status == statusDone && t2.Player != nil && t2.Player.Party[0].HP == 35
	w.mu.Unlock()
	if !kept {
		t.Fatalf("finished run did not keep its final player (player=%+v)", t2.Player)
	}
}

func TestWallStatePersistsPlayer(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "wall-state.json")
	ctx := context.Background()

	w1 := NewWall("")
	w1.SetStatePath(stateFile)
	srv1 := httptest.NewServer(w1.Handler())
	enqueueViaHTTP(t, srv1.URL, farm.Spec{RunID: "persist-party", Planner: "llm"})
	c1 := farm.NewClient(srv1.URL)
	spec, err := c1.Lease(ctx)
	if err != nil || spec == nil {
		t.Fatalf("lease = %v, %v", spec, err)
	}
	if _, err := c1.Heartbeat(ctx, farm.Heartbeat{RunID: spec.RunID, Frame: 3, Player: samplePlayer(20)}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	srv1.Close()

	w2 := NewWall("")
	w2.SetStatePath(stateFile)
	srv2 := httptest.NewServer(w2.Handler())
	defer srv2.Close()

	dash := getDashboardView(t, srv2.URL)
	if len(dash.Runs) != 1 || dash.Runs[0].Player == nil || dash.Runs[0].Player.Party[0].HP != 20 {
		t.Fatalf("restored dashboard dropped the player: %+v", dash.Runs)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/pokewall -run 'TestDashboardCarriesPlayer|TestWallResetsPlayerOnRetryKeptOnDone|TestWallStatePersistsPlayer'`

Expected: FAIL — `tileRow` / dashboard has no `Player` field.

**Step 3: Write minimal implementation**

In `cmd/pokewall/wall.go`, add `Player *farm.Player` next to `Stats` on:

- `Tile` (comment: kept on finish, nilled on retry)
- `tileRow` (`json:"player,omitempty"`)
- `persistedTile` (`json:"player,omitempty"`)

Copy it in every place `Stats` is copied:

- `handleHeartbeat`: `t.Player = hb.Player`
- `snapshot` (`tileRow`): `Player: t.Player`
- `marshalStateLocked`: `Player: t.Player`
- restore from `persistedTile`: `Player: pt.Player`
- re-queue / retry reset (the two sites that already do `t.Stats = nil`): `t.Player = nil`

Do not add it to live-only fields (Raw / Sprites / Trail). Player persists like Stats.

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/pokewall -run 'TestDashboardCarriesPlayer|TestWallResetsPlayerOnRetryKeptOnDone|TestWallStatePersistsPlayer|TestDashboardCarriesLLMStats|TestWallResetsStatsOnRetryKeptOnDone|TestWallStatePersistsLLMStats'`

Expected: PASS

**Step 5: Commit** (skip if agent-runner)

```bash
git add cmd/pokewall/wall.go cmd/pokewall/llmstats_test.go
git commit -m "$(cat <<'EOF'
pokewall: keep the live player snapshot on the dashboard

EOF
)"
```

---

### Task 5: Local watch — `#party` panel

**Files:**
- Modify: `emu/watch.go` (CSS, markup, `renderParty`)
- Test: none in emu for HTML (the blob test is Task 2). Source-level check is optional; prefer a small string assert if one already exists. If not, skip a new test here — pokeui has the source-grep, and `TestTracePlayerRoundTrip` covers the pipe.

**Step 1: Extend the watch page**

In `watchPage` CSS, next to `#stats`:

```css
  #party { flex:0 0 auto; display:none; max-height:28vh; overflow-y:auto;
           box-sizing:border-box; padding:8px 12px; background:#181818;
           border-bottom:1px solid #333; }
  #psum { color:#fc9; margin-bottom:6px; }
  .prow { display:grid; grid-template-columns:1fr auto auto; gap:6px 10px;
          align-items:center; padding:2px 0; }
  .php { color:#ddd; }
  .pstatus { color:#f96; }
  .hp { grid-column:1 / -1; height:4px; background:#2a3a4a; border-radius:2px; }
  .hp i { display:block; height:100%; background:#6c6; border-radius:2px; }
  .hp.mid i { background:#fc9; }
  .hp.low i { background:#f66; }
  .pempty { color:#888; }
```

In the `#side` markup, **above** `#stats`:

```html
  <div id="party"></div>
```

In the script, next to `renderStats`:

```javascript
function renderParty(p) {
  const el = document.getElementById('party');
  if (!p) { el.style.display = 'none'; el.replaceChildren(); return; }
  el.style.display = 'block';
  const badges = (p.badges && p.badges.length) ? p.badges.join(', ') : 'no badges';
  const rows = (p.party || []).map(m => {
    const max = m.max_hp || 0, hp = m.hp || 0;
    const pct = max ? Math.max(0, Math.min(100, 100 * hp / max)) : 0;
    const cls = (!max || hp === 0 || pct < 20) ? 'low' : (pct < 50 ? 'mid' : '');
    const status = m.status ? '<span class="pstatus">' + esc(m.status) + '</span>' : '';
    return '<div class="prow"><span>' + esc(m.name) + '</span><span>Lv.' +
      esc(m.level) + '</span><span class="php">' + hp + '/' + max +
      '</span>' + status + '<div class="hp ' + cls + '"><i style="width:' +
      pct + '%"></i></div></div>';
  }).join('');
  el.innerHTML = '<div id="psum">₽' + esc(p.money) + ' · ' + esc(badges) + '</div>' +
    (rows || '<div class="pempty">no Pokémon yet</div>');
}
```

In `tickTrace`, after `renderStats(payload.stats)`:

```javascript
    renderParty(payload.player);
```

**Step 2: Compile check**

Run: `go test ./emu -count=0`

Expected: PASS (the page is a string constant; a bad quote fails the build).

**Step 3: Commit** (skip if agent-runner)

```bash
git add emu/watch.go
git commit -m "$(cat <<'EOF'
watch: render the live party roster above the LLM tally

EOF
)"
```

---

### Task 6: pokeui — Party block in the detail pane

**Files:**
- Modify: `cmd/pokeui/ui/ui.js` (`partyHTML`, `renderDetail`)
- Modify: `cmd/pokeui/ui/index.html` (CSS only)
- Test: `cmd/pokeui/console_test.go`

**Step 1: Write the failing test**

Append to `cmd/pokeui/console_test.go`:

```go
func TestUIRendersPlayerRoster(t *testing.T) {
	js := string(uiJS)
	for _, want := range []string{
		"partyHTML",
		`r.player`,
		`<h3>Party</h3>`,
		"no Pokémon yet",
		"no badges",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("ui.js missing %q", want)
		}
	}
	// Cards stay a one-line inspector; the roster is detail-only.
	if strings.Contains(js, "partyHTML(r)") || strings.Contains(js, "partyHTML(run)") && !strings.Contains(js, "partyHTML(run)") {
		// renderDetail must call partyHTML(run); renderLive must not.
	}
	live := js[strings.Index(js, "function renderLive"):]
	if i := strings.Index(live, "function render"); i > 0 {
		// keep looking
	}
	if strings.Contains(js[strings.Index(js, "function renderLive"):strings.Index(js, "function renderWorkers")], "partyHTML") {
		t.Error("live cards must not render the party roster")
	}
	html := string(indexHTML)
	for _, want := range []string{`.party-row`, `.party-hp`, `.party-sum`} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html missing %s", want)
		}
	}
}
```

Use this cleaner card assertion (replace the messy middle of the test above):

```go
func TestUIRendersPlayerRoster(t *testing.T) {
	js := string(uiJS)
	for _, want := range []string{
		"partyHTML",
		`r.player`,
		`<h3>Party</h3>`,
		"no Pokémon yet",
		"no badges",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("ui.js missing %q", want)
		}
	}
	start := strings.Index(js, "function renderLive")
	end := strings.Index(js, "function renderWorkers")
	if start < 0 || end <= start {
		t.Fatal("renderLive bounds")
	}
	if strings.Contains(js[start:end], "partyHTML") {
		t.Error("live cards must not render the party roster")
	}
	html := string(indexHTML)
	for _, want := range []string{`.party-row`, `.party-hp`, `.party-sum`} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html missing %s", want)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/pokeui -run TestUIRendersPlayerRoster`

Expected: FAIL — `ui.js missing "partyHTML"`.

**Step 3: Write minimal implementation**

In `cmd/pokeui/ui/ui.js`, add next to `playHTML`:

```javascript
  function partyHTML(run) {
    const p = run.player;
    if (!p) return "";
    const badges = (p.badges && p.badges.length) ? p.badges.join(", ") : "no badges";
    const rows = (p.party || []).map((m) => {
      const max = m.max_hp || 0, hp = m.hp || 0;
      const pct = max ? Math.max(0, Math.min(100, (100 * hp) / max)) : 0;
      const cls = (!max || hp === 0 || pct < 20) ? "low" : (pct < 50 ? "mid" : "");
      const status = m.status ? `<span class="pstatus">${esc(m.status)}</span>` : "";
      return `<div class="party-row"><span>${esc(m.name)}</span><span>Lv.${esc(m.level)}</span><span class="php">${hp}/${max}</span>${status}<div class="party-hp ${cls}"><i style="width:${pct}%"></i></div></div>`;
    }).join("");
    return `<div class="block"><h3>Party</h3><div class="party-sum">₽${esc(p.money)} · ${esc(badges)}</div>${rows || `<p class="pempty">no Pokémon yet</p>`}</div>`;
  }
```

In `renderDetail`, append it with the other blocks:

```javascript
    $("detail-body").innerHTML = `<div class="block"><h3>Settings</h3>${settings}</div><div class="block"><h3>${run.status === "done" ? "Outcome" : "Now"}</h3>${kv(stateRows)}</div>` + planHTML(run) + playHTML(run) + partyHTML(run) + lastEventHTML(run);
```

In `cmd/pokeui/ui/index.html` style block, add next to `.pchoice`:

```css
.party-sum{margin:0 0 6px;color:var(--amber);font-size:15px}
.party-row{display:grid;grid-template-columns:1fr auto auto auto;gap:4px 8px;align-items:center;font-size:15px}
.party-row .php{color:var(--ink)}
.party-row .pstatus{color:var(--red);font-size:12px}
.party-hp{grid-column:1/-1;height:4px;background:#2c3a52;border-radius:2px}
.party-hp i{display:block;height:100%;background:var(--green);border-radius:2px}
.party-hp.mid i{background:var(--amber)}
.party-hp.low i{background:var(--red)}
.pempty{margin:0;color:var(--muted)}
```

Do **not** mention `partyHTML` or `r.player` inside `renderLive`.

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/pokeui -run TestUIRendersPlayerRoster`

Expected: PASS

**Step 5: Commit** (skip if agent-runner)

```bash
git add cmd/pokeui/ui/ui.js cmd/pokeui/ui/index.html cmd/pokeui/console_test.go
git commit -m "$(cat <<'EOF'
pokeui: show the party roster in the run detail pane

EOF
)"
```

---

### Task 7: Verification

**Step 1: Package tests this plan touched**

Run: `go test ./farm ./emu ./cmd/pokepilot ./cmd/pokewall ./cmd/pokeui`

Expected: PASS

Do not adopt a red journey test that is not on this surface.

**Step 2: Manual smoke (when a ROM and wall are handy)**

- Local: `pokepilot` with watch open — after the starter, `#party` lists the mon, money, `no badges`.
- Farm: queue an llm run, open it in pokeui — Party block in the detail pane, card facts unchanged. Heartbeat without `player` hides the block.

---

## Self-review

| Spec item | Task |
|-----------|------|
| `farm.Player` on heartbeat, omitempty / empty party | 1 |
| `/trace.json` opaque player blob | 2 |
| RAM decode + heartbeat + local TracePlayer | 3 |
| Wall dashboard / persist / retry | 4 |
| Local watch panel | 5 |
| pokeui detail-only Party block | 6 |
| Cards unchanged | 6 test |
| No new routes, no Progress change, no bag | out of scope |
