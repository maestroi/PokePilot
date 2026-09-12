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

Install and build from this directory:

```sh
npm install
npm run typecheck
npm run build
```

`npm run build` produces independent static trees for the two trust surfaces:

- `cmd/pokeui/ui/vue/operator/`
- `cmd/pokeui/ui/vue/spectator/`

Production Docker builds those assets before compiling `pokeui`, which embeds them with Go. Node/Vite is not present in or required by the runtime image. A Go-only checkout still compiles before the frontend has been built and falls back to the legacy page at root.

The Vue app is the normal `/` UI. During cutover the previous vanilla operator/spectator page remains reachable at `/legacy/`. The old `/next/` preview URL redirects to `/`.

Vue HTML is served with `Cache-Control: no-store`; Vite's hashed assets are immutable. Operator and spectator resolve files from different embedded subtrees so the public process cannot serve the private operator entry/bundle.

## Tailwind Plus

PokePilot may adapt Tailwind Plus application and marketing UI patterns as part of the finished PokePilot application. Licensed Tailwind Plus source archives and the complete block/template catalog are **not vendored into this repository**.

Only the specific markup and interaction patterns actually adapted into PokePilot should be committed. Shared PokePilot components exist to support this application and its operator/spectator surfaces; this directory must not become a redistribution of the Tailwind Plus component library or a general-purpose template product.
