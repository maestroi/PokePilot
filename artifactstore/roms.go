package artifactstore

// ROMObjectKey is where the cartridge for a game ID lives in the bucket.
// cmd/romupload writes it and farm runners read it, so the layout has one owner.
func ROMObjectKey(gameID string) string { return "roms/" + gameID }
