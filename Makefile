# Pokemon Red ROM. Override: make run POKEMON_RED_ROM=/path/to/red.gb
# roms/ is gitignored, so an agent-runner worktree has none — fall back to
# ~/.config/pokepilot/, outside every checkout. The checkout's own copy wins,
# and the plain path stays the default so the not-found message names it.
POKEMON_RED_ROM ?= $(firstword $(wildcard $(CURDIR)/roms/pokemon_red.gb $(HOME)/.config/pokepilot/pokemon_red.gb) $(CURDIR)/roms/pokemon_red.gb)
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
# Optional Agent Orchestrator issue handoff (wall only). Empty values
# leave reporting disabled. LAN examples, not defaults:
#   AGENT_ORCHESTRATOR_API=http://192.168.50.81:8080
#   AGENT_ORCHESTRATOR_UI=http://192.168.50.81:8081
#   AGENT_ORCHESTRATOR_POKEPILOT_PROJECT_ID=<project uuid>
GOMEBOY_CONTEXT ?= ../gomeboy

# A model served locally instead of the LAN box .env points at. It is the
# same run as run-llm with the endpoint, the model name and the reply room
# overridden — nothing about the agent changes, only who answers.
#
# MODEL must match what the server reports: the planner rejects a reply
# whose model field names a different one (a mismatch would make an
# ablation compare a model to itself). Ask the server what it serves:
#   curl -s localhost:8002/v1/models | head -c 200
#
# NO_THINK is the whole reason this target is usable. MEASURED 2026-08-31
# on one 16-objective menu: thinking on, 47s and 4096 completion tokens,
# truncated mid-thought and rejected as finish_reason "length"; thinking
# off, 0.88s and 22 tokens, a clean answer. A coding model reasons its way
# through a menu at temperature 0 and never gets to the JSON.
#
# MAX_TOKENS is then room, not a leash — a rejected truncation reads as a
# broken model rather than a short one, so leave headroom. TIMEOUT is
# generous because a card shared with your editor is not a card answering
# only this.
LOCAL_LLM_URL ?= http://localhost:8002/v1
LOCAL_LLM_MODEL ?= qwen3.8-27b
LOCAL_LLM_NO_THINK ?= 1
LOCAL_LLM_MAX_TOKENS ?= 1024
LOCAL_LLM_TIMEOUT ?= 300s

.PHONY: run run-60 run-0 run-llm run-llm-local test farm-image farm-up farm-down

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

# .env carries llm_token, the API key for the model server. Agent runners get a
# fresh git worktree and .env is gitignored, so it is never there — fall back to
# ~/.config/pokepilot/env, which lives outside every checkout. Local .env wins.
load_env = set -a; for f in $$HOME/.config/pokepilot/env ./.env; do [ -f "$$f" ] && . "$$f"; done; set +a;
run-llm:
	$(require-rom)
	$(load_env) \
	go run ./cmd/pokepilot -planner llm -fps 0 $(ARGS)

# The local model answers without a key; .env's llm_token is for the LAN
# box, and sending someone else's bearer token to localhost is not a thing
# to do by accident, so it is cleared rather than inherited.
run-llm-local:
	$(require-rom)
	POKEPILOT_LLM_URL=$(LOCAL_LLM_URL) \
	POKEPILOT_LLM_MODEL=$(LOCAL_LLM_MODEL) \
	POKEPILOT_LLM_NO_THINK=$(LOCAL_LLM_NO_THINK) \
	POKEPILOT_LLM_MAX_TOKENS=$(LOCAL_LLM_MAX_TOKENS) \
	POKEPILOT_LLM_TIMEOUT=$(LOCAL_LLM_TIMEOUT) \
	llm_token= \
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
	$(load_env) \
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

