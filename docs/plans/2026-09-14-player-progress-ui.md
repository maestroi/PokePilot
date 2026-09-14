# Player Progress UI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show bag space, Pokédex counts, and major story milestones on the operator and spectator live player views.

**Architecture:** Extend `farm.Player` with portable bag/Dex/milestone fields. The Red runner projects names and adapter limits in `playerSnapshot`. Vue reads those fields only; it never hardcodes 20 or 151.

**Tech Stack:** Go (`farm`, `cmd/pokepilot`, `cmd/pokeui`, `red/state`, `red/profile`), Vue 3 + TypeScript in `web/`.

---

### Task 1: Adapter constants and milestone labels

**Files:**
- Modify: `red/state/inventory.go`
- Create: `red/profile/milestones.go`
- Test: `red/profile/milestones_test.go`
- Test: `red/state/inventory.go` (use `BagCapacity` in `DecodeInventory`)

**Step 1:** Export `BagCapacity = 20` from `red/state` and use it in `DecodeInventory`.

**Step 2:** Add `MajorMilestoneLabels(facts state.StoryFacts) []string` in `red/profile` for the approved completed-only list, in story order.

**Step 3:** Test the label order and that incomplete facts yield a nil/empty slice.

---

### Task 2: Snapshot contract

**Files:**
- Modify: `farm/spec.go`
- Modify: `farm/spec_test.go`
- Modify: `cmd/pokepilot/farm.go`
- Modify: `cmd/pokepilot/farm_test.go`
- Modify: `cmd/pokepilot/main.go`

**Step 1:** Add `BagItem` and the new `Player` fields (`bag_used`, `bag_capacity`, `bag`, `dex_owned`, `dex_seen`, `dex_total`, `milestones`).

**Step 2:** Change `playerSnapshot` to `playerSnapshot(g state.GameState, facts state.StoryFacts)`. Name bag items with `agent.ItemName`, set capacity/`sym.PokedexCount`, and attach `redprofile.MajorMilestoneLabels(facts)`.

**Step 3:** Update `sampleHeartbeat` and the watch tracer to decode story facts before snapshotting.

**Step 4:** Extend `TestPlayerSnapshotNamesParty` and heartbeat JSON round-trip tests.

---

### Task 3: Spectator public whitelist

**Files:**
- Modify: `cmd/pokeui/spectator_public.go`
- Modify: `cmd/pokeui/spectator_test.go`

Copy the new player fields into `publicPlayer`. Add bag/Dex/milestone values to the dashboard fixture and assert they appear in `/v1/watch` without leaking private metadata.

---

### Task 4: Shared web meters

**Files:**
- Create: `web/src/shared/playerProgress.ts`
- Create: `web/test/playerProgress.test.ts`
- Modify: `web/src/shared/api/types.ts`
- Modify: `web/src/shared/api/spectator.ts`

Add TypeScript fields and helpers that return `null` when capacity/total is missing, otherwise `14/20` and `12/151`.

---

### Task 5: Operator Game state

**Files:**
- Modify: `web/src/operator/LiveView.vue`

Header: money, bag meter, Dex meter, badges. Three `<details>` rows under the party grid for bag items, Dex owned/seen, and milestones.

---

### Task 6: Spectator strip, lists, and activity

**Files:**
- Modify: `web/src/spectator/App.vue`

6-up meters (3-up on narrow viewports). Party footer disclosures. Activity events for Dex owned increases and new milestone labels.

---

### Task 7: Verify

Run:

```
go test ./farm ./red/state ./red/profile ./cmd/pokepilot ./cmd/pokeui -count=1
cd web && npm test && npx vue-tsc --noEmit
```
