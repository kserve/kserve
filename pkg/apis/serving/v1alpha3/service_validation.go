package v1alpha3

import "sigs.k8s.io/controller-runtime/pkg/client"

// Validate the resource
func (s *Service) Validate(client client.Client) {
	s.GetFramework().Validate()
}
