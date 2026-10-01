package profile

// TMHMPartyMenuMarker returns Red's party-select wording for teaching a TM/HM.
// The phrase is a Red/Blue fact; Yellow overrides it with its own wording.
func (*Profile) TMHMPartyMenuMarker() string { return "Use TM" }
