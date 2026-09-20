# syntax=docker/dockerfile:1
#
# PokePilot farm image: one image with runner, wall, private/public UI, replay,
# GitHub issue-sink, and Gen-I link/trade entrypoints. See docs/RUN_INSPECTOR.md.
#
# GomeBoy is pinned to the PokePilot-maintained GitHub fork in go.mod, so a
# normal repository checkout is a complete build context.
#
# The ROM is never part of this image: .dockerignore keeps roms/, *.gb,
# *.sav and *.state out of the build context. Runners, replay, and the virtual
# trader receive the ROM by read-only bind mount at runtime.

FROM node:22-alpine AS web

WORKDIR /src/web
COPY web/package.json ./
RUN npm install --no-audit --no-fund
COPY web/ ./
# prebuild generate:world reads vendored pokered map constants/headers.
COPY pokered/constants/map_constants.asm /src/pokered/constants/map_constants.asm
COPY pokered/data/maps/headers/ /src/pokered/data/maps/headers/
COPY pokered/data/maps/objects/ /src/pokered/data/maps/objects/
RUN npm run build

FROM golang:1.26 AS build

WORKDIR /src
COPY . .
COPY --from=web /src/cmd/pokeui/ui/vue ./cmd/pokeui/ui/vue

ARG GIT_SHA=dev
ARG GIT_PR_NUMBER=0
ARG GIT_PR_TITLE_B64=
ARG GIT_REPOSITORY=maestroi/PokePilot
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${GIT_SHA}" -o /out/pokepilot ./cmd/pokepilot && \
    CGO_ENABLED=0 go build -ldflags "-X main.version=${GIT_SHA}" -o /out/pokewall ./cmd/pokewall && \
    CGO_ENABLED=0 go build -ldflags "-X main.version=${GIT_SHA} -X main.buildPR=${GIT_PR_NUMBER} -X main.buildTitleB64=${GIT_PR_TITLE_B64} -X main.buildRepo=${GIT_REPOSITORY}" -o /out/pokeui ./cmd/pokeui && \
    CGO_ENABLED=0 go build -o /out/pokereplay ./cmd/pokereplay && \
    CGO_ENABLED=0 go build -o /out/pokeissues ./cmd/pokeissues && \
    CGO_ENABLED=0 go build -o /out/gen1-trade-peer ./cmd/gen1-trade-peer

# Keep the replay renderer on the v1.1.0 core used by PokePilot, plus the
# one-line model-selection fix that makes CGB recordings replay as CGB while
# historical DMG recordings continue to replay as DMG. The link broker is
# installed from the same v1.3.0 GomeBoy release PokePilot compiles against.
RUN GOBIN=/out CGO_ENABLED=0 go install github.com/maestroi/gomeboy/cmd/gomeboy-stream@26c3dbd668c2351ade5d3ca57019a2a0882255f8 && \
    GOBIN=/out CGO_ENABLED=0 go install github.com/maestroi/gomeboy/cmd/gomeboy-link-broker@v1.3.0

# FFmpeg is runtime-only and is used by gomeboy-stream to encode deterministic
# .gbrun replays into cached MP4s. intel-media-driver is the userspace half of
# Iris Xe / Quick Sync (iHD). No ROM or source is copied into the image.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates ffmpeg intel-media-driver && update-ca-certificates
COPY --from=build /out/pokepilot /usr/local/bin/pokepilot
COPY --from=build /out/pokewall /usr/local/bin/pokewall
COPY --from=build /out/pokeui /usr/local/bin/pokeui
COPY --from=build /out/pokereplay /usr/local/bin/pokereplay
COPY --from=build /out/pokeissues /usr/local/bin/pokeissues
COPY --from=build /out/gen1-trade-peer /usr/local/bin/gen1-trade-peer
COPY --from=build /out/gomeboy-link-broker /usr/local/bin/gomeboy-link-broker
COPY --from=build /out/gomeboy-stream /usr/local/bin/gomeboy-stream
# The manager-side updater is deliberately only a stable bootstrap. Changing
# rollout behavior and LiteLLM config travel inside the exact image digest so
# a stale host copy cannot keep the farm pinned to old deployment logic.
COPY deploy/rollout-latest.sh /usr/local/share/pokepilot/deploy/rollout-latest.sh
COPY deploy/litellm.yaml /usr/local/share/pokepilot/deploy/litellm.yaml
RUN chmod +x /usr/local/share/pokepilot/deploy/rollout-latest.sh && \
    ln -s pokereplay /usr/local/bin/ffmpeg-vaapi
