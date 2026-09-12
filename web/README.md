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

## Tailwind Plus

PokePilot may adapt Tailwind Plus application and marketing UI patterns as part of the finished PokePilot application. Licensed Tailwind Plus source archives and the complete block/template catalog are **not vendored into this repository**.

Only the specific markup and interaction patterns actually adapted into PokePilot should be committed. Shared PokePilot components exist to support this application and its operator/spectator surfaces; this directory must not become a redistribution of the Tailwind Plus component library or a general-purpose template product.

## Migration

The Vue applications initially coexist with the existing Go-embedded vanilla UIs. Views move across incrementally. Until an individual view is explicitly cut over, the existing UI remains authoritative.
