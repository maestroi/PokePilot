# syntax=docker/dockerfile:1
#
# PokePilot farm image: one image, three entrypoints (pokepilot runner,
# pokewall wall, pokeui operator console). See
# docs/archive/2026-08-26-farm-design.md section 6 and
# docs/archive/2026-08-29-pokefarm-wall-ui-design.md.
#
# GomeBoy is pinned to the PokePilot-maintained GitHub fork in go.mod, so a
# normal repository checkout is a complete build context. No developer-local
# dependency checkout or BuildKit named context is required.
#
# The ROM is never part of this image: .dockerignore keeps roms/, *.gb,
# *.sav and *.state out of the build context, and no COPY below touches a
# ROM path. Runners receive the ROM by bind mount at runtime (deploy/farm.yml).

FROM golang:1.26 AS build

WORKDIR /src
COPY . .

# CGO_ENABLED=0: the golang image ships gcc, so a plain `go build` emits
# dynamically-linked glibc binaries. The runtime stage is alpine (musl),
# which has no /lib64/ld-linux-x86-64.so.2, so exec fails with "no such
# file or directory" and every container exits 255 in a crash loop.
# Static pure-Go binaries are the only kind that can cross that boundary.
ARG GIT_SHA=dev
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${GIT_SHA}" -o /out/pokepilot ./cmd/pokepilot && \
    CGO_ENABLED=0 go build -ldflags "-X main.version=${GIT_SHA}" -o /out/pokewall ./cmd/pokewall && \
    CGO_ENABLED=0 go build -ldflags "-X main.version=${GIT_SHA}" -o /out/pokeui ./cmd/pokeui

# Slim runtime: the three binaries plus CA certificates (the wall serves
# HTTP and the runners may talk to an LLM endpoint over TLS). No ROM, no
# sources.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && update-ca-certificates
COPY --from=build /out/pokepilot /usr/local/bin/pokepilot
COPY --from=build /out/pokewall /usr/local/bin/pokewall
COPY --from=build /out/pokeui /usr/local/bin/pokeui
