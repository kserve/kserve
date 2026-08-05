package fixture

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"

	platformv1alpha1 "github.com/opendatahub-io/kserve-module/pkg/apis/v1alpha1"
)

type KserveOption func(*platformv1alpha1.Kserve)

func KserveCR(opts ...KserveOption) *platformv1alpha1.Kserve {
	cr := &platformv1alpha1.Kserve{
		ObjectMeta: metav1.ObjectMeta{
			Name: platformv1alpha1.KserveInstanceName,
		},
		Spec: platformv1alpha1.KserveSpec{
			ManagementSpec: common.ManagementSpec{
				ManagementState: common.Managed,
			},
		},
	}
	for _, opt := range opts {
		opt(cr)
	}
	return cr
}

func WithName(name string) KserveOption {
	return func(k *platformv1alpha1.Kserve) {
		k.Name = name
	}
}

func WithRawServiceConfig(config platformv1alpha1.RawServiceConfig) KserveOption {
	return func(k *platformv1alpha1.Kserve) {
		k.Spec.RawDeploymentServiceConfig = config
	}
}

func WithManagementState(state common.ManagementState) KserveOption {
	return func(k *platformv1alpha1.Kserve) {
		k.Spec.ManagementState = state
	}
}

func WithWVAManagementState(state common.ManagementState) KserveOption {
	return func(k *platformv1alpha1.Kserve) {
		k.Spec.WVA.ManagementState = state
	}
}

func WithEnableLLMInferenceServiceTLS(val *bool) KserveOption {
	return func(k *platformv1alpha1.Kserve) {
		k.Spec.EnableLLMInferenceServiceTLS = val
	}
}

func WithEnableLLMInferenceServiceConsoleDashboards(val *bool) KserveOption {
	return func(k *platformv1alpha1.Kserve) {
		k.Spec.EnableLLMInferenceServiceConsoleDashboards = val
	}
}

func WithModelRegistryManagementState(state common.ManagementState) KserveOption {
	return func(k *platformv1alpha1.Kserve) {
		k.Spec.ModelRegistry.ManagementState = state
	}
}

func WithAnnotation(key, value string) KserveOption {
	return func(k *platformv1alpha1.Kserve) {
		if k.Annotations == nil {
			k.Annotations = make(map[string]string)
		}
		k.Annotations[key] = value
	}
}
