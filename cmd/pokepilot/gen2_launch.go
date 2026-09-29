package main

func isGen2GameID(gameID string) bool {
	return gameID == "pokemon-gold" || gameID == "pokemon-silver"
}

func isGen2StarterRequest(starter string) bool {
	switch starter {
	case "", "chikorita", "cyndaquil", "totodile":
		return true
	default:
		return false
	}
}
