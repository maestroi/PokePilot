package agent

// ObjectiveCatalogProvider is implemented by a concrete game adapter. It is
// intentionally separate from ProgressionPlanner so games can expose world
// opportunities without coupling catalog discovery to story sequencing.
type ObjectiveCatalogProvider interface {
	ObjectiveCatalog(Observation) ObjectiveCatalog
}
