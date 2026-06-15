/*
Copyright 2026 The KServe Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha2pool

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	v1 "sigs.k8s.io/gateway-api-inference-extension/api/v1"
)

func TestConvertTo_RoundTrip(t *testing.T) {
	original := &InferencePool{
		TypeMeta: metav1.TypeMeta{
			Kind:       "InferencePool",
			APIVersion: "inference.networking.x-k8s.io/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pool",
			Namespace: "default",
		},
		Spec: InferencePoolSpec{
			Selector: map[LabelKey]LabelValue{
				"app": "vllm",
			},
			TargetPortNumber: 8000,
			ExtensionRef: Extension{
				Group:       ptr.To(Group("")),
				Kind:        ptr.To(Kind("Service")),
				Name:        "my-epp",
				PortNumber:  ptr.To(PortNumber(9002)),
				FailureMode: ptr.To(FailClose),
			},
		},
	}

	// Convert v1alpha2 -> v1
	v1Pool := &v1.InferencePool{}
	err := original.ConvertTo(v1Pool)
	require.NoError(t, err)

	assert.Equal(t, v1.LabelValue("vllm"), v1Pool.Spec.Selector.MatchLabels["app"])
	assert.Equal(t, v1.PortNumber(8000), v1Pool.Spec.TargetPorts[0].Number)
	assert.Equal(t, v1.ObjectName("my-epp"), v1Pool.Spec.EndpointPickerRef.Name)
	assert.Equal(t, v1.EndpointPickerFailClose, v1Pool.Spec.EndpointPickerRef.FailureMode)

	// Convert v1 -> v1alpha2 (round trip)
	roundTripped := &InferencePool{}
	err = roundTripped.ConvertFrom(v1Pool)
	require.NoError(t, err)

	assert.Equal(t, original.Spec.Selector, roundTripped.Spec.Selector)
	assert.Equal(t, original.Spec.TargetPortNumber, roundTripped.Spec.TargetPortNumber)
	assert.Equal(t, original.Spec.ExtensionRef.Name, roundTripped.Spec.ExtensionRef.Name)
	assert.Equal(t, *original.Spec.ExtensionRef.PortNumber, *roundTripped.Spec.ExtensionRef.PortNumber)
	assert.Equal(t, *original.Spec.ExtensionRef.FailureMode, *roundTripped.Spec.ExtensionRef.FailureMode)
}

func TestConvertFrom_RoundTrip(t *testing.T) {
	original := &v1.InferencePool{
		TypeMeta: metav1.TypeMeta{
			Kind:       "InferencePool",
			APIVersion: "inference.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pool",
			Namespace: "default",
		},
		Spec: v1.InferencePoolSpec{
			Selector: v1.LabelSelector{
				MatchLabels: map[v1.LabelKey]v1.LabelValue{
					"app": "vllm",
				},
			},
			TargetPorts: []v1.Port{{Number: 8000}},
			EndpointPickerRef: v1.EndpointPickerRef{
				Group:       ptr.To(v1.Group("")),
				Kind:        v1.Kind("Service"),
				Name:        "my-epp",
				Port:        ptr.To(v1.Port{Number: 9002}),
				FailureMode: v1.EndpointPickerFailClose,
			},
		},
	}

	// Convert v1 -> v1alpha2
	alpha := &InferencePool{}
	err := alpha.ConvertFrom(original)
	require.NoError(t, err)

	assert.Equal(t, LabelValue("vllm"), alpha.Spec.Selector["app"])
	assert.Equal(t, int32(8000), alpha.Spec.TargetPortNumber)
	assert.Equal(t, ObjectName("my-epp"), alpha.Spec.ExtensionRef.Name)

	// Convert v1alpha2 -> v1 (round trip)
	roundTripped := &v1.InferencePool{}
	err = alpha.ConvertTo(roundTripped)
	require.NoError(t, err)

	assert.Equal(t, original.Spec.Selector.MatchLabels, roundTripped.Spec.Selector.MatchLabels)
	assert.Equal(t, original.Spec.TargetPorts[0].Number, roundTripped.Spec.TargetPorts[0].Number)
	assert.Equal(t, original.Spec.EndpointPickerRef.Name, roundTripped.Spec.EndpointPickerRef.Name)
	assert.Equal(t, original.Spec.EndpointPickerRef.FailureMode, roundTripped.Spec.EndpointPickerRef.FailureMode)
}

func TestSchemeRegistration(t *testing.T) {
	scheme := runtime.NewScheme()
	err := Install(scheme)
	require.NoError(t, err)

	// Verify InferencePool is registered with the correct group
	gvk := SchemeGroupVersion.WithKind("InferencePool")
	obj, err := scheme.New(gvk)
	require.NoError(t, err)
	assert.IsType(t, &InferencePool{}, obj)

	// Verify InferencePoolList is registered
	gvkList := SchemeGroupVersion.WithKind("InferencePoolList")
	objList, err := scheme.New(gvkList)
	require.NoError(t, err)
	assert.IsType(t, &InferencePoolList{}, objList)

	// Verify the group name
	assert.Equal(t, "inference.networking.x-k8s.io", GroupName)
	assert.Equal(t, "v1alpha2", SchemeGroupVersion.Version)
}

func TestConvertTo_NilFailureModeDefaultsToFailClose(t *testing.T) {
	original := &InferencePool{
		TypeMeta: metav1.TypeMeta{
			Kind:       "InferencePool",
			APIVersion: "inference.networking.x-k8s.io/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pool",
			Namespace: "default",
		},
		Spec: InferencePoolSpec{
			Selector: map[LabelKey]LabelValue{
				"app": "vllm",
			},
			TargetPortNumber: 8000,
			ExtensionRef: Extension{
				Group:       ptr.To(Group("")),
				Kind:        ptr.To(Kind("Service")),
				Name:        "my-epp",
				PortNumber:  ptr.To(PortNumber(9002)),
				FailureMode: nil,
			},
		},
	}

	v1Pool := &v1.InferencePool{}
	err := original.ConvertTo(v1Pool)
	require.NoError(t, err)

	assert.Equal(t, v1.EndpointPickerFailClose, v1Pool.Spec.EndpointPickerRef.FailureMode)
}

func TestConvertTo_NilKindDefaultsToService(t *testing.T) {
	original := &InferencePool{
		TypeMeta: metav1.TypeMeta{
			Kind:       "InferencePool",
			APIVersion: "inference.networking.x-k8s.io/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pool",
			Namespace: "default",
		},
		Spec: InferencePoolSpec{
			Selector:         map[LabelKey]LabelValue{"app": "vllm"},
			TargetPortNumber: 8000,
			ExtensionRef: Extension{
				Name:        "my-epp",
				Kind:        nil,
				FailureMode: ptr.To(FailClose),
			},
		},
	}

	v1Pool := &v1.InferencePool{}
	err := original.ConvertTo(v1Pool)
	require.NoError(t, err)

	assert.Equal(t, v1.Kind("Service"), v1Pool.Spec.EndpointPickerRef.Kind)
}

func TestConvertTo_NilParentKindDefaultsToGateway(t *testing.T) {
	original := &InferencePool{
		TypeMeta: metav1.TypeMeta{
			Kind:       "InferencePool",
			APIVersion: "inference.networking.x-k8s.io/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pool",
			Namespace: "default",
		},
		Spec: InferencePoolSpec{
			Selector:         map[LabelKey]LabelValue{"app": "vllm"},
			TargetPortNumber: 8000,
			ExtensionRef: Extension{
				Name:        "my-epp",
				FailureMode: ptr.To(FailClose),
			},
		},
		Status: InferencePoolStatus{
			Parents: []PoolStatus{
				{
					GatewayRef: ParentGatewayReference{
						Name: "my-gateway",
						Kind: nil,
					},
					Conditions: []metav1.Condition{
						{
							Type:   string(InferencePoolConditionAccepted),
							Status: metav1.ConditionTrue,
							Reason: string(InferencePoolReasonAccepted),
						},
					},
				},
			},
		},
	}

	v1Pool := &v1.InferencePool{}
	err := original.ConvertTo(v1Pool)
	require.NoError(t, err)

	assert.Equal(t, v1.Kind("Gateway"), v1Pool.Status.Parents[0].ParentRef.Kind)
}

func TestConvertTo_ObjectMetaDeepCopy(t *testing.T) {
	original := &InferencePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pool",
			Namespace: "default",
			Labels:    map[string]string{"key": "value"},
		},
		Spec: InferencePoolSpec{
			Selector:         map[LabelKey]LabelValue{"app": "vllm"},
			TargetPortNumber: 8000,
			ExtensionRef: Extension{
				Name:        "my-epp",
				FailureMode: ptr.To(FailClose),
			},
		},
	}

	v1Pool := &v1.InferencePool{}
	err := original.ConvertTo(v1Pool)
	require.NoError(t, err)

	original.Labels["key"] = "mutated"
	assert.Equal(t, "value", v1Pool.Labels["key"])
}

func TestDeepCopy(t *testing.T) {
	pool := &InferencePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pool",
			Namespace: "default",
		},
		Spec: InferencePoolSpec{
			Selector: map[LabelKey]LabelValue{
				"app": "vllm",
			},
			TargetPortNumber: 8000,
			ExtensionRef: Extension{
				Group:       ptr.To(Group("")),
				Kind:        ptr.To(Kind("Service")),
				Name:        "my-epp",
				PortNumber:  ptr.To(PortNumber(9002)),
				FailureMode: ptr.To(FailClose),
			},
		},
		Status: InferencePoolStatus{
			Parents: []PoolStatus{
				{
					GatewayRef: ParentGatewayReference{
						Name: "my-gateway",
					},
					Conditions: []metav1.Condition{
						{
							Type:   string(InferencePoolConditionAccepted),
							Status: metav1.ConditionTrue,
							Reason: string(InferencePoolReasonAccepted),
						},
					},
				},
			},
		},
	}

	copied := pool.DeepCopy()

	// Verify deep copy is independent
	assert.Equal(t, pool.Spec.Selector, copied.Spec.Selector)
	copied.Spec.Selector["new-key"] = "new-value"
	assert.NotEqual(t, pool.Spec.Selector, copied.Spec.Selector)

	// Verify status is deeply copied
	assert.Equal(t, pool.Status.Parents[0].Conditions[0].Type, copied.Status.Parents[0].Conditions[0].Type)
}
