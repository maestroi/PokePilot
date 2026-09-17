package skill

import "testing"

func TestPokemonNicknamePromptMatchesAskNameOnly(t *testing.T) {
	mem := newFakeRAM()
	openChoice(mem, 8, 12, "Do you want to give a nickname to CHARMANDER")
	if !pokemonNicknamePrompt(mem, redWram()) {
		t.Fatal("AskName two-option prompt was not recognized as a Pokemon nickname prompt")
	}

	other := newFakeRAM()
	openChoice(other, 8, 12, "Would you like me to heal your POKEMON")
	if pokemonNicknamePrompt(other, redWram()) {
		t.Fatal("ordinary story yes/no prompt was mistaken for a Pokemon nickname prompt")
	}

	plain := newFakeRAM()
	openTextBox(plain, "give a nickname")
	if pokemonNicknamePrompt(plain, redWram()) {
		t.Fatal("ordinary text containing nickname words was treated as a two-option nickname prompt")
	}
}
