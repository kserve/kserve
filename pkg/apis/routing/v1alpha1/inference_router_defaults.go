package v1alpha1

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Default the resource
func (i *v1alpha1.InferenceRouter) Default(client client.Client) {
}
