# Pokemon Red ROM. Override: make run POKEMON_RED_ROM=/path/to/red.gb
POKEMON_RED_ROM ?= $(CURDIR)/roms/pokemon_red.gb
export POKEMON_RED_ROM

# Extra flags, e.g. make run ARGS='-goto "pallet town"'
ARGS ?=

.PHONY: run run-60 run-0 run-llm test pokered

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

# The pokered disassembly, as `pokered/` at the repo root. See docs/POKERED.md.
POKERED ?= $(HOME)/.cache/pokered

pokered:
	@test -d "$(POKERED)" || { \
		echo "pokered checkout not found: $(POKERED)"; \
		echo "git clone https://github.com/pret/pokered $(POKERED)"; \
		exit 1; \
	}
	@ln -sfn "$(POKERED)" pokered && echo "pokered -> $(POKERED)"
