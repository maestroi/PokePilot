# syntax=docker/dockerfile:1
#
# PokePilot farm image: one image with runner, wall, private/public UI, and
# replay-sidecar entrypoints. See docs/RUN_INSPECTOR.md.
#
# GomeBoy is pinned to the PokePilot-maintained GitHub fork in go.mod, so a
# normal repository checkout is a complete build context.
#
# The ROM is never part of this image: .dockerignore keeps roms/, *.gb,
# *.sav and *.state out of the build context. Runners and the replay sidecar
# receive the ROM by read-only bind mount at runtime.

FROM golang:1.26 AS build

WORKDIR /src
COPY . .

ARG GIT_SHA=dev
ARG GIT_PR_NUMBER=0
ARG GIT_PR_TITLE_B64=
ARG GIT_REPOSITORY=maestroi/PokePilot
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${GIT_SHA}" -o /out/pokepilot ./cmd/pokepilot && \
    CGO_ENABLED=0 go build -ldflags "-X main.version=${GIT_SHA}" -o /out/pokewall ./cmd/pokewall && \
    CGO_ENABLED=0 go build -ldflags "-X main.version=${GIT_SHA} -X main.buildPR=${GIT_PR_NUMBER} -X main.buildTitleB64=${GIT_PR_TITLE_B64} -X main.buildRepo=${GIT_REPOSITORY}" -o /out/pokeui ./cmd/pokeui && \
    CGO_ENABLED=0 go build -o /out/pokereplay ./cmd/pokereplay && \
    CGO_ENABLED=0 go build -o /out/gomeboy-stream github.com/maestroi/gomeboy/cmd/gomeboy-stream

# FFmpeg is runtime-only and is used by gomeboy-stream to encode deterministic
# .gbrun replays into cached MP4s. No ROM or source is copied into the image.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates ffmpeg && update-ca-certificates
COPY --from=build /out/pokepilot /usr/local/bin/pokepilot
COPY --from=build /out/pokewall /usr/local/bin/pokewall
COPY --from=build /out/pokeui /usr/local/bin/pokeui
COPY --from=build /out/pokereplay /usr/local/bin/pokereplay
COPY --from=build /out/gomeboy-stream /usr/local/bin/gomeboy-stream
