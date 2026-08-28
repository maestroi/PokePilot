# Pokemon Red ROM. Override: make run POKEMON_RED_ROM=/path/to/red.gb
POKEMON_RED_ROM ?= $(CURDIR)/roms/pokemon_red.gb
export POKEMON_RED_ROM

# Extra flags, e.g. make run ARGS='-goto "pallet town"'
ARGS ?=

# Local single-node Swarm farm (docs/plans/2026-08-26-farm-design.md 6).
# The image is built locally and loaded into this node's image store;
# `farm-up` deploys with --resolve-image never so the local Swarm uses it.
# A multi-node Swarm cannot see a --load'ed image on other nodes: publish
# FARM_IMAGE to a registry and point it at that reference instead.
FARM_IMAGE ?= pokepilot-farm:local
GOMEBOY_CONTEXT ?= ../gomeboy

.PHONY: run run-60 run-0 run-llm test farm-image farm-up farm-down

require-rom = @test -f "$(POKEMON_RED_ROM)" || { \
	echo "POKEMON_RED_ROM not found: $(POKEMON_RED_ROM)"; \
	echo "point it at a Pokemon Red ROM"; \
	exit 1; \
}

run: run-60

run-60:
	$(require-rom)
	go run ./cmd/pokepilot -fps 60 $(ARGS)

run-0:
	$(require-rom)
	go run ./cmd/pokepilot -fps 0 $(ARGS)

# .env carries llm_token, the API key for the model server.
run-llm:
	$(require-rom)
	set -a; [ -f .env ] && . ./.env; set +a; \
	go run ./cmd/pokepilot -planner llm -fps 0 $(ARGS)

test:
	go test ./... $(ARGS)

# go.mod replaces gomeboy with an absolute path, so the checkout arrives as
# a BuildKit named context that the Dockerfile copies to that exact path.
farm-image:
	docker buildx build --load --build-context gomeboy=$(GOMEBOY_CONTEXT) -t $(FARM_IMAGE) -f deploy/Dockerfile .

farm-up: farm-image
	$(require-rom)
	docker stack deploy --resolve-image never -c deploy/farm.yml pokefarm

farm-down:
	docker stack rm pokefarm

