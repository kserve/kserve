package v1alpha3

import "sigs.k8s.io/controller-runtime/pkg/client"

// Validate the resource
func (i *InferenceService) Validate(client client.Client) {
	i.GetPredictor().Validate()
}
