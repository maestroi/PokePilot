package agent

import (
	"fmt"
	"sort"
	"strings"
)

const (
	DexReportOwned       = "owned"
	DexReportRemaining   = "remaining"
	DexReportUnavailable = "unavailable"
)

// DexSpeciesReport is one species row in the final completion report. Method
// is populated only when a verified runtime objective identifies how the
// species was acquired; catalog sources remain available separately on Entry.
type DexSpeciesReport struct {
	Species SpeciesID `json:"species"`
	Dex     uint8     `json:"dex"`
	Status  string    `json:"status"`
	Reason  string    `json:"reason,omitempty"`
	Method  string    `json:"method,omitempty"`
	Entry   DexEntry  `json:"entry"`
}

// DexCompletionReport is reproducible from the final observation plus the
// structured objective outcomes. It deliberately uses the save/ROM-derived Dex
// catalog as the source of truth rather than assuming a fixed 151 denominator.
type DexCompletionReport struct {
	Complete         bool               `json:"complete"`
	Owned            int                `json:"owned"`
	Obtainable       int                `json:"obtainable"`
	Remaining        int                `json:"remaining"`
	Unavailable      int                `json:"unavailable"`
	Summary          string             `json:"summary"`
	Species          []DexSpeciesReport `json:"species"`
	AcquisitionKnown int                `json:"acquisition_known"`
}

// DexReport returns the final machine-readable/human-readable completion
// report for a run. It is safe to call for a non-Dex run; an empty catalog
// yields a report that is explicitly incomplete rather than a false success.
func (r Result) DexReport() DexCompletionReport {
	return BuildDexCompletionReport(r.Final, r.Outcomes)
}

// BuildDexCompletionReport classifies every catalog entry and attaches the
// acquisition method when a completed objective provides positive evidence.
func BuildDexCompletionReport(obs Observation, outcomes []ObjectiveResult) DexCompletionReport {
	methods := dexAcquisitionMethods(obs, outcomes)
	report := DexCompletionReport{
		Owned:       len(obs.Dex.Owned),
		Remaining:   len(obs.Dex.Targets),
		Unavailable: len(obs.Dex.Unavailable),
		Species:     make([]DexSpeciesReport, 0, len(obs.Dex.Owned)+len(obs.Dex.Targets)+len(obs.Dex.Unavailable)),
	}
	report.Obtainable = report.Owned + report.Remaining
	report.Complete = report.Obtainable > 0 && report.Remaining == 0

	appendRows := func(entries []DexEntry, status string) {
		for _, entry := range entries {
			row := DexSpeciesReport{
				Species: entry.Species,
				Dex:     entry.Dex,
				Status:  status,
				Entry:   entry,
			}
			if status == DexReportUnavailable {
				row.Reason = entry.Unavailable
			}
			if method := methods[entry.Species]; method != "" {
				row.Method = method
				report.AcquisitionKnown++
			}
			report.Species = append(report.Species, row)
		}
	}
	appendRows(obs.Dex.Owned, DexReportOwned)
	appendRows(obs.Dex.Targets, DexReportRemaining)
	appendRows(obs.Dex.Unavailable, DexReportUnavailable)
	sort.Slice(report.Species, func(i, j int) bool {
		if report.Species[i].Dex != report.Species[j].Dex {
			return report.Species[i].Dex < report.Species[j].Dex
		}
		return report.Species[i].Species < report.Species[j].Species
	})

	if report.Obtainable == 0 {
		report.Summary = fmt.Sprintf("Pokédex report unavailable: no obtainable catalog entries; %d unavailable", report.Unavailable)
	} else if report.Complete {
		report.Summary = fmt.Sprintf("Pokédex complete: %d/%d obtainable owned; %d unavailable", report.Owned, report.Obtainable, report.Unavailable)
	} else {
		report.Summary = fmt.Sprintf("Pokédex incomplete: %d/%d obtainable owned; %d remaining; %d unavailable", report.Owned, report.Obtainable, report.Remaining, report.Unavailable)
	}
	return report
}

func dexAcquisitionMethods(final Observation, outcomes []ObjectiveResult) map[SpeciesID]string {
	methods := map[SpeciesID]string{}
	owned := map[SpeciesID]DexEntry{}
	for _, entry := range final.Dex.Owned {
		owned[entry.Species] = entry
	}

	for _, result := range outcomes {
		if result.Outcome != OutcomeCompleted {
			continue
		}
		o := result.Objective
		switch o.Kind {
		case KindCatch:
			if o.Species != "" && owned[o.Species].Species != "" {
				methods[o.Species] = dexCatchMethod(o.Intent)
			}
		case KindStarter:
			id := SpeciesID(starterName(o.Starter))
			if owned[id].Species != "" {
				methods[id] = AcquireStarter
			}
		case KindTrain:
			if o.Intent != "dex-evolution" || o.Species == "" {
				continue
			}
			for id, entry := range owned {
				for _, src := range entry.Sources {
					if src.Kind == AcquireLevelEvo && src.From == o.Species && (src.Level == 0 || src.Level <= uint8(o.Level)) {
						methods[id] = AcquireLevelEvo
					}
				}
			}
		case KindUseItem:
			if o.Intent != "dex-evolution" {
				continue
			}
			// The item-evolution executor verifies the final party slot. Use that
			// semantic target when it is available, then verify its catalog source.
			if o.Slot < 0 || o.Slot >= len(result.Final.Party) {
				continue
			}
			id := result.Final.Party[o.Slot].Species
			entry, ok := owned[id]
			if !ok {
				continue
			}
			for _, src := range entry.Sources {
				if src.Kind == AcquireItemEvo && strings.EqualFold(string(src.Item), string(o.Item)) {
					methods[id] = AcquireItemEvo
					break
				}
			}
		}
	}
	return methods
}

func dexCatchMethod(intent string) string {
	switch intent {
	case dexFishingIntent:
		return AcquireFishing
	case dexWaterIntent:
		return AcquireWildWater
	case dexSafariIntent:
		return "safari"
	case dexGiftIntent:
		return AcquireGift
	case dexTradeIntent:
		return AcquireInGameTrade
	case dexFossilIntent:
		return AcquireFossil
	case dexGameCornerIntent:
		return "game_corner_prize"
	case dexStaticIntent:
		return AcquireStatic
	default:
		return AcquireWildGrass
	}
}
