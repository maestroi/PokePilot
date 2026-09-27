package profile

import (
	"testing"

	"github.com/maestroi/pokepilot/gs/sym"
)

func setGSEvent(mem fakeGSReader, event uint16) {
	mem[sym.EventFlags+event/8] |= byte(1 << uint(event%8))
}

func TestEarlyJohtoProgressionProjectsDurableFlags(t *testing.T) {
	mem := fakeGSReader{}
	setGSEvent(mem, eventGotPokemonFromElm)
	setGSEvent(mem, eventGotMysteryEggMrPokemon)
	mem[sym.StatusFlags] = statusFlagsPokedexMask
	mem[sym.MrPokemonsHouseSceneID] = sceneMrPokemonsHouseNoop
	mem[sym.CherrygroveCitySceneID] = sceneCherrygroveNoop
	mem[sym.ElmsLabSceneID] = sceneElmsLabNoop
	setGSEvent(mem, eventGaveMysteryEggToElm)

	story := projectEarlyStory(mem)
	for _, id := range []string{
		string(ProgressStarterReceived),
		string(ProgressMysteryEggReceived),
		string(ProgressPokedexAcquired),
		string(ProgressCherrygroveRivalResolved),
		string(ProgressRivalNamed),
		string(ProgressMysteryEggReturned),
	} {
		if !story.Has(id) {
			t.Fatalf("story missing completed %q: %+v", id, story)
		}
	}
}

func TestEarlyJohtoProgressionDoesNotConfuseFreshNoopScenesWithCompletion(t *testing.T) {
	mem := fakeGSReader{
		sym.CherrygroveCitySceneID: sceneCherrygroveNoop,
		sym.ElmsLabSceneID:         sceneElmsLabNoop,
	}
	story := projectEarlyStory(mem)
	if story.Has(ProgressCherrygroveRivalResolved) {
		t.Fatal("fresh Cherrygrove noop scene falsely resolved rival")
	}
	if story.Has(ProgressRivalNamed) {
		t.Fatal("fresh Elm noop scene falsely resolved rival naming")
	}
}

func TestEarlyJohtoProgressionRequiresMrPokemonScriptToFinishBeforeRivalResolution(t *testing.T) {
	mem := fakeGSReader{
		sym.StatusFlags:            statusFlagsPokedexMask,
		sym.CherrygroveCitySceneID: sceneCherrygroveNoop,
	}
	setGSEvent(mem, eventGotMysteryEggMrPokemon)

	story := projectEarlyStory(mem)
	if !story.Has(ProgressMysteryEggReceived) || !story.Has(ProgressPokedexAcquired) {
		t.Fatalf("egg/dex not projected: %+v", story)
	}
	if story.Has(ProgressCherrygroveRivalResolved) {
		t.Fatal("rival resolved before Mr. Pokemon scene reached its durable noop state")
	}

	mem[sym.MrPokemonsHouseSceneID] = sceneMrPokemonsHouseNoop
	mem[sym.CherrygroveCitySceneID] = 1
	story = projectEarlyStory(mem)
	if story.Has(ProgressCherrygroveRivalResolved) {
		t.Fatal("rival resolved while Cherrygrove rival scene is still armed")
	}

	mem[sym.CherrygroveCitySceneID] = sceneCherrygroveNoop
	story = projectEarlyStory(mem)
	if !story.Has(ProgressCherrygroveRivalResolved) {
		t.Fatal("rival did not resolve after Mr. Pokemon completion and Cherrygrove scene reset")
	}
}

func TestGSObservationIncludesMysteryEggReturnVerifier(t *testing.T) {
	mem := fakeGSReader{
		sym.MapGroup:       24,
		sym.MapNumber:      5,
		sym.MapWidth:       5,
		sym.MapHeight:      6,
		sym.MapStatus:      gen2MapStatusHandle,
		sym.MapEventStatus: gen2MapEventsOn,
	}
	setGSEvent(mem, eventGaveMysteryEggToElm)

	obs, err := NewGold().DecodeObservation(mem, nil)
	if err != nil {
		t.Fatalf("DecodeObservation: %v", err)
	}
	fact, ok := obs.Story.Lookup(ProgressMysteryEggReturned)
	if !ok || !fact.Complete {
		t.Fatalf("mystery egg return fact = %+v, ok=%v", fact, ok)
	}
}
