package sym

// WhichPokemon is wWhichPokemon: the party slot currently being processed by
// LearnMove and the battle experience loop. It is deliberately distinct from
// PlayerMonNumber: a Pokémon can gain experience, level up, and be asked to
// learn a move after another party member has become the active battler.
// The Red WRAM symbol map places it at 0xCF92 (with ListMenuID at 0xCF94).
const WhichPokemon uint16 = 0xCF92
