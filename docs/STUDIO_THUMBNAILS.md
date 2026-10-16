# Thumbnail Studio (#802)

Thumbnails are a **derived, review-only artifact** of a verified #801 content package. The renderer never influences gameplay, a producer assessment, an edit plan, or YouTube publication. Only #803 may use an approved thumbnail during publication.

## Operator workflow

1. Generate a publishable producer assessment (#797), edit plan (#799) where desired, and a verified content package (#801).
2. In the private operator UI, open **Media → Studio**, choose the recorded run and paste its 64-character **packaging hash** into **Thumbnail studio**.
3. Load the package, select 1–3 variants per thumbnail concept, and choose whether to request optional image-provider artwork.
4. Generate previews. Each candidate is a real **1280 × 720 JPEG**, with deterministic Go-font typography, safe-area positioning, PokePilot branding, and a palette/layout variant.
5. Compare candidate images and click **Select candidate**. The selection is durable, tied to the current immutable manifest, and does **not** upload or publish anything.

If the provider is not configured or fails, the renderer produces the complete thumbnail using a deterministic branded built-in illustration; `artwork_status` reflects `unconfigured` or `fallback`. Repeating generation with a working provider is safe and retryable.

## Private replay/operator API

```http
POST /v1/runs/{run-id}/studio/packages/{packaging-sha256}/thumbnails
Content-Type: application/json

{"variants":2,"generated_art":false}
```

The reply includes `manifest`, containing the source package and template versions, per-variant render-spec hashes, image sizes/SHA-256, provider IDs, immutable artwork references (when present), and manifest checksum. Defaults: 2 variants, built-in artwork. Maximum: 3 variants × concepts. The image provider may be called once per concept and that asset reused across the concept's layout variants.

```http
GET  /v1/runs/{run-id}/studio/packages/{packaging-sha256}/thumbnails
GET  /v1/runs/{run-id}/studio/packages/{packaging-sha256}/thumbnails/{candidate-sha256}/image
POST /v1/runs/{run-id}/studio/packages/{packaging-sha256}/thumbnails/{candidate-sha256}/select
```

The manifest GET returns `selected_id` when the selection references the **current** manifest. Selection requires that candidate's stored JPEG to exist. If the manifest is regenerated with new creative art, a previous incompatible selection is no longer shown. All four routes are **private operator-only** in `pokeui`; the public spectator process does not expose them. `pokereplay` rechecks the package's checksum, source assessment and current semantic evidence before rendering or presenting the candidates.

S3 hierarchy is derived from the packaging object's existing run-scoped namespace. `images/{spec.hash}.jpg` and `artwork/{asset_sha256}.bin` are immutable content-addressed inputs; `manifest.json` holds the latest set of choices; `selection.json` holds an explicit human choice. Regenerating never edits the producer assessment, original run or replay.

## Optional generated artwork

All provider configuration lives on the server:

- `POKEPILOT_THUMBNAIL_IMAGE_URL`: trusted OpenAI-compatible `images/generations` HTTP(S) endpoint returning `data[0].b64_json`
- `POKEPILOT_THUMBNAIL_IMAGE_MODEL`: configured image model identifier
- `POKEPILOT_THUMBNAIL_IMAGE_DEPLOYMENT`: optional provider identity suffix
- `POKEPILOT_THUMBNAIL_IMAGE_API_KEY`: optional server-only bearer token

Image generation is **off by default**, including when a provider is configured. No user-supplied endpoint or prompt is accepted. Providers see only allowlisted public thumbnail subject labels, a layout/palette number, and a nonliteral-artwork instruction ("no words, no logos, no watermarks"). Raw image bytes are bounded to 5 MiB; PNG/JPEG dimensions are validated **before decode**, limited to 4096 on either axis and 12 megapixels; SVG/external-image URLs are never loaded. Upstream errors never expose the provider response body or its credentials.

The image-provider model, renderer version, template version, source package, concept ID, variant, and source asset digest are bound into each image's render spec. Text is **never** delegated to image generation: only #801's verified thumbnail concept text, `POKEPILOT` and a generic autonomous-run footer can be drawn by the compositor. A provider failure cannot introduce alternate factual copy. Generated output always remains JPEG, max 2 MiB.

## Verification

```sh
go test ./media/thumbnail ./cmd/pokereplay ./cmd/pokeui
cd web && npm run typecheck && npm test
```

Tests cover deterministic regeneration and visibly different layouts, rendered JPEG dimensions/upload ceilings, invalid/outdated spec identities, artwork size/type limits, provider HTTP credentials and failure sanitation. #803 will consume the selected candidate ID and artwork manifest; no additional YouTube uploader is introduced here.
