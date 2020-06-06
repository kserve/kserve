package components

import "github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"

// Component can be reconciled to create underlying resources for an InferenceService
type Component interface {
	Reconcile(isvc *v1beta1.InferenceService) error
}
