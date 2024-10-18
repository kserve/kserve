/*
Copyright 2021 The KServe Authors.

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

package utils

import (
	"strconv"
	"testing"

	"github.com/onsi/gomega/types"
	"knative.dev/pkg/apis"
	knativeV1 "knative.dev/pkg/apis/duck/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	. "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIsMMSPredictor(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	var requestedResource = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			"cpu": resource.MustParse("100m"),
		},
		Requests: v1.ResourceList{
			"cpu": resource.MustParse("90m"),
		},
	}

	scenarios := map[string]struct {
		isvc     InferenceService
		expected bool
	}{
		"ModelSpec": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "sklearn",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								Container: v1.Container{
									Image:     "customImage:0.1.0",
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		"ModelWithURI": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearnWithURI",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "sklearn",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://someUri"),
								Container: v1.Container{
									Image:     "customImage:0.1.0",
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expected: false,
		},
		"HuggingFaceModel": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hg-model",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						HuggingFace: &HuggingFaceRuntimeSpec{
							PredictorExtensionSpec: PredictorExtensionSpec{RuntimeVersion: proto.String("latest")}},
					},
				},
			},
			expected: false,
		},
		"CustomSpec": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "CustomSpecMMS",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PodSpec: PodSpec{
							Containers: []v1.Container{
								{Name: constants.InferenceServiceContainerName,
									Image: "some-image",
									Env:   []v1.EnvVar{{Name: constants.CustomSpecMultiModelServerEnvVarKey, Value: strconv.FormatBool(true)}},
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		"CustomSpecWithURI": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "CustomSpecMMSWithURI",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PodSpec: PodSpec{
							Containers: []v1.Container{
								{Name: constants.InferenceServiceContainerName,
									Image: "some-image",
									Env: []v1.EnvVar{
										{Name: constants.CustomSpecMultiModelServerEnvVarKey, Value: strconv.FormatBool(false)},
										{Name: constants.CustomSpecStorageUriEnvVarKey, Value: "gs://some-uri"}},
								},
							},
						},
					},
				},
			},
			expected: false,
		},
		"CustomSpecWithoutURI": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "CustomSpecMMSWithURI",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PodSpec: PodSpec{
							Containers: []v1.Container{
								{Name: constants.InferenceServiceContainerName,
									Image: "some-image",
								},
							},
						},
					},
				},
			},
			expected: false,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res := IsMMSPredictor(&scenario.isvc.Spec.Predictor)
			if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %t, want %t", res, scenario.expected)
			}
		})
	}
}

func TestIsMemoryResourceAvailable(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	reqResourcesScenarios := map[string]struct {
		resource v1.ResourceRequirements
		expected bool
	}{
		"EnoughMemoryResource": {
			// Enough memory
			resource: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					"cpu":    resource.MustParse("100m"),
					"memory": resource.MustParse("100Gi"),
				},
				Requests: v1.ResourceList{
					"cpu":    resource.MustParse("90m"),
					"memory": resource.MustParse("100Gi"),
				},
			},
			expected: true,
		},
		"NotEnoughMemoryResource": {
			// Enough memory
			resource: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					"cpu":    resource.MustParse("100m"),
					"memory": resource.MustParse("1Mi"),
				},
				Requests: v1.ResourceList{
					"cpu":    resource.MustParse("90m"),
					"memory": resource.MustParse("1Mi"),
				},
			},
			expected: false,
		},
	}

	totalReqMemory := resource.MustParse("1Gi")

	protocolV1 := constants.ProtocolV1
	protocolV2 := constants.ProtocolV2

	for _, reqResourcesScenario := range reqResourcesScenarios {
		scenarios := map[string]struct {
			isvc InferenceService
		}{
			"LightGBM": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "lightgbm",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							LightGBM: &LightGBMSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									Container: v1.Container{
										Image:     "customImage:0.1.0",
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"LightGBMWithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "lightgbmWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							LightGBM: &LightGBMSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI: proto.String("gs://someUri"),
									Container: v1.Container{
										Image:     "customImage:0.1.0",
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"Onnx": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "onnx",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							ONNX: &ONNXRuntimeSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									Container: v1.Container{
										Image:     "mcr.microsoft.com/onnxruntime/server:v0.5.0",
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"OnnxWithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "onnxWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							ONNX: &ONNXRuntimeSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI: proto.String("gs://someUri"),
									Container: v1.Container{
										Image:     "mcr.microsoft.com/onnxruntime/server:v0.5.0",
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"Pmml": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pmml",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PMML: &PMMLSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"PmmlWithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pmmlWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PMML: &PMMLSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI: proto.String("gs://someUri"),
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"SKLearnV1": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sklearn",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							SKLearn: &SKLearnSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									ProtocolVersion: &protocolV1,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"SKLearnV2": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sklearn2",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							SKLearn: &SKLearnSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									ProtocolVersion: &protocolV2,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"SKLearnV1WithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sklearnWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							SKLearn: &SKLearnSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:      proto.String("gs://someUri"),
									ProtocolVersion: &protocolV1,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"SKLearnV2WithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "sklearn2WithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							SKLearn: &SKLearnSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:      proto.String("gs://someUri"),
									ProtocolVersion: &protocolV2,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"tfserving": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "tfserving",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							Tensorflow: &TFServingSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"TfservingWithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "tfservingWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							Tensorflow: &TFServingSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI: proto.String("gs://someUri"),
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"PyTorchV1": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pytorch",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PyTorch: &TorchServeSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									ProtocolVersion: &protocolV1,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"PyTorchV2": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pytorch2",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PyTorch: &TorchServeSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									ProtocolVersion: &protocolV2,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"PyTorchV1WithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pytorchWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PyTorch: &TorchServeSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:      proto.String("gs://someUri"),
									ProtocolVersion: &protocolV1,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"PyTorchV2WithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pytorch2WithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							PyTorch: &TorchServeSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:      proto.String("gs://someUri"),
									ProtocolVersion: &protocolV2,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"Triton": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "triton",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							Triton: &TritonSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									ProtocolVersion: &protocolV1,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"TritonWithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "tritonWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							Triton: &TritonSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:      proto.String("gs://someUri"),
									ProtocolVersion: &protocolV1,
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"XGBoostV1": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "xgboost",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							XGBoost: &XGBoostSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									RuntimeVersion: proto.String("0.1.0"),
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"XGBoostV2": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "xgboost2",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							XGBoost: &XGBoostSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									ProtocolVersion: &protocolV2,
									RuntimeVersion:  proto.String("0.1.0"),
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"XGBoostV1WithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "xgboostWithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							XGBoost: &XGBoostSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:     proto.String("gs://someUri"),
									RuntimeVersion: proto.String("0.1.0"),
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
			"XGBoostV2WithURI": {
				isvc: InferenceService{
					ObjectMeta: metav1.ObjectMeta{
						Name: "xgboost2WithURI",
					},
					Spec: InferenceServiceSpec{
						Predictor: PredictorSpec{
							XGBoost: &XGBoostSpec{
								PredictorExtensionSpec: PredictorExtensionSpec{
									StorageURI:      proto.String("gs://someUri"),
									ProtocolVersion: &protocolV2,
									RuntimeVersion:  proto.String("0.1.0"),
									Container: v1.Container{
										Name:      constants.InferenceServiceContainerName,
										Resources: reqResourcesScenario.resource,
									},
								},
							},
						},
					},
				},
			},
		}

		for name, scenario := range scenarios {
			t.Run(name, func(t *testing.T) {
				scenario.isvc.Spec.Predictor.GetImplementation().Default(nil)
				res := IsMemoryResourceAvailable(&scenario.isvc, totalReqMemory)
				if !g.Expect(res).To(gomega.Equal(reqResourcesScenario.expected)) {
					t.Errorf("got %t, want %t with memory %s vs %s", res, reqResourcesScenario.expected, reqResourcesScenario.resource.Limits.Memory().String(), totalReqMemory.String())
				}
			})
		}
	}
}
func TestMergeRuntimeContainers(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		containerBase     *v1.Container
		containerOverride *v1.Container
		expected          *v1.Container
	}{
		"BasicMerge": {
			containerBase: &v1.Container{
				Name:  "kserve-container",
				Image: "default-image",
				Args: []string{
					"--foo=bar",
					"--test=dummy",
				},
				Env: []v1.EnvVar{
					{Name: "PORT", Value: "8080"},
					{Name: "PORT2", Value: "8081"},
				},
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("1"),
						v1.ResourceMemory: resource.MustParse("2Gi"),
					},
					Requests: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("1"),
						v1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
			},
			containerOverride: &v1.Container{
				Args: []string{
					"--new-arg=baz",
				},
				Env: []v1.EnvVar{
					{Name: "PORT2", Value: "8082"},
					{Name: "Some", Value: "Var"},
				},
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("2"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
				},
			},
			expected: &v1.Container{
				Name:  "kserve-container",
				Image: "default-image",
				Args: []string{
					"--foo=bar",
					"--test=dummy",
					"--new-arg=baz",
				},
				Env: []v1.EnvVar{
					{Name: "PORT", Value: "8080"},
					{Name: "PORT2", Value: "8082"},
					{Name: "Some", Value: "Var"},
				},
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("2"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
					Requests: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("1"),
						v1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res, _ := MergeRuntimeContainers(scenario.containerBase, scenario.containerOverride)
			if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", res, scenario.expected)
			}
		})
	}
}

func TestMergePodSpec(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		podSpecBase     *v1alpha1.ServingRuntimePodSpec
		podSpecOverride *PodSpec
		expected        *v1.PodSpec
	}{
		"BasicMerge": {
			podSpecBase: &v1alpha1.ServingRuntimePodSpec{
				NodeSelector: map[string]string{
					"foo": "bar",
					"aaa": "bbb",
				},
				Tolerations: []v1.Toleration{
					{Key: "key1", Operator: v1.TolerationOpExists, Effect: v1.TaintEffectNoSchedule},
				},
				Volumes: []v1.Volume{
					{
						Name: "foo",
						VolumeSource: v1.VolumeSource{
							PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
								ClaimName: "bar",
							},
						},
					},
					{
						Name: "aaa",
						VolumeSource: v1.VolumeSource{
							PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
								ClaimName: "bbb",
							},
						},
					},
				},
				ImagePullSecrets: []v1.LocalObjectReference{
					{Name: "foo"},
				},
			},
			podSpecOverride: &PodSpec{
				NodeSelector: map[string]string{
					"foo": "baz",
					"xxx": "yyy",
				},
				ServiceAccountName: "testAccount",
				Volumes: []v1.Volume{
					{
						Name: "foo",
						VolumeSource: v1.VolumeSource{
							PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
								ClaimName: "baz",
							},
						},
					},
					{
						Name: "xxx",
						VolumeSource: v1.VolumeSource{
							PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
								ClaimName: "yyy",
							},
						},
					},
				},
				ImagePullSecrets: []v1.LocalObjectReference{
					{Name: "foo"},
					{Name: "bar"},
				},
			},
			expected: &v1.PodSpec{
				NodeSelector: map[string]string{
					"foo": "baz",
					"xxx": "yyy",
					"aaa": "bbb",
				},
				Tolerations: []v1.Toleration{
					{Key: "key1", Operator: v1.TolerationOpExists, Effect: v1.TaintEffectNoSchedule},
				},
				ServiceAccountName: "testAccount",
				Volumes: []v1.Volume{
					{
						Name: "foo",
						VolumeSource: v1.VolumeSource{
							PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
								ClaimName: "baz",
							},
						},
					},
					{
						Name: "xxx",
						VolumeSource: v1.VolumeSource{
							PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
								ClaimName: "yyy",
							},
						},
					},
					{
						Name: "aaa",
						VolumeSource: v1.VolumeSource{
							PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
								ClaimName: "bbb",
							},
						},
					},
				},
				ImagePullSecrets: []v1.LocalObjectReference{
					{Name: "foo"},
					{Name: "bar"},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res, _ := MergePodSpec(scenario.podSpecBase, scenario.podSpecOverride)
			if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", res, scenario.expected)
			}
		})
	}
}

func TestGetServingRuntime(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	namespace := "default"

	tfRuntime := "tf-runtime"
	sklearnRuntime := "sklearn-runtime"

	servingRuntimeSpecs := map[string]v1alpha1.ServingRuntimeSpec{
		tfRuntime: {
			SupportedModelFormats: []v1alpha1.SupportedModelFormat{
				{
					Name:    "tensorflow",
					Version: proto.String("1"),
				},
			},
			ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
				Containers: []v1.Container{
					{
						Name:  "kserve-container",
						Image: tfRuntime + "-image:latest",
					},
				},
			},
			Disabled: proto.Bool(false),
		},
		sklearnRuntime: {
			SupportedModelFormats: []v1alpha1.SupportedModelFormat{
				{
					Name:    "sklearn",
					Version: proto.String("0"),
				},
			},
			ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
				Containers: []v1.Container{
					{
						Name:  "kserve-container",
						Image: sklearnRuntime + "-image:latest",
					},
				},
			},
			Disabled: proto.Bool(false),
		},
	}

	runtimes := &v1alpha1.ServingRuntimeList{
		Items: []v1alpha1.ServingRuntime{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tfRuntime,
					Namespace: namespace,
				},
				Spec: servingRuntimeSpecs[tfRuntime],
			},
		},
	}

	clusterRuntimes := &v1alpha1.ClusterServingRuntimeList{
		Items: []v1alpha1.ClusterServingRuntime{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: sklearnRuntime,
				},
				Spec: servingRuntimeSpecs[sklearnRuntime],
			},
		},
	}

	scenarios := map[string]struct {
		runtimeName string
		expected    v1alpha1.ServingRuntimeSpec
	}{
		"NamespaceServingRuntime": {
			runtimeName: tfRuntime,
			expected:    servingRuntimeSpecs[tfRuntime],
		},
		"ClusterServingRuntime": {
			runtimeName: sklearnRuntime,
			expected:    servingRuntimeSpecs[sklearnRuntime],
		},
	}

	s := runtime.NewScheme()
	v1alpha1.AddToScheme(s)

	mockClient := fake.NewClientBuilder().WithLists(runtimes, clusterRuntimes).WithScheme(s).Build()
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res, _ := GetServingRuntime(mockClient, scenario.runtimeName, namespace)
			if !g.Expect(res).To(gomega.Equal(&scenario.expected)) {
				t.Errorf("got %v, want %v", res, &scenario.expected)
			}
		})
	}

	// Check invalid case
	t.Run("InvalidServingRuntime", func(t *testing.T) {
		res, err := GetServingRuntime(mockClient, "foo", namespace)
		if !g.Expect(res).To(gomega.BeNil()) {
			t.Errorf("got %v, want %v", res, nil)
		}
		g.Expect(err.Error()).To(gomega.ContainSubstring("No ServingRuntimes or ClusterServingRuntimes with the name"))
	})

}

func TestReplacePlaceholders(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		container *v1.Container
		meta      metav1.ObjectMeta
		expected  *v1.Container
	}{
		"ReplaceArgsAndEnvPlaceholders": {
			container: &v1.Container{
				Name:  "kserve-container",
				Image: "default-image",
				Args: []string{
					"--foo={{.Name}}",
					"--test=dummy",
					"--new-arg=baz",
				},
				Env: []v1.EnvVar{
					{Name: "PORT", Value: "8080"},
					{Name: "MODELS_DIR", Value: "{{.Labels.modelDir}}"},
				},
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("2"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
					Requests: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("1"),
						v1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
			},
			meta: metav1.ObjectMeta{
				Name: "bar",
				Labels: map[string]string{
					"modelDir": "/mnt/models",
				},
			},
			expected: &v1.Container{
				Name:  "kserve-container",
				Image: "default-image",
				Args: []string{
					"--foo=bar",
					"--test=dummy",
					"--new-arg=baz",
				},
				Env: []v1.EnvVar{
					{Name: "PORT", Value: "8080"},
					{Name: "MODELS_DIR", Value: "/mnt/models"},
				},
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("2"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
					Requests: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("1"),
						v1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
			},
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			_ = ReplacePlaceholders(scenario.container, scenario.meta)
			if !g.Expect(scenario.container).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.container, scenario.expected)
			}
		})
	}
}

func TestUpdateImageTag(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		container      *v1.Container
		runtimeVersion *string
		servingRuntime string
		isvcConfig     *InferenceServicesConfig
		expected       string
	}{
		"UpdateRuntimeVersion": {
			container: &v1.Container{
				Name:  "kserve-container",
				Image: "tfserving",
				Args: []string{
					"--foo=bar",
					"--test=dummy",
					"--new-arg=baz",
				},
				Env: []v1.EnvVar{
					{Name: "PORT", Value: "8080"},
					{Name: "MODELS_DIR", Value: "/mnt/models"},
				},
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("2"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
					Requests: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("1"),
						v1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
			},
			runtimeVersion: proto.String("2.6.2"),
			servingRuntime: constants.TFServing,
			expected:       "tfserving:2.6.2",
		},
		"UpdateGPUImageTag": {
			container: &v1.Container{
				Name:  "kserve-container",
				Image: "tfserving:1.14.0",
				Args: []string{
					"--foo=bar",
					"--test=dummy",
					"--new-arg=baz",
				},
				Env: []v1.EnvVar{
					{Name: "PORT", Value: "8080"},
					{Name: "MODELS_DIR", Value: "/mnt/models"},
				},
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						"nvidia.com/gpu": resource.MustParse("1"),
					},
				},
			},
			runtimeVersion: nil,
			servingRuntime: constants.TFServing,
			expected:       "tfserving:1.14.0-gpu",
		},
		"UpdateGPUImageTagWithProxy": {
			container: &v1.Container{
				Name:  "kserve-container",
				Image: "localhost:8888/tfserving:1.14.0",
				Args: []string{
					"--foo=bar",
					"--test=dummy",
					"--new-arg=baz",
				},
				Env: []v1.EnvVar{
					{Name: "PORT", Value: "8080"},
					{Name: "MODELS_DIR", Value: "/mnt/models"},
				},
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						"nvidia.com/gpu": resource.MustParse("1"),
					},
				},
			},
			runtimeVersion: nil,
			servingRuntime: constants.TFServing,
			expected:       "localhost:8888/tfserving:1.14.0-gpu",
		},
		"UpdateRuntimeVersionWithProxy": {
			container: &v1.Container{
				Name:  "kserve-container",
				Image: "localhost:8888/tfserving",
				Args: []string{
					"--foo=bar",
					"--test=dummy",
					"--new-arg=baz",
				},
				Env: []v1.EnvVar{
					{Name: "PORT", Value: "8080"},
					{Name: "MODELS_DIR", Value: "/mnt/models"},
				},
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("2"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
					Requests: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("1"),
						v1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
			},
			runtimeVersion: proto.String("2.6.2"),
			servingRuntime: constants.TFServing,
			expected:       "localhost:8888/tfserving:2.6.2",
		},
		"UpdateRuntimeVersionWithProxyAndTag": {
			container: &v1.Container{
				Name:  "kserve-container",
				Image: "localhost:8888/tfserving:1.2.3",
				Args: []string{
					"--foo=bar",
					"--test=dummy",
					"--new-arg=baz",
				},
				Env: []v1.EnvVar{
					{Name: "PORT", Value: "8080"},
					{Name: "MODELS_DIR", Value: "/mnt/models"},
				},
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("2"),
						v1.ResourceMemory: resource.MustParse("4Gi"),
					},
					Requests: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("1"),
						v1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
			},
			runtimeVersion: proto.String("2.6.2"),
			servingRuntime: constants.TFServing,
			expected:       "localhost:8888/tfserving:2.6.2",
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			UpdateImageTag(scenario.container, scenario.runtimeVersion, &scenario.servingRuntime)
			if !g.Expect(scenario.container.Image).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", scenario.container.Image, scenario.expected)
			}
		})
	}
}

func TestGetDeploymentMode(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		annotations  map[string]string
		deployConfig *DeployConfig
		expected     constants.DeploymentModeType
	}{
		"RawDeployment": {
			annotations: map[string]string{
				constants.DeploymentMode: string(constants.RawDeployment),
			},
			deployConfig: &DeployConfig{},
			expected:     constants.DeploymentModeType(constants.RawDeployment),
		},
		"ServerlessDeployment": {
			annotations: map[string]string{
				constants.DeploymentMode: string(constants.Serverless),
			},
			deployConfig: &DeployConfig{},
			expected:     constants.DeploymentModeType(constants.Serverless),
		},
		"ModelMeshDeployment": {
			annotations: map[string]string{
				constants.DeploymentMode: string(constants.ModelMeshDeployment),
			},
			deployConfig: &DeployConfig{},
			expected:     constants.DeploymentModeType(constants.ModelMeshDeployment),
		},
		"DefaultDeploymentMode": {
			annotations: map[string]string{},
			deployConfig: &DeployConfig{
				DefaultDeploymentMode: string(constants.Serverless),
			},
			expected: constants.DeploymentModeType(constants.Serverless),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			deploymentMode := GetDeploymentMode(scenario.annotations, scenario.deployConfig)
			if !g.Expect(deploymentMode).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %v, want %v", deploymentMode, scenario.expected)
			}
		})
	}
}

func TestModelName(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	var requestedResource = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			"cpu": resource.MustParse("100m"),
		},
		Requests: v1.ResourceList{
			"cpu": resource.MustParse("90m"),
		},
	}

	scenarios := map[string]struct {
		isvc     InferenceService
		expected string
	}{
		"ModelSpec": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "sklearn",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								Container: v1.Container{
									Image:     "customImage:0.1.0",
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expected: "sklearn",
		},
		"CustomModelName": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn-iris",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "sklearn",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("gs://someUri"),
								Container: v1.Container{
									Args:      []string{"--model_name=sklearn-custom"},
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expected: "sklearn-custom",
		},
		"CustomSpec": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "CustomSpecMMS",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PodSpec: PodSpec{
							Containers: []v1.Container{
								{Name: constants.InferenceServiceContainerName,
									Image: "some-image",
								},
							},
						},
					},
				},
			},
			expected: "CustomSpecMMS",
		},
		"CustomSpecWithModelNameArg": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "Custom",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PodSpec: PodSpec{
							Containers: []v1.Container{
								{Name: constants.InferenceServiceContainerName,
									Image: "some-image",
									Args:  []string{"--model_name=custom-model"},
								},
							},
						},
					},
				},
			},
			expected: "custom-model",
		},
		"ModelSpecWithModelNameEnv": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn-iris-v2",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "sklearn",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								Container: v1.Container{
									Env:       []v1.EnvVar{{Name: constants.MLServerModelNameEnv, Value: "sklearn-custom"}},
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expected: "sklearn-custom",
		},
		"TransformerWithModelNameArg": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn-iris-transformer",
				},
				Spec: InferenceServiceSpec{
					Transformer: &TransformerSpec{
						PodSpec: PodSpec{
							Containers: []v1.Container{
								{Name: constants.InferenceServiceContainerName,
									Image: "some-image",
									Args:  []string{"--model_name=custom-model"},
								},
							},
						},
					},
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "sklearn",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								Container: v1.Container{
									Resources: requestedResource,
								},
							},
						},
					},
				},
			},
			expected: "custom-model",
		},
		"multiple modelname arguments": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "sklearn",
							},

							PredictorExtensionSpec: PredictorExtensionSpec{
								Container: v1.Container{
									Image:     "customImage:0.1.0",
									Resources: requestedResource,
									Args:      []string{"--model_name=sklearn", "--model_dir", "/mnt/models", "--model_name", "iris"},
								},
							},
						},
					},
				},
			},
			expected: "iris",
		},
		"modelname argument and value in single string": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "sklearn",
							},

							PredictorExtensionSpec: PredictorExtensionSpec{
								Container: v1.Container{
									Image:     "customImage:0.1.0",
									Resources: requestedResource,
									Args:      []string{"--model_dir", "/mnt/models", "--model_name iris"}, // This format is not recognized by the modelserver. So we ignore this format.
								},
							},
						},
					},
				},
			},
			expected: "sklearn",
		},
		"modelname value in separate string": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "sklearn",
							},

							PredictorExtensionSpec: PredictorExtensionSpec{
								Container: v1.Container{
									Image:     "customImage:0.1.0",
									Resources: requestedResource,
									Args:      []string{"--model_dir", "/mnt/models", "--model_name", "iris"},
								},
							},
						},
					},
				},
			},
			expected: "iris",
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res := GetModelName(&scenario.isvc)
			if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %s, want %s", res, scenario.expected)
			}
		})
	}
}

func TestGetPredictorEndpoint(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	var requestedResource = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			"cpu": resource.MustParse("100m"),
		},
		Requests: v1.ResourceList{
			"cpu": resource.MustParse("90m"),
		},
	}

	scenarios := map[string]struct {
		isvc        InferenceService
		expectedUrl string
		expectedErr types.GomegaMatcher
	}{
		"MMSPredictor": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "sklearn",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								Container: v1.Container{
									Image:     "customImage:0.1.0",
									Resources: requestedResource,
								},
							},
						},
					},
				},
				Status: InferenceServiceStatus{
					Address: &knativeV1.Addressable{
						URL: &apis.URL{
							Scheme: "http",
							Host:   "sklearn-predictor.default.svc.cluster.local",
						},
					},
				},
			},
			expectedUrl: "http://sklearn-predictor.default.svc.cluster.local",
			expectedErr: gomega.BeNil(),
		},
		"DefaultProtocol": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "sklearn",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("s3://test"),
							},
						},
					},
				},
				Status: InferenceServiceStatus{
					Address: &knativeV1.Addressable{
						URL: &apis.URL{
							Scheme: "http",
							Host:   "sklearn-predictor.default.svc.cluster.local",
						},
					},
				},
			},
			expectedUrl: "http://sklearn-predictor.default.svc.cluster.local/v1/models/sklearn:predict",
			expectedErr: gomega.BeNil(),
		},
		"DefaultProtocolVersionWithTransformer": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "sklearn",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("s3://test"),
							},
						},
					},
					Transformer: &TransformerSpec{
						PodSpec: PodSpec{
							Containers: []v1.Container{
								{
									Name:  constants.InferenceServiceContainerName,
									Image: "kserve/transformer:1.0",
								},
							},
						},
					},
				},
				Status: InferenceServiceStatus{
					Address: &knativeV1.Addressable{
						URL: &apis.URL{
							Scheme: "http",
							Host:   "sklearn-transformer.default.svc.cluster.local",
						},
					},
				},
			},
			expectedUrl: "http://sklearn-transformer.default.svc.cluster.local/v1/models/sklearn:predict",
			expectedErr: gomega.BeNil(),
		},
		"ProtocolSpecified": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "sklearn",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI:      proto.String("s3://test"),
								ProtocolVersion: (*constants.InferenceServiceProtocol)(proto.String(string(constants.ProtocolV2))),
							},
						},
					},
				},
				Status: InferenceServiceStatus{
					Address: &knativeV1.Addressable{
						URL: &apis.URL{
							Scheme: "http",
							Host:   "sklearn-predictor.default.svc.cluster.local",
						},
					},
				},
			},
			expectedUrl: "http://sklearn-predictor.default.svc.cluster.local/v2/models/sklearn/infer",
			expectedErr: gomega.BeNil(),
		},
		"ProtocolSpecifiedWithTransformer": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "sklearn",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("s3://test"),
							},
						},
					},
					Transformer: &TransformerSpec{
						PodSpec: PodSpec{
							Containers: []v1.Container{
								{
									Name:  constants.InferenceServiceContainerName,
									Image: "kserve/transformer:1.0",
									Env: []v1.EnvVar{
										{
											Name:  constants.CustomSpecProtocolEnvVarKey,
											Value: string(constants.ProtocolV2),
										},
									},
								},
							},
						},
					},
				},
				Status: InferenceServiceStatus{
					Address: &knativeV1.Addressable{
						URL: &apis.URL{
							Scheme: "http",
							Host:   "sklearn-transformer.default.svc.cluster.local",
						},
					},
				},
			},
			expectedUrl: "http://sklearn-transformer.default.svc.cluster.local/v2/models/sklearn/infer",
			expectedErr: gomega.BeNil(),
		},
		"CustomModel": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn-iris",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						PodSpec: PodSpec{
							Containers: []v1.Container{
								{
									Name:      constants.InferenceServiceContainerName,
									Image:     "kserve/custom-image:1.0",
									Args:      []string{"--model_name=sklearn-custom"},
									Resources: requestedResource,
									Env: []v1.EnvVar{
										{
											Name:  constants.CustomSpecProtocolEnvVarKey,
											Value: string(constants.ProtocolV2),
										},
										{
											Name:  constants.CustomSpecStorageUriEnvVarKey,
											Value: "s3://test",
										},
										{
											Name:  constants.CustomSpecMultiModelServerEnvVarKey,
											Value: strconv.FormatBool(false),
										},
									},
								},
							},
						},
					},
				},
				Status: InferenceServiceStatus{
					Address: &knativeV1.Addressable{
						URL: &apis.URL{
							Scheme: "http",
							Host:   "sklearn-predictor.default.svc.cluster.local",
						},
					},
				},
			},
			expectedUrl: "http://sklearn-predictor.default.svc.cluster.local/v2/models/sklearn-custom/infer",
			expectedErr: gomega.BeNil(),
		},
		"IsvcIsNotReady": {
			isvc: InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sklearn",
				},
				Spec: InferenceServiceSpec{
					Predictor: PredictorSpec{
						Model: &ModelSpec{
							ModelFormat: ModelFormat{
								Name: "sklearn",
							},
							PredictorExtensionSpec: PredictorExtensionSpec{
								StorageURI: proto.String("s3://test"),
							},
						},
					},
					Transformer: &TransformerSpec{
						PodSpec: PodSpec{
							Containers: []v1.Container{
								{
									Name:  constants.InferenceServiceContainerName,
									Image: "kserve/transformer:1.0",
									Env: []v1.EnvVar{
										{
											Name:  constants.CustomSpecProtocolEnvVarKey,
											Value: string(constants.ProtocolV2),
										},
									},
								},
							},
						},
					},
				},
				Status: InferenceServiceStatus{
					Address: &knativeV1.Addressable{
						URL: nil,
					},
				},
			},
			expectedUrl: "",
			expectedErr: gomega.MatchError("service sklearn is not ready"),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res, err := GetPredictorEndpoint(&scenario.isvc)
			g.Expect(err).To(scenario.expectedErr)
			if !g.Expect(res).To(gomega.Equal(scenario.expectedUrl)) {
				t.Errorf("got %s, want %s", res, scenario.expectedUrl)
			}
		})
	}
}

func TestValidateStorageURIForDefaultStorageInitializer(t *testing.T) {
	validUris := []string{
		"https://kfserving.blob.core.windows.net/triton/simple_string/",
		"https://kfserving.blob.core.windows.net/triton/simple_string",
		"https://kfserving.blob.core.windows.net/triton/",
		"https://kfserving.blob.core.windows.net/triton",
		"https://raw.githubusercontent.com/someOrg/someRepo/model.tar.gz",
		"http://raw.githubusercontent.com/someOrg/someRepo/model.tar.gz",
		"hdfs://",
		"webhdfs://",
		"some/relative/path",
		"/",
		"foo",
		"",
	}
	s := runtime.NewScheme()
	err := v1alpha1.AddToScheme(s)
	if err != nil {
		t.Errorf("Failed to add v1alpha1 to scheme %s", err)
	}
	mockClient := fake.NewClientBuilder().WithScheme(s).Build()
	for _, uri := range validUris {
		if err := ValidateStorageURI(&uri, mockClient); err != nil {
			t.Errorf("%q validation failed: %s", uri, err)
		}
	}
}

func TestValidateStorageURIForCustomPrefix(t *testing.T) {
	invalidUris := []string{
		"custom://custom.com/model",
	}
	s := runtime.NewScheme()
	err := v1alpha1.AddToScheme(s)
	if err != nil {
		t.Errorf("Failed to add v1alpha1 to scheme %s", err)
	}
	mockClient := fake.NewClientBuilder().WithScheme(s).Build()
	for _, uri := range invalidUris {
		if err := ValidateStorageURI(&uri, mockClient); err == nil {
			t.Errorf("%q validation failed: error expected", uri)
		}
	}
}

func TestValidateStorageURIForDefaultStorageInitializerCRD(t *testing.T) {
	customSpec := v1alpha1.ClusterStorageContainer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "custom",
		},
		Spec: v1alpha1.StorageContainerSpec{
			Container: v1.Container{
				Image: "kserve/storage-initializer:latest",
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceMemory: resource.MustParse("200Mi"),
					},
				},
			},
			SupportedUriFormats: []v1alpha1.SupportedUriFormat{{Prefix: "custom://"}},
			WorkloadType:        v1alpha1.InitContainer,
		},
	}

	storageContainerSpecs := &v1alpha1.ClusterStorageContainerList{
		Items: []v1alpha1.ClusterStorageContainer{customSpec},
	}
	validUris := []string{
		"custom://custom.com/model/",
	}
	s := runtime.NewScheme()
	err := v1alpha1.AddToScheme(s)
	if err != nil {
		t.Errorf("Failed to add v1alpha1 to scheme %s", err)
	}
	mockClient := fake.NewClientBuilder().WithLists(storageContainerSpecs).WithScheme(s).Build()
	for _, uri := range validUris {
		if err := ValidateStorageURI(&uri, mockClient); err != nil {
			t.Errorf("%q validation failed: %s", uri, err)
		}
	}
}
