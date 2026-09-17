# Portable farm repro bundles

Automated farm issues can publish the exact failing checkpoint as a small,
ROM-free ZIP on GitHub so a fixer does not need network access to the private
PokePilot farm.

## GitHub token permissions

`pokeissues` uses the existing `POKEPILOT_GITHUB_TOKEN`. For a fine-grained
personal access token, restrict repository access to `maestroi/PokePilot` and
grant only:

- **Issues: Read and write** — create/deduplicate/reopen farm issues and comments.
- **Contents: Read and write** — create the long-lived `farm-repros` prerelease
  and upload release assets.

No Actions, Workflows, Administration, Packages, or repository-secret permission
is required. GitHub grants Metadata read access implicitly for repository-scoped
tokens.

## What is published

For failures that carry a replayable objective checkpoint, `pokeissues` retains
only the exact matching evidence needed for local reproduction and writes a ZIP
containing:

- `repro.json` — run/attempt/revision/fingerprint and bounded failure metadata;
- the exact `round-*.state` checkpoint;
- its matching `round-*.knowledge-vN.json` agent knowledge;
- the matching bounded `failure-repro.json` contract when present.

The ROM, model/API credentials, recordings, screenshots, raw model exchanges,
and unrelated run artifacts are not included.

The ZIP is uploaded as a content-addressed asset on one long-lived GitHub
prerelease/tag named `farm-repros`. The farm issue is then updated with the
asset URL, ZIP SHA-256, and a command such as:

```sh
go run ./cmd/pokerepro -bundle 'https://github.com/maestroi/PokePilot/releases/download/farm-repros/repro-issue-123-....zip' -play
```

`pokerepro -bundle` accepts either that public HTTPS URL or a local ZIP path,
verifies the embedded state/knowledge hashes, materializes the pair beside each
other, and can launch the current checkout without contacting
`admin.rompilot.app`.

Because the PokePilot repository is public, these release assets are public too.
Only enable this workflow for evidence that is safe to publish.
