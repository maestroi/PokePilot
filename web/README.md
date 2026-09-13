# PokePilot web frontend

The browser-facing operator and public spectator applications use Vue 3 + TypeScript + Vite with a shared Tailwind CSS design system.

## Stack

- Vue 3
- TypeScript
- Vite
- Tailwind CSS 4
- Headless UI for accessible interactive primitives
- Heroicons for icons

The small local/emulator utility UI intentionally stays outside this workspace as plain HTML/JavaScript.

## Development and builds

For hot-reload development, start the local farm first so `pokeui` owns the real APIs:

```sh
make farm-up
```

Then run the operator frontend from `web/`:

```sh
npm install
npm run dev
```

Open `http://localhost:5173/operator.html`. Vite proxies `/v1`, `/frame`, and `/maps` to the private `pokeui` at `http://localhost:18080`, so the dev page uses the same backend as the deployed console.

For the public read-only surface use:

```sh
npm run dev:spectator
```

and open `http://localhost:5173/spectator.html`; that mode proxies to `http://localhost:18081` instead. Set `POKEPILOT_DEV_BACKEND` before starting Vite to use a different local backend URL.

Run frontend behavior/API tests and typechecking with:

```sh
npm test
npm run typecheck
npm run build
```

`npm run build` runs the frontend tests, typechecks the Vue app, and produces independent static trees for the two trust surfaces:

- `cmd/pokeui/ui/vue/operator/`
- `cmd/pokeui/ui/vue/spectator/`

Production Docker builds those assets before compiling `pokeui`, which embeds them with Go. Node/Vite is not present in or required by the runtime image. A Go-only checkout still compiles before the frontend has been built, but a running `pokeui` requires built Vue assets and returns `503` at root if they are missing rather than exposing the retired vanilla console.

The Vue app is the normal `/` UI. The old `/next/` preview URL redirects to `/`; `/legacy/` is no longer exposed.

Vue HTML is served with `Cache-Control: no-store`; Vite's hashed assets are immutable. Operator and spectator resolve files from different embedded subtrees so the public process cannot serve the private operator entry/bundle. `/v1/build` exposes only non-secret deployment provenance (revision, PR number/title, and GitHub links) for the operator header.

## Tailwind Plus

PokePilot may adapt Tailwind Plus application and marketing UI patterns as part of the finished PokePilot application. Licensed Tailwind Plus source archives and the complete block/template catalog are **not vendored into this repository**.

Only the specific markup and interaction patterns actually adapted into PokePilot should be committed. Shared PokePilot components exist to support this application and its operator/spectator surfaces; this directory must not become a redistribution of the Tailwind Plus component library or a general-purpose template product.
