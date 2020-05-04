package v1alpha3

// SplitterSpec defines a simple weighted traffic split interface
type SplitterSpec struct {
	// Weights defines the weights of the routes
	Weights []*WeightsSpec `json:"weights,omitempty"`
}

// WeightsSpec defines a simple weighted traffic split interface
type WeightsSpec struct {
	// The name for the route
	Name string `json:"name"`
	// The weight to send traffic to this route.
	Weight int `json:"weight"`
}
