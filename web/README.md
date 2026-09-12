# PokePilot web frontend

The browser-facing operator and public spectator applications are migrating to Vue 3 + TypeScript + Vite with a shared Tailwind CSS design system.

## Stack

- Vue 3
- TypeScript
- Vite
- Tailwind CSS 4
- Headless UI for accessible interactive primitives
- Heroicons for icons

The small local/emulator utility UI is intentionally outside this workspace and remains plain HTML/JavaScript.

## Development and builds

Install and run the frontend from this directory:

```sh
npm install
npm run typecheck
npm run build
```

`npm run build` produces two independent trees:

- `cmd/pokeui/ui/vue/operator/`
- `cmd/pokeui/ui/vue/spectator/`

The generated files are gitignored and embedded by Go at compile time. Production Docker builds the frontend first and copies those trees into the Go build stage, so Node/Vite is not required at runtime.

During the migration the existing vanilla UI stays at `/`. The Vue build is available from the same `pokeui` process at `/next/`:

- private `pokeui`: `/next/` serves only the operator tree
- `pokeui -spectator`: `/next/` serves only the spectator tree and keeps the spectator security headers/read-only backend surface

HTML is served with `Cache-Control: no-store`; Vite's hashed assets are served immutable. This lets a deployment move between revisions without a browser remaining stuck on old application code.

A Go-only checkout still compiles before `npm run build`; `/next/` returns 503 with a build instruction until the generated frontend exists. CI builds the frontend before the Go suite, so embed/route integration is exercised there.

## Tailwind Plus

PokePilot may adapt Tailwind Plus application and marketing UI patterns as part of the finished PokePilot application. Licensed Tailwind Plus source archives and the complete block/template catalog are **not vendored into this repository**.

Only the specific markup and interaction patterns actually adapted into PokePilot should be committed. Shared PokePilot components exist to support this application and its operator/spectator surfaces; this directory must not become a redistribution of the Tailwind Plus component library or a general-purpose template product.

## Migration

The Vue applications coexist with the existing Go-embedded vanilla UIs. Views move across incrementally. Until an individual view is explicitly cut over, the existing UI remains authoritative. `/next/` is the preview/canary surface for checking migrated views against live farm data before changing `/`.
