package agent

// appendObjectiveNote is shared by generic recovery and provider annotations.
func appendObjectiveNote(o Objective, note string) Objective {
	if note == "" {
		return o
	}
	if o.Note == "" {
		o.Note = note
	} else {
		o.Note += " " + note
	}
	return o
}
