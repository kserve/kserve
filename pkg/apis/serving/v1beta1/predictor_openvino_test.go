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

package v1beta1

import (
	"testing"

	"github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestOpenVINODefaulter(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	defaultResource := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("1"),
		corev1.ResourceMemory: resource.MustParse("2Gi"),
	}
	config := &InferenceServicesConfig{
		Resource: ResourceConfig{
			CPULimit:      "1",
			MemoryLimit:   "2Gi",
			CPURequest:    "1",
			MemoryRequest: "2Gi",
		},
	}
	protocolV2 := constants.ProtocolV2

	scenarios := map[string]struct {
		spec     PredictorSpec
		expected PredictorSpec
	}{
		"DefaultResourceAndProtocol": {
			spec: PredictorSpec{
				OpenVINO: &OpenVINOSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{},
				},
			},
			expected: PredictorSpec{
				OpenVINO: &OpenVINOSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: &protocolV2,
						Container: corev1.Container{
							Name: constants.InferenceServiceContainerName,
							Resources: corev1.ResourceRequirements{
								Requests: defaultResource,
								Limits:   defaultResource,
							},
						},
					},
				},
			},
		},
		"PreserveExplicitProtocol": {
			spec: PredictorSpec{
				OpenVINO: &OpenVINOSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: (*constants.InferenceServiceProtocol)(proto.String(string(constants.ProtocolV1))),
					},
				},
			},
			expected: PredictorSpec{
				OpenVINO: &OpenVINOSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: (*constants.InferenceServiceProtocol)(proto.String(string(constants.ProtocolV1))),
						Container: corev1.Container{
							Name: constants.InferenceServiceContainerName,
							Resources: corev1.ResourceRequirements{
								Requests: defaultResource,
								Limits:   defaultResource,
							},
						},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.spec.OpenVINO.Default(config)
			if !g.Expect(scenario.spec).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.spec, scenario.expected)
			}
		})
	}
}

func TestOpenVINOSpec_GetContainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	metadata := metav1.ObjectMeta{Name: constants.InferenceServiceContainerName}
	scenarios := map[string]struct {
		spec PredictorSpec
	}{
		"simple": {
			spec: PredictorSpec{
				OpenVINO: &OpenVINOSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("s3://modelzoo"),
						Container: corev1.Container{
							Name:      constants.InferenceServiceContainerName,
							Image:     "openvino/model_server:latest",
							Args:      nil,
							Env:       nil,
							Resources: corev1.ResourceRequirements{},
						},
					},
				},
				ComponentExtensionSpec: ComponentExtensionSpec{},
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res := scenario.spec.OpenVINO.GetContainer(metadata, &scenario.spec.ComponentExtensionSpec, nil)
			if !g.Expect(res).To(gomega.Equal(&scenario.spec.OpenVINO.Container)) {
				t.Errorf("got %v, want %v", res, scenario.spec.OpenVINO.Container)
			}
		})
	}
}

func TestOpenVINOSpec_GetProtocol(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec     PredictorSpec
		expected constants.InferenceServiceProtocol
	}{
		"default": {
			spec: PredictorSpec{
				OpenVINO: &OpenVINOSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						StorageURI: proto.String("s3://modelzoo"),
						Container: corev1.Container{
							Image:     "openvino/model_server:latest",
							Args:      nil,
							Env:       nil,
							Resources: corev1.ResourceRequirements{},
						},
					},
				},
				ComponentExtensionSpec: ComponentExtensionSpec{},
			},
			// unlike most predictors, OpenVINO defaults to v2 (not v1) when unset.
			expected: constants.ProtocolV2,
		},
		"ProtocolSpecified": {
			spec: PredictorSpec{
				OpenVINO: &OpenVINOSpec{
					PredictorExtensionSpec: PredictorExtensionSpec{
						ProtocolVersion: (*constants.InferenceServiceProtocol)(proto.String(string(constants.ProtocolGRPCV2))),
						StorageURI:      proto.String("s3://modelzoo"),
						Container: corev1.Container{
							Image:     "openvino/model_server:latest",
							Args:      nil,
							Env:       nil,
							Resources: corev1.ResourceRequirements{},
						},
					},
				},
				ComponentExtensionSpec: ComponentExtensionSpec{},
			},
			expected: constants.ProtocolGRPCV2,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res := scenario.spec.OpenVINO.GetProtocol()
			if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", res, scenario.expected)
			}
		})
	}
}

// TestOpenVINOClusterServingRuntimeSelection verifies that a ClusterServingRuntime shaped like the
// built-in "kserve-openvino" runtime (config/runtimes/kserve-openvino.yaml) is actually discovered and
// selected by the runtime auto-selection logic for a ModelSpec requesting the "openvino" model format.
// This guards against the runtime manifest silently becoming unreachable (e.g. wrong autoSelect/label
// wiring) even though the OpenVINOSpec defaulting/container logic above is otherwise correct.
func TestOpenVINOClusterServingRuntimeSelection(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	namespace := "default"

	openvinoRuntimeName := "kserve-openvino"
	protocolV2 := constants.ProtocolV2

	openvinoRuntime := v1alpha1.ClusterServingRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: openvinoRuntimeName},
		Spec: v1alpha1.ServingRuntimeSpec{
			SupportedModelFormats: []v1alpha1.SupportedModelFormat{
				{
					Name:       constants.SupportedModelOpenVINO,
					Version:    ptr.To("10"),
					AutoSelect: ptr.To(true),
					Priority:   ptr.To(int32(1)),
				},
				{
					Name:       constants.SupportedModelONNX,
					Version:    ptr.To("1"),
					AutoSelect: ptr.To(true),
					Priority:   ptr.To(int32(2)),
				},
			},
			ProtocolVersions: []constants.InferenceServiceProtocol{constants.ProtocolV1, constants.ProtocolV2, constants.ProtocolGRPCV2},
			ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
				Containers: []corev1.Container{
					{
						Name:  constants.InferenceServiceContainerName,
						Image: "openvino/model_server:latest",
						Args: []string{
							"--model_name={{.Name}}",
							"--model_path=/mnt/models",
							"--port=9000",
							"--rest_port=8080",
							"--file_system_poll_wait_seconds=0",
						},
					},
				},
			},
			Disabled: ptr.To(false),
		},
	}

	scenarios := map[string]struct {
		modelFormat string
		expected    []v1alpha1.SupportedRuntime
	}{
		"OpenVINOModelFormatAutoSelectsOpenVINORuntime": {
			modelFormat: constants.SupportedModelOpenVINO,
			expected: []v1alpha1.SupportedRuntime{
				{Name: openvinoRuntimeName, Spec: openvinoRuntime.Spec},
			},
		},
		"ONNXModelFormatAlsoAutoSelectsOpenVINORuntime": {
			modelFormat: constants.SupportedModelONNX,
			expected: []v1alpha1.SupportedRuntime{
				{Name: openvinoRuntimeName, Spec: openvinoRuntime.Spec},
			},
		},
		"UnsupportedModelFormatSelectsNoRuntime": {
			modelFormat: constants.SupportedModelSKLearn,
			expected:    []v1alpha1.SupportedRuntime{},
		},
	}

	s := runtime.NewScheme()
	g.Expect(v1alpha1.AddToScheme(s)).To(gomega.Succeed())

	clusterRuntimes := &v1alpha1.ClusterServingRuntimeList{Items: []v1alpha1.ClusterServingRuntime{openvinoRuntime}}
	mockClient := fake.NewClientBuilder().WithLists(clusterRuntimes).WithScheme(s).Build()

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			modelSpec := &ModelSpec{
				ModelFormat: ModelFormat{Name: scenario.modelFormat},
				PredictorExtensionSpec: PredictorExtensionSpec{
					ProtocolVersion: &protocolV2,
				},
			}
			res, err := modelSpec.GetSupportingRuntimes(t.Context(), mockClient, namespace, false, false)
			g.Expect(err).ToNot(gomega.HaveOccurred())
			if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", res, scenario.expected)
			}
		})
	}
}
