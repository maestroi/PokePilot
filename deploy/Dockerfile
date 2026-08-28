# syntax=docker/dockerfile:1
#
# PokePilot farm image: one image, two entrypoints (pokepilot runner and
# pokewall wall). See docs/plans/2026-08-26-farm-design.md section 6.
#
# go.mod replaces github.com/thelolagemann/gomeboy with the ABSOLUTE path
# /home/maestro/Documents/projects/gomeboy, so the build must place the
# gomeboy checkout at exactly that location inside the builder. It arrives
# as a BuildKit named context, passed by the operator as
#   --build-context gomeboy=<path-to-gomeboy-checkout>
# (the Makefile does this via GOMEBOY_CONTEXT). A plain `COPY .` of this
# repo can never satisfy the replace, which is why the old task's
# Dockerfile could not build.
#
# The ROM is never part of this image: .dockerignore keeps roms/, *.gb,
# *.sav and *.state out of the build context, and no COPY below touches a
# ROM path. Runners receive the ROM by bind mount at runtime (deploy/farm.yml).

FROM golang:1.26 AS build

WORKDIR /src

# The named context first, at the exact path go.mod's replace expects.
COPY --from=gomeboy . /home/maestro/Documents/projects/gomeboy/
# Then this repo, as the main module.
COPY . /home/maestro/Documents/projects/PokePilot/

WORKDIR /home/maestro/Documents/projects/PokePilot
RUN go build -o /out/pokepilot ./cmd/pokepilot && \
    go build -o /out/pokewall ./cmd/pokewall

# Slim runtime: the two binaries plus CA certificates (the wall serves HTTP
# and the runners may talk to an LLM endpoint over TLS). No ROM, no sources.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && update-ca-certificates
COPY --from=build /out/pokepilot /usr/local/bin/pokepilot
COPY --from=build /out/pokewall /usr/local/bin/pokewall
