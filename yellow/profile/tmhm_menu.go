package profile

// TMHMPartyMenuMarker returns Yellow's party-select wording for teaching a
// TM/HM. Yellow renamed the Red/Blue "Use TM on which #MON?" prompt to "Teach
// to which #MON?"; the shared Gen-I engine matches this profile fact rather
// than a Red/Blue phrase.
func (*Profile) TMHMPartyMenuMarker() string { return "Teach to which" }
