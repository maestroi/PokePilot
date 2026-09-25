package skill

func init() {
	// Indigo's lobby provides the same nurse/blackout-checkpoint service as an
	// ordinary Pokemon Center, but its ROM map name is INDIGO_PLATEAU_LOBBY.
	// Give that service a first-class semantic place so recovery/planning can
	// target it just like every other activated Center.
	places["indigo plateau pokemon center"] = MapDestination(indigoPlateauLobbyMap)
}
