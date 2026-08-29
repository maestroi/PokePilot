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
# Host port for pokeui (operator console). 8080 is taken by other local
# services, so it defaults to 18080. Host-mode publish in farm.yml, not
# swarm ingress. Override: make farm-up FARM_WALL_PORT=9999
FARM_WALL_PORT ?= 18080
export FARM_WALL_PORT
# Wall's durable state (tile map + finish dumps). A named volume loses
# its data on every task rollover here, so the wall remembers across
# restarts only via this host bind mount.
FARM_STATE_DIR ?= /tmp/pokefarm-state
export FARM_STATE_DIR
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
	mkdir -p "$(FARM_STATE_DIR)"
	# A leftover standalone pokefarm_ui (from before it joined the stack)
	# would hold the host port and block the new task.
	docker rm -f pokefarm_ui >/dev/null 2>&1 || true
	# llm_token (and optional POKEPILOT_LLM_*) live in .env for
	# make run-llm. Source the same file here so farm runners get the
	# key; docker stack deploy interpolates ${llm_token} from the env.
	set -a; [ -f .env ] && . ./.env; set +a; \
	docker stack deploy --resolve-image never -c deploy/farm.yml pokefarm
	# The image tag does not change between builds, so the service spec is
	# identical and Docker would not roll healthy tasks — a rebuilt image
	# would never land. (Crash-looping tasks pick it up on their own; this
	# is how the CGO fix slipped through.) Force the rollout.
	docker service update --force --detach pokefarm_wall
	docker service update --force --detach pokefarm_runner
	docker service update --force --detach pokefarm_ui
	@echo "pokefarm UI: http://localhost:$(FARM_WALL_PORT)/"

farm-down:
	docker rm -f pokefarm_ui >/dev/null 2>&1 || true
	docker stack rm pokefarm

