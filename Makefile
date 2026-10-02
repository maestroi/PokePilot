# Pokemon Red ROM. Override: make run POKEMON_RED_ROM=/path/to/red.gb
# roms/ is gitignored, so an agent-runner worktree has none — fall back to
# ~/.config/pokepilot/, outside every checkout. The checkout's own copy wins,
# and the plain path stays the default so the not-found message names it.
POKEMON_RED_ROM ?= $(firstword $(wildcard $(CURDIR)/roms/pokemon_red.gb $(HOME)/.config/pokepilot/pokemon_red.gb) $(CURDIR)/roms/pokemon_red.gb)
export POKEMON_RED_ROM

# Pokémon Yellow qualification is opt-in just like Red emulator fixtures. The
# ROM is never required by normal CI and is never committed.
POKEMON_YELLOW_ROM ?= $(firstword $(wildcard $(CURDIR)/roms/pokemon_yellow.gb $(HOME)/.config/pokepilot/pokemon_yellow.gb) $(CURDIR)/roms/pokemon_yellow.gb)
export POKEMON_YELLOW_ROM

# The whole ROM directory is mounted read-only into the farm runners, so a
# worker plays any cartridge the operator put there (Red, Blue, ...). It
# defaults to wherever the Red ROM was found.
POKEPILOT_ROM_DIR ?= $(dir $(POKEMON_RED_ROM))
export POKEPILOT_ROM_DIR

# Extra flags, e.g. make run ARGS='-goto "pallet town"'
ARGS ?=
# One-id coding-agent debugging. Example: make debug RUN=run-abc123
RUN ?=
DEBUG_MODE ?= normal

# Local single-node Swarm farm (docs/archive/2026-08-26-farm-design.md 6).
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
# GitHub farm issues. POKEPILOT_GITHUB_TOKEN is read from .env or
# ~/.config/pokepilot/env by farm-up and reaches only the issue adapter.
# A fine-grained token only needs Issues read/write for maestroi/PokePilot.
# POKEPILOT_GITHUB_REPO defaults to maestroi/PokePilot in deploy/farm.yml.

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
# Goal-driven by default. Zero means no hard round cap; agent.Run's short
# loop detector, long stagnation watchdog and frame watchdog still stop
# runaway sessions. Set this or ARGS='-max-rounds N' for a fixed experiment.
LOCAL_LLM_MAX_ROUNDS ?= 0

# GPU-first local routing. The primary is the same localhost model above.
# Give a real generation enough time to finish, but still fail over well
# before LOCAL_LLM_TIMEOUT when the 4090 is genuinely unavailable. Because
# statsPlanner pins the fallback after the first transport failure, this
# timeout should not turn a single slow generation into a permanent downgrade.
AUTO_LLM_URL ?= $(LOCAL_LLM_URL)
AUTO_LLM_MODEL ?= $(LOCAL_LLM_MODEL)
AUTO_LLM_NO_THINK ?= 1
AUTO_LLM_MAX_TOKENS ?= 1024
AUTO_LLM_TIMEOUT ?= 30s
AUTO_LLM_FALLBACK_URL ?= http://192.168.50.204:8000/v1
AUTO_LLM_FALLBACK_MODEL ?= qwen3.5-4b
AUTO_LLM_FALLBACK_TIMEOUT ?= 60s

.PHONY: run run-60 run-0 run-llm run-llm-local run-llm-auto debug test test-short test-race test-farm test-agent test-state test-yellow-rom verify-yellow-rom fmt-check vet verify farm-image farm-up farm-down roms-upload qwagent-triage-install fixer-image fixer-up fixer-down

require-rom = @test -f "$(POKEMON_RED_ROM)" || { \
	echo "POKEMON_RED_ROM not found: $(POKEMON_RED_ROM)"; \
	echo "point it at a Pokemon Red ROM"; \
	exit 1; \
}

require-yellow-rom = @test -f "$(POKEMON_YELLOW_ROM)" || { \
	echo "POKEMON_YELLOW_ROM not found: $(POKEMON_YELLOW_ROM)"; \
	echo "point it at the supported Pokemon Yellow EN rev0 ROM"; \
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
	go run ./cmd/pokepilot -planner llm -fps 0 -max-rounds $(LOCAL_LLM_MAX_ROUNDS) $(ARGS)

# Prefer the idle 4090, but keep the LAN/CPU model as a real fallback. The
# fallback URL/model are taken from the sourced POKEPILOT_LLM_* values when
# present, so an operator's .env still wins over these Make defaults. Capture
# the LAN bearer token into the fallback-specific variable before clearing
# llm_token for localhost. Shell assignments are expanded left-to-right here:
# POKEPILOT_LLM_FALLBACK_TOKEN sees the sourced llm_token before llm_token is
# cleared later in the same command environment.
run-llm-auto:
	$(require-rom)
	$(load_env) \
	POKEPILOT_LLM_FALLBACK_URL="$${POKEPILOT_LLM_URL:-$(AUTO_LLM_FALLBACK_URL)}" \
	POKEPILOT_LLM_FALLBACK_MODEL="$${POKEPILOT_LLM_MODEL:-$(AUTO_LLM_FALLBACK_MODEL)}" \
	POKEPILOT_LLM_FALLBACK_TOKEN="$$llm_token" \
	POKEPILOT_LLM_FALLBACK_TIMEOUT=$(AUTO_LLM_FALLBACK_TIMEOUT) \
	POKEPILOT_LLM_URL=$(AUTO_LLM_URL) \
	POKEPILOT_LLM_MODEL=$(AUTO_LLM_MODEL) \
	POKEPILOT_LLM_NO_THINK=$(AUTO_LLM_NO_THINK) \
	POKEPILOT_LLM_MAX_TOKENS=$(AUTO_LLM_MAX_TOKENS) \
	POKEPILOT_LLM_TIMEOUT=$(AUTO_LLM_TIMEOUT) \
	llm_token= \
	go run ./cmd/pokepilot -planner llm -fps 0 $(ARGS)

# Build one bounded model-facing packet from a run id. The command performs
# source localization itself and, when a structured failure repro plus Red ROM
# are available, replays it deterministically through the current checkout.
# ATTEMPT/KEY scope the evidence to the attempt that failed; both default to
# the run's latest attempt.
debug:
	@test -n "$(RUN)" || { echo "usage: make debug RUN=run-... [DEBUG_MODE=tiny|normal|deep] [ATTEMPT=N] [KEY=triage-key]"; exit 2; }
	$(load_env) \
	go run ./cmd/pokedebug -run "$(RUN)" -mode "$(DEBUG_MODE)" $(if $(ATTEMPT),-attempt "$(ATTEMPT)") $(if $(KEY),-key "$(KEY)") $(ARGS)

test:
	go test ./... $(ARGS)

# verify is deliberately ROM-free and mirrors CI. Emulator-backed fixture
# tests skip when POKEMON_RED_ROM is empty; -short skips long journey tests.
verify: fmt-check vet test-short test-race

fmt-check:
	@files="$$(gofmt -l .)"; \
	if [ -n "$$files" ]; then \
		echo "gofmt required:"; \
		echo "$$files"; \
		gofmt -d $$files; \
		exit 1; \
	fi

vet:
	POKEMON_RED_ROM= go vet ./...

test-short:
	POKEMON_RED_ROM= go test -short -count=1 ./... $(ARGS)

test-race:
	POKEMON_RED_ROM= go test -race -short -count=1 ./... $(ARGS)

# Focused ROM-free loops for the areas changed most often. These are not
# substitutes for verify; they keep edit/test cycles short before the full gate.
test-farm:
	POKEMON_RED_ROM= go test -short -count=1 ./farm ./artifactstore ./cmd/pokewall ./cmd/pokeissues ./cmd/pokeui ./cmd/pokereplay ./deploy $(ARGS)

test-agent:
	POKEMON_RED_ROM= go test -short -count=1 ./agent ./cmd/pokepilot $(ARGS)

test-state:
	POKEMON_RED_ROM= go test -short -count=1 ./red/state ./red/sym ./world $(ARGS)

# Local Yellow qualification. These targets are intentionally not part of
# normal CI because the copyrighted ROM is supplied by the operator.
test-yellow-rom:
	$(require-yellow-rom)
	POKEMON_RED_ROM= POKEMON_YELLOW_ROM="$(POKEMON_YELLOW_ROM)" go test -count=1 ./yellow/... $(ARGS)
	POKEMON_RED_ROM= POKEMON_YELLOW_ROM="$(POKEMON_YELLOW_ROM)" go test -count=1 -run Yellow ./agent $(ARGS)

verify-yellow-rom:
	$(require-yellow-rom)
	go run ./cmd/worldverify -game yellow -rom "$(POKEMON_YELLOW_ROM)" -max-exhaustive-capabilities 16
	$(MAKE) test-yellow-rom POKEMON_YELLOW_ROM="$(POKEMON_YELLOW_ROM)" ARGS='$(ARGS)'

# GomeBoy is pinned to the maintained GitHub fork in go.mod, so the Docker
# build needs only this repository as its build context.
farm-image:
	docker buildx build --load -t $(FARM_IMAGE) -f deploy/Dockerfile .

farm-up: farm-image
	$(require-rom)
	mkdir -p "$(FARM_STATE_DIR)"
	# A leftover standalone pokefarm_ui (from before it joined the stack)
	# would hold the host port and block the new task.
	docker rm -f pokefarm_ui >/dev/null 2>&1 || true
	# Secrets and optional endpoint overrides live in .env or
	# ~/.config/pokepilot/env. docker stack deploy interpolates them here;
	# the GitHub token is passed only to the issue-adapter service.
	$(load_env) \
	red_sha="${POKEPILOT_ROM_SHA256_POKEMON_RED:-$(sha256sum "$(POKEMON_RED_ROM)" | awk '{print $1}')}" ; \
	blue_rom="$(POKEPILOT_ROM_DIR)/pokemon_blue.gb" ; \
	blue_sha="${POKEPILOT_ROM_SHA256_POKEMON_BLUE:-}" ; \
	if [ -z "$blue_sha" ] && [ -f "$blue_rom" ]; then blue_sha="$(sha256sum "$blue_rom" | awk '{print $1}')" ; fi ; \
	prompt_sha="${POKEPILOT_PROMPT_SHA256:-$(git ls-files agent cmd/pokepilot | grep '\.go$' | grep -v '_test\.go$' | xargs sha256sum | sha256sum | awk '{print $1}')}" ; \
	POKEPILOT_ROM_SHA256_POKEMON_RED="$red_sha" \
	POKEPILOT_ROM_SHA256_POKEMON_BLUE="$blue_sha" \
	POKEPILOT_PROMPT_SHA256="$prompt_sha" \
	docker stack deploy --resolve-image never -c deploy/farm.yml pokefarm
	# The image tag does not change between builds, so the service spec is
	# identical and Docker would not roll healthy tasks — a rebuilt image
	# would never land. Force every PokePilot-image service that owns code.
	docker service update --force --detach pokefarm_wall
	docker service update --force --detach pokefarm_issues
	docker service update --force --detach pokefarm_runner
	docker service update --force --detach pokefarm_replay
	docker service update --force --detach pokefarm_ui
	docker service update --force --detach pokefarm_linkbroker
	docker service update --force --detach pokefarm_virtualtrader
	@echo "pokefarm UI: http://localhost:$(FARM_WALL_PORT)/"

# Publish every recognised cartridge in $(POKEPILOT_ROM_DIR) to the S3 ROM
# store (roms/<game id>) that runners on other nodes fetch from. Games already
# in the store are left alone, so this is safe to rerun after adding a ROM.
roms-upload:
	$(load_env) \
	go run ./cmd/romupload -dir "$(POKEPILOT_ROM_DIR)"

farm-down:
	docker rm -f pokefarm_ui >/dev/null 2>&1 || true
	docker stack rm pokefarm

# Swarm fixer (deploy/fixer.yml): FIXER_REPLICAS containers running the
# qwagent ladder in parallel. Secrets and POKEPILOT_QWEN_URL come from
# ~/.config/pokepilot/env. Stop the desktop qwtriage timer first, or three
# fixers compete for qwen and the same keys.
FIXER_IMAGE ?= pokepilot-fixer:local
FIXER_STATE_DIR ?= $(HOME)/.local/share/pokepilot/fixer-swarm
# Cartridges only: POKEPILOT_ROM_DIR can be ~/.config/pokepilot, which also
# holds the env file, and the agent must not see those secrets.
FIXER_ROM_DIR ?= $(HOME)/.local/share/pokepilot/fixer-roms
fixer-image:
	docker buildx build --load -t $(FIXER_IMAGE) -f deploy/fixer.Dockerfile .

fixer-up: fixer-image
	$(require-rom)
	mkdir -p "$(FIXER_STATE_DIR)" "$(FIXER_ROM_DIR)"
	cp -u "$(POKEPILOT_ROM_DIR)"/pokemon_*.gb* "$(FIXER_ROM_DIR)/"
	$(load_env) FIXER_IMAGE=$(FIXER_IMAGE) FIXER_STATE_DIR="$(FIXER_STATE_DIR)" \
		POKEPILOT_ROM_DIR="$(FIXER_ROM_DIR)" \
		docker stack deploy -c deploy/fixer.yml pokefixer

fixer-down:
	docker stack rm pokefixer

# Opt-in local qwagent loop against MCP pokepilot_get_triage. Installs user
# systemd units and zsh helpers; does not enable the timer.
qwagent-triage-install:
	mkdir -p "$(HOME)/.config/systemd/user"
	sed 's|@@POKEPILOT_ORIGIN@@|$(shell git remote get-url origin)|g' deploy/qwagent-triage.service.in \
		> "$(HOME)/.config/systemd/user/qwagent-triage.service"
	cp deploy/qwagent-triage.timer "$(HOME)/.config/systemd/user/qwagent-triage.timer"
	cp deploy/farm-watch.service.in "$(HOME)/.config/systemd/user/farm-watch.service"
	cp deploy/farm-watch.timer "$(HOME)/.config/systemd/user/farm-watch.timer"
	systemctl --user daemon-reload
	@marker='# PokePilot qwagent-triage helpers'; \
	if [ -f "$(HOME)/.zshrc" ] && ! grep -q "$$marker" "$(HOME)/.zshrc"; then \
		printf '\n%s\nsource %s/deploy/qwagent-triage.zsh\n' "$$marker" "$(CURDIR)" >> "$(HOME)/.zshrc"; \
		echo "appended source line to ~/.zshrc (open a new shell)"; \
	fi
	@echo "timer installed but not enabled. qwtriage-on to start, qwtriage-off to stop."
	@echo "watchdog: set TELEGRAM_BOT_TOKEN, TELEGRAM_CHAT_ID, POKEPILOT_WALL_URL in ~/.config/pokepilot/env, then systemctl --user enable --now farm-watch.timer"
	@echo "ladder mode: qwen first, then Cursor ('agent login'), then Claude Code ('claude auth login')."
