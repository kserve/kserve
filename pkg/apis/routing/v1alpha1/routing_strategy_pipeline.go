package v1alpha1

// PipelineSpec defines the inference graph with specified dependency on the routing steps
type PipelineSpec struct {
	// Retries represents how many times the request should be retried in case of failure, by default 0.
	// +optional
	Retries int `json:"retries,omitempty"`
	// Pipeline timeout seconds specifies how long it should wait for all the steps to finish on the pipeline.
	TimeoutSeconds int `json:"timeoutSeconds,omitempty"`
}
