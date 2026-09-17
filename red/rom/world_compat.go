package rom

// These tiny accessors let generic world tests and migration-era callers pass
// the existing Red Connection type through the portable connection boundary
// without making world import red/rom.
func (c Connection) WorldDirection() uint8 { return c.Dir }
func (c Connection) WorldMapID() uint8     { return c.MapID }
func (c Connection) WorldOffset() int8     { return c.Offset }
