package v1beta1

import "sigs.k8s.io/controller-runtime/pkg/client"

// Default the resource
func (i *InferenceService) Default(client client.Client) {
	i.GetPredictor().Default()
}
