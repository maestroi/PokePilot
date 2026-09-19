package skill

import "sort"

// InteractionPlaceNames returns every interaction-owned destination that Place
// accepts but PlaceNames deliberately does not expose as a standalone journey.
// Route availability audits use this complete set so a compound/gift/catch
// objective cannot be offered merely because its private destination was
// omitted from capability-aware reachability.
func InteractionPlaceNames() []string {
	names := make([]string, 0, len(interactionPlaces))
	for name := range interactionPlaces {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
