# Calm Navy Operator Console Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Soften the PokéFarm console's overall tone without changing its layout or behavior.

**Architecture:** Keep the refinement entirely in the existing CSS visual system. Update shared tokens first, then replace the few hard-coded near-black and high-contrast colors that bypass those tokens; no HTML or JavaScript changes are required.

**Tech Stack:** Embedded HTML/CSS, Go static asset tests, Docker Swarm local farm.

---

### Task 1: Pin the calm palette contract

**Files:**
- Modify: `cmd/pokeui/console_test.go`

1. Add a failing test that requires the calm navy canvas, gently stepped surfaces, lower-contrast borders, warm text, muted semantic accents, and no coral header rule.
2. Run `go test ./cmd/pokeui -run TestUIUsesCalmNavyPalette -count=1` and confirm it fails against the current values.

### Task 2: Apply the CSS-only refinement

**Files:**
- Modify: `cmd/pokeui/ui/console.css`

1. Replace the shared palette tokens with the approved calm navy values.
2. Replace hard-coded near-black media/rail/timeline surfaces and strong error/button borders with compatible muted values.
3. Remove the coral application-header rule while preserving its one-pixel structural divider.
4. Run `go test ./cmd/pokeui -count=1`, `git diff --check`, and the Impeccable detector.

### Task 3: Verify and run locally

**Files:**
- No source changes expected.

1. Run `make verify` and require a zero exit code.
2. Run `make farm-up` and wait for every service to converge.
3. Compare the live `/console.css` byte-for-byte with `cmd/pokeui/ui/console.css` and probe the dashboard endpoint.
