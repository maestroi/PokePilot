package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/maestroi/pokepilot/emu"
	gen1trade "github.com/maestroi/pokepilot/gen1/trade"
	"github.com/maestroi/pokepilot/skill"
)

const (
	dexVirtualTradebackIntent = "dex-virtual-tradeback"
	dexVirtualVersionIntent   = "dex-virtual-version-assisted"
	dexVirtualPokedexIntent   = "dex-virtual-pokedex"
	linkBrokerEnv             = "POKEPILOT_LINK_BROKER"
	virtualTradeRunIDEnv      = "POKEPILOT_RUN_ID"
	virtualTradeOfferLimit    = 4
	virtualTradeSetupTimeout  = 10 * time.Second
	virtualTradeLinkTimeout   = 10 * time.Second
)

// appendDexVirtualTradeObjectives turns the catalog's explicitly unavailable
// species into executable objectives only when the farm advertises a virtual
// trader. Event-only species stay unavailable: synthetic Mew is intentionally
// outside this branch. Trade evolutions require the exact base in the party;
// version/choice gaps require a repeatably obtainable donor so completing the
// Dex never destroys an irreplaceable field or one-off Pokemon.
func appendDexVirtualTradeObjectives(obs Observation, out []Objective) []Objective {
	if obs.Services == nil || !obs.Services.VirtualTrader || len(obs.Dex.Unavailable) == 0 {
		return out
	}
	already := map[SpeciesID]bool{}
	for _, objective := range out {
		if objective.Kind == KindCatch && objective.Species != "" {
			already[objective.Species] = true
		}
	}

	added := 0
	for _, entry := range obs.Dex.Unavailable {
		if added >= virtualTradeOfferLimit || entry.Species == "" || already[entry.Species] {
			continue
		}
		if entry.Unavailable == UnavailableEventOnly {
			continue
		}

		if entry.Unavailable == UnavailableTradeEvolution {
			for _, src := range entry.Sources {
				if src.Kind != AcquireTradeEvo || src.From == "" {
					continue
				}
				slot, _, ok := partySpeciesSlot(obs.Party, src.From)
				if !ok || safeTradebackPlaceholder(obs) == "" {
					continue
				}
				out = append(out, Objective{
					Kind:    KindCatch,
					Species: entry.Species,
					Slot:    slot,
					Intent:  dexVirtualTradebackIntent,
					Note: fmt.Sprintf("(virtual tradeback: send %s and receive the exact same Pokemon back so the ROM evolves it into %s; provenance=tradeback)",
						strings.ToUpper(string(src.From)), strings.ToUpper(string(entry.Species))),
				})
				already[entry.Species] = true
				added++
				break
			}
			continue
		}

		slot, ok := virtualTradeDonorSlot(obs)
		if !ok {
			continue
		}
		intent, policy := "", ""
		switch {
		case entry.Unavailable == UnavailableNoLocalSource:
			intent, policy = dexVirtualVersionIntent, "version-assisted"
		case strings.HasPrefix(entry.Unavailable, UnavailableForfeited+":"):
			intent, policy = dexVirtualPokedexIntent, "pokedex"
		default:
			continue
		}
		out = append(out, Objective{
			Kind:    KindCatch,
			Species: entry.Species,
			Slot:    slot,
			Intent:  intent,
			Note: fmt.Sprintf("(virtual %s trade: synthetic remote %s through the real Cable Club; donor party slot %d is replaceable; provenance=%s)",
				policy, strings.ToUpper(string(entry.Species)), slot, policy),
		})
		already[entry.Species] = true
		added++
	}
	return out
}

func virtualTradeDonorSlot(obs Observation) (int, bool) {
	if len(obs.Party) <= 1 {
		return 0, false
	}
	repeatable := map[SpeciesID]bool{}
	for _, entry := range obs.Dex.Owned {
		for _, src := range entry.Sources {
			switch src.Kind {
			case AcquireWildGrass, AcquireWildWater, AcquireFishing:
				repeatable[entry.Species] = true
			}
		}
	}
	tradeBases := tradeEvolutionBases(obs)
	for slot := len(obs.Party) - 1; slot >= 0; slot-- {
		mon := obs.Party[slot]
		if mon.Species == "" || !repeatable[mon.Species] || tradeBases[mon.Species] || partySlotCarriesFieldMove(obs, slot) {
			continue
		}
		return slot, true
	}
	return 0, false
}

func tradeEvolutionBases(obs Observation) map[SpeciesID]bool {
	out := map[SpeciesID]bool{}
	collect := func(entries []DexEntry) {
		for _, entry := range entries {
			for _, src := range entry.Sources {
				if src.Kind == AcquireTradeEvo && src.From != "" {
					out[src.From] = true
				}
		}
	}
	collect(obs.Dex.Owned)
	collect(obs.Dex.Targets)
	collect(obs.Dex.Unavailable)
	return out
}

func safeTradebackPlaceholder(obs Observation) SpeciesID {
	blocked := tradeEvolutionBases(obs)
	for _, entry := range obs.Dex.Owned {
		if entry.Species == "" || blocked[entry.Species] {
			continue
		}
		if _, ok := redSpeciesID(entry.Species); ok {
			return entry.Species
		}
	}
	for _, species := range obs.PokedexOwned {
		if species == "" || blocked[species] {
			continue
		}
		if _, ok := redSpeciesID(species); ok {
			return species
		}
	}
	return ""
}

func virtualTradeEvolutionSource(obs Observation, target SpeciesID) SpeciesID {
	for _, entry := range obs.Dex.Unavailable {
		if entry.Species != target || entry.Unavailable != UnavailableTradeEvolution {
			continue
		}
		for _, src := range entry.Sources {
			if src.Kind == AcquireTradeEvo && src.From != "" {
				return src.From
			}
		}
	}
	return ""
}

func executeDexVirtualTrade(m *emu.Emu, romData []byte, o Objective, result ObjectiveResult) (ObjectiveResult, error) {
	serviceURL := strings.TrimSpace(os.Getenv(virtualTraderURLEnv))
	broker := strings.TrimSpace(os.Getenv(linkBrokerEnv))
	if serviceURL == "" || broker == "" {
		result.Outcome = OutcomeBlocked
		return result, fmt.Errorf("agent: %s: virtual trader is not fully configured", o)
	}
	initial, err := ObserveChecked(m, romData)
	if err != nil {
		return result, fmt.Errorf("agent: %s: observe virtual trade precondition: %w", o, err)
	}
	if o.Slot < 0 || o.Slot >= len(initial.Party) {
		result.Outcome = OutcomeBlocked
		return result, fmt.Errorf("agent: %s: donor slot %d absent from party of %d", o, o.Slot, len(initial.Party))
	}

	policy := ""
	offerSpecies := o.Species
	tradeback := false
	switch o.Intent {
	case dexVirtualTradebackIntent:
		policy = "tradeback"
		tradeback = true
		source := virtualTradeEvolutionSource(initial, o.Species)
		if source == "" || initial.Party[o.Slot].Species != source {
			result.Outcome = OutcomeBlocked
			return result, fmt.Errorf("agent: %s: tradeback source is no longer in party slot %d", o, o.Slot)
		}
		offerSpecies = safeTradebackPlaceholder(initial)
		if offerSpecies == "" {
			result.Outcome = OutcomeBlocked
			return result, fmt.Errorf("agent: %s: no already-owned non-trade-evolving placeholder is available", o)
		}
	case dexVirtualVersionIntent:
		policy = "version-assisted"
	case dexVirtualPokedexIntent:
		policy = "pokedex"
	default:
		return result, fmt.Errorf("agent: %s: unsupported virtual trade intent %q", o, o.Intent)
	}

	session, err := newVirtualTradeSession(o.Species)
	if err != nil {
		return result, fmt.Errorf("agent: %s: allocate trade session: %w", o, err)
	}
	level := int(initial.Party[o.Slot].Level)
	if level < 1 {
		level = 1
	}
	client := gen1trade.ServiceClient{BaseURL: serviceURL}
	setupCtx, cancelSetup := context.WithTimeout(context.Background(), virtualTradeSetupTimeout)
	defer cancelSetup()
	if _, err := client.StartSession(setupCtx, gen1trade.SessionRequest{
		Session: session,
		RunID:   strings.TrimSpace(os.Getenv(virtualTradeRunIDEnv)),
		Game:    string(initial.GameID),
		Policy:  policy,
		Species: string(offerSpecies),
		Level:   level,
	}); err != nil {
		result.Outcome = OutcomeBlocked
		return result, fmt.Errorf("agent: %s: start virtual trader: %w", o, err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cleanupCancel()
		_ = client.DeleteSession(cleanupCtx, session)
	}()
	if _, err := client.WaitReady(setupCtx, session); err != nil {
		result.Outcome = OutcomeBlocked
		return result, fmt.Errorf("agent: %s: wait for virtual trader: %w", o, err)
	}

	metadata := map[string]string{
		"game":              string(initial.GameID),
		"trade_policy":      policy,
		"requested_species": string(o.Species),
		"status":            "active_trade",
	}
	link, err := m.ConnectBrokerLink(broker, session, "emulator-"+session, "emulator", metadata, virtualTradeLinkTimeout)
	if err != nil {
		result.Outcome = OutcomeBlocked
		return result, fmt.Errorf("agent: %s: attach emulator link: %w", o, err)
	}
	defer link.Close()

	trade, err := skill.VirtualTrade(m, romData, o.Slot, tradeback, skill.StatAwareMove(romData))
	result.Travel = travelEvidenceFromRed(trade.Travel)
	// Provenance is durable in two existing records without widening the
	// ObjectiveResult schema: the objective Intent/Note names the policy and
	// the trader service emits structured session/run trade_event records.
	if err != nil {
		return result, fmt.Errorf("agent: %s: %w", o, err)
	}
	return result, nil
}

func newVirtualTradeSession(species SpeciesID) (string, error) {
	var entropy [6]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	name := strings.ToLower(strings.TrimSpace(string(species)))
	name = strings.ReplaceAll(name, " ", "-")
	if name == "" {
		name = "pokemon"
	}
	return "vt-" + name + "-" + hex.EncodeToString(entropy[:]), nil
}
