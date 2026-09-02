# Player roster in local watch and pokeui — Design

**Date:** 2026-09-02
**Status:** approved (maestro, 2026-09-02)

## Context

The runner already decodes party, money, and badges every round
(`agent.Observe` / `state.Read`). Neither operator surface shows them.

- Local watch (`emu/watch.go`, port 8099) renders the LLM tally from
  `/trace.json` (`stats`) and the event trace. No game-state panel.
- pokeui renders map/tile, frame, Plan, and Play (LLM stats). Cards and
  the detail pane have no party, money, or badges.

`farm.Progress` is a start/end eval sample (badge *count*, no party). It
is the wrong contract for a live inspector.

## Goal

When an operator watches a run — locally or in pokeui's detail pane — they
see the pause-menu glance: money, badges, and each party member's name,
level, HP, and status. Cards stay unchanged.

## Decisions

- **Live snapshot on the existing pipes, not a new endpoint.** Same pattern
  as LLM stats: RAM → typed field → heartbeat / `/trace.json` → UI.
- **A separate field, not stuffed into `stats`.** Game RAM is not a planner
  tally. Scripted walks have no `stats` and still have a party. HP must
  move during a battle, not only when the model is asked.
- **Typed `farm.Player`, not `json.RawMessage` on the farm wire.** farm
  already owns the heartbeat contract and must not import `agent`. Species
  are named in `cmd/pokepilot` before they hit the wire.
- **Opaque blob on `/trace.json`.** emu is not allowed to know what a
  Pokémon is. `TracePlayer` mirrors `TraceStats`: marshal here, carry
  verbatim.
- **Detail pane only in pokeui.** No roster on live cards. Local watch has
  no cards; the side panel is the detail pane.
- **Kept on finish, reset on retry.** Same rule as `stats`: the last
  snapshot explains a done run; a new attempt starts empty.
- **Omit the block when `player` is absent.** Old runners and the first
  frames before a sample. Never invent zeros. An empty party (pre-starter)
  is a real snapshot (`"party":[]`), not an omitted field.

## Data flow

```
cmd/pokepilot  --state.Read-->  farm.Player  { money, badges[], party[] }
                                  |
                    +-------------+-------------+
                    |                           |
              /trace.json                  heartbeat ~1 Hz
              emu.TracePlayer              wall tile → /v1/dashboard
                    |                           |
              local watch #party           pokeui Party block
```

1. **Wire (`farm/spec.go`).** `Player` + `PartyMon` (name, level, hp,
   max_hp, status). `Heartbeat.Player *Player json:"player,omitempty"`.
   Nil = old runner / unsampled. Empty party still encodes.
2. **Runner (`cmd/pokepilot`).** `playerSnapshot(state.GameState) *farm.Player`
   names species via `agent.SpeciesName` (unknown → `species 0xNN`) and
   badges via `state.Badge.String()`, same loop as `Observe`.
   `sampleHeartbeat` already has the `GameState`; it fills `Player` and
   also calls `TracePlayer` so a farm worker's own :8099 page matches
   pokeui. Local `OnSample` composes the same snapshot onto `TracePlayer`.
   `storeStatus` takes the new RAM `Player` and keeps planner fields
   (`stats`, question, decision, raw). A new lease `store`s a bare
   heartbeat and clears it.
3. **Wall (`cmd/pokewall`).** `Tile.Player`, copied in `handleHeartbeat`,
   added to `tileRow` and `persistedTile`. `settleRun` keeps it on done,
   nils it on retry. Re-queue nils it like `Stats`.
4. **Local watch (`emu/watch.go`).** A `#party` panel above the LLM tally,
   independent of it, so scripted runs still show a roster. Hidden when
   the blob is absent.
5. **pokeui (`cmd/pokeui/ui`).** A Party block in the detail pane after
   Now / Plan / Play. Cards do not read `r.player`.

## Party block

Same layout on both surfaces:

- Summary: `₽1840 · Boulder` (money, then badge names; `no badges` when
  the list is empty).
- 0–6 rows: name, Lv.n, HP bar with `12/35`, status if any.
- HP colour: muted when full, amber under half, red under 20% or fainted.
- Empty party: summary plus `no Pokémon yet`. No fake slots.

The local stats column keeps its height cap so six rows cannot eat the
trace.

## Out of scope

- Card-line roster or HP bars on the run rail.
- Bag, box, Pokedex count, player name.
- Changing `farm.Progress`, planner prompts, or `agent.Observation`.
- New pokeui / wall HTTP routes.

## Testing

- **farm:** heartbeat JSON round-trip with `player` set, nil (key omitted),
  and empty party (`"party":[]` present).
- **pokepilot:** `playerSnapshot` names a known species and formats an
  unknown as `species 0xNN`; `storeStatus` takes the new snapshot and does
  not blank `stats`; a new lease clears `player`.
- **pokewall:** dashboard includes `player`; finish keeps it; retry clears
  it; wall restart restores it.
- **emu:** `/trace.json` carries the opaque `player` blob; unset omits the
  key.
- **pokeui:** source-grep for the Party block and `r.player`, and that
  live cards do not render roster rows.

No emulator journey. This is wire + render.

## Verification

`go test ./farm ./emu ./cmd/pokepilot ./cmd/pokewall ./cmd/pokeui`

Manual smoke: local `pokepilot` watch shows the roster after the starter;
wall + pokeui with a heartbeat carrying `player` shows the Party block in
the detail pane and nothing extra on the card.
