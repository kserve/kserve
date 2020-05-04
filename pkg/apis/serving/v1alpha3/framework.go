package v1alpha3

import (
	v1 "k8s.io/api/core/v1"
)

// Framework is an abstraction over machine learning server frameworks
type Framework interface {
	GetContainers() []v1.Container
	Validate() error
	Default()
}

// GetFramework returns the framework for the Service
func (s *Service) GetFramework() Framework {
	for _, f := range []Framework{
		s.Spec.Predictor.KFServer,
		s.Spec.Predictor.ONNXRuntime,
		s.Spec.Predictor.TFServing,
		s.Spec.Predictor.TorchServe,
		s.Spec.Predictor.Triton,
	} {
		if f != nil {
			return f
		}
	}
	return s.Spec.Predictor.CustomFramework
}
