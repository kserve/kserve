package v1alpha1

//This has been extracted as v1 does not utilise criticality, but v1alpha2 does.

// ModelCriticality expresses the relative importance of serving a model.
// This is used by our scheduler integration and stays in KServe domain.
type ModelCriticality string

const (
	// Critical traffic must not be dropped when possible.
	Critical ModelCriticality = "Critical"

	// Sheddable traffic may be dropped under load.
	Sheddable ModelCriticality = "Sheddable"
)
