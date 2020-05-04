package v1alpha3

import "sigs.k8s.io/controller-runtime/pkg/client"

// Default the resource
func (s *Service) Default(client client.Client) {
	s.GetPredictor().Default()
}
