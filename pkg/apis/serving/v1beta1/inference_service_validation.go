package v1beta1

import "sigs.k8s.io/controller-runtime/pkg/client"

// Validate the resource
func (i *InferenceService) Validate(client client.Client) {
	//i.GetPredictor().Validate()
}
