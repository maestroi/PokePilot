package agent

// This file intentionally contains no concrete game imports.
//
// Generic objective execution lives in game_adapter.go and is bound through
// ObjectiveGameAdapter. Red/Gen-I dispatch is isolated in red_execute_owned.go;
// keeping this file concrete-free makes the runtime boundary obvious and gives
// architecture tests a stable generic execution seam to protect.
