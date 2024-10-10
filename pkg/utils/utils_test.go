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
	"errors"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials/gcs"
)

func TestFilterUtil(t *testing.T) {
	scenarios := map[string]struct {
		input     map[string]string
		predicate func(string) bool
		expected  map[string]string
	}{
		"TruthyFilter": {
			input:     map[string]string{"key1": "val1", "key2": "val2"},
			predicate: func(key string) bool { return true },
			expected:  map[string]string{"key1": "val1", "key2": "val2"},
		},
		"FalsyFilter": {
			input:     map[string]string{"key1": "val1", "key2": "val2"},
			predicate: func(key string) bool { return false },
			expected:  map[string]string{},
		},
	}
	for name, scenario := range scenarios {
		result := Filter(scenario.input, scenario.predicate)

		if diff := cmp.Diff(scenario.expected, result); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}

func TestUnionUtil(t *testing.T) {
	scenarios := map[string]struct {
		input1   map[string]string
		input2   map[string]string
		expected map[string]string
	}{
		"UnionTwoMaps": {
			input1: map[string]string{"serving.kserve.io/service": "mnist",
				"label1": "value1"},
			input2: map[string]string{"service.knative.dev/service": "mnist",
				"label2": "value2"},
			expected: map[string]string{"serving.kserve.io/service": "mnist",
				"label1": "value1", "service.knative.dev/service": "mnist", "label2": "value2"},
		},
		"UnionTwoMapsOverwritten": {
			input1: map[string]string{"serving.kserve.io/service": "mnist",
				"label1": "value1", "label3": "value1"},
			input2: map[string]string{"service.knative.dev/service": "mnist",
				"label2": "value2", "label3": "value3"},
			expected: map[string]string{"serving.kserve.io/service": "mnist",
				"label1": "value1", "service.knative.dev/service": "mnist", "label2": "value2", "label3": "value3"},
		},
		"UnionWithEmptyMap": {
			input1: map[string]string{},
			input2: map[string]string{"service.knative.dev/service": "mnist",
				"label2": "value2"},
			expected: map[string]string{"service.knative.dev/service": "mnist", "label2": "value2"},
		},
		"UnionWithNilMap": {
			input1: nil,
			input2: map[string]string{"service.knative.dev/service": "mnist",
				"label2": "value2"},
			expected: map[string]string{"service.knative.dev/service": "mnist", "label2": "value2"},
		},
		"UnionNilMaps": {
			input1:   nil,
			input2:   nil,
			expected: map[string]string{},
		},
	}
	for name, scenario := range scenarios {
		result := Union(scenario.input1, scenario.input2)

		if diff := cmp.Diff(scenario.expected, result); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}

func TestContainsUtil(t *testing.T) {
	scenarios := map[string]struct {
		input1   []string
		input2   string
		expected bool
	}{
		"SliceContainsString": {
			input1:   []string{"hey", "hello"},
			input2:   "hey",
			expected: true,
		},
		"SliceDoesNotContainString": {
			input1:   []string{"hey", "hello"},
			input2:   "he",
			expected: false,
		},
		"SliceIsEmpty": {
			input1:   []string{},
			input2:   "hey",
			expected: false,
		},
	}
	for name, scenario := range scenarios {
		result := Includes(scenario.input1, scenario.input2)
		if diff := cmp.Diff(scenario.expected, result); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}

func TestAppendVolumeIfNotExists(t *testing.T) {
	scenarios := map[string]struct {
		volumes         []v1.Volume
		volume          v1.Volume
		expectedVolumes []v1.Volume
	}{
		"DuplicateVolume": {
			volumes: []v1.Volume{
				{
					Name: gcs.GCSCredentialVolumeName,
					VolumeSource: v1.VolumeSource{
						Secret: &v1.SecretVolumeSource{
							SecretName: "user-gcp-sa",
						},
					},
				},
				{
					Name: "blue",
					VolumeSource: v1.VolumeSource{
						Secret: &v1.SecretVolumeSource{
							SecretName: "user-gcp-sa",
						},
					},
				},
			},
			volume: v1.Volume{
				Name: gcs.GCSCredentialVolumeName,
				VolumeSource: v1.VolumeSource{
					Secret: &v1.SecretVolumeSource{
						SecretName: "user-gcp-sa",
					},
				},
			},
			expectedVolumes: []v1.Volume{
				{
					Name: gcs.GCSCredentialVolumeName,
					VolumeSource: v1.VolumeSource{
						Secret: &v1.SecretVolumeSource{
							SecretName: "user-gcp-sa",
						},
					},
				},
				{
					Name: "blue",
					VolumeSource: v1.VolumeSource{
						Secret: &v1.SecretVolumeSource{
							SecretName: "user-gcp-sa",
						},
					},
				},
			},
		},
		"NotDuplicateVolume": {
			volumes: []v1.Volume{
				{
					Name: gcs.GCSCredentialVolumeName,
					VolumeSource: v1.VolumeSource{
						Secret: &v1.SecretVolumeSource{
							SecretName: "user-gcp-sa",
						},
					},
				},
				{
					Name: "blue",
					VolumeSource: v1.VolumeSource{
						Secret: &v1.SecretVolumeSource{
							SecretName: "user-gcp-sa",
						},
					},
				},
			},
			volume: v1.Volume{
				Name: "green",
				VolumeSource: v1.VolumeSource{
					Secret: &v1.SecretVolumeSource{
						SecretName: "user-gcp-sa",
					},
				},
			},
			expectedVolumes: []v1.Volume{
				{
					Name: gcs.GCSCredentialVolumeName,
					VolumeSource: v1.VolumeSource{
						Secret: &v1.SecretVolumeSource{
							SecretName: "user-gcp-sa",
						},
					},
				},
				{
					Name: "blue",
					VolumeSource: v1.VolumeSource{
						Secret: &v1.SecretVolumeSource{
							SecretName: "user-gcp-sa",
						},
					},
				},
				{
					Name: "green",
					VolumeSource: v1.VolumeSource{
						Secret: &v1.SecretVolumeSource{
							SecretName: "user-gcp-sa",
						},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		volumes := AppendVolumeIfNotExists(scenario.volumes, scenario.volume)

		if diff := cmp.Diff(scenario.expectedVolumes, volumes); diff != "" {
			t.Errorf("Test %q unexpected volume (-want +got): %v", name, diff)
		}
	}
}

func TestMergeEnvs(t *testing.T) {
	scenarios := map[string]struct {
		baseEnvs     []v1.EnvVar
		overrideEnvs []v1.EnvVar
		expectedEnvs []v1.EnvVar
	}{
		"EmptyOverrides": {
			baseEnvs: []v1.EnvVar{
				{
					Name:  "name1",
					Value: "value1",
				},
			},
			overrideEnvs: []v1.EnvVar{},
			expectedEnvs: []v1.EnvVar{
				{
					Name:  "name1",
					Value: "value1",
				},
			},
		},
		"EmptyBase": {
			baseEnvs: []v1.EnvVar{},
			overrideEnvs: []v1.EnvVar{
				{
					Name:  "name1",
					Value: "value1",
				},
			},
			expectedEnvs: []v1.EnvVar{
				{
					Name:  "name1",
					Value: "value1",
				},
			},
		},
		"NoOverlap": {
			baseEnvs: []v1.EnvVar{
				{
					Name:  "name1",
					Value: "value1",
				},
			},
			overrideEnvs: []v1.EnvVar{
				{
					Name:  "name2",
					Value: "value2",
				},
			},
			expectedEnvs: []v1.EnvVar{
				{
					Name:  "name1",
					Value: "value1",
				},
				{
					Name:  "name2",
					Value: "value2",
				},
			},
		},
		"SingleOverlap": {
			baseEnvs: []v1.EnvVar{
				{
					Name:  "name1",
					Value: "value1",
				},
			},
			overrideEnvs: []v1.EnvVar{
				{
					Name:  "name1",
					Value: "value2",
				},
			},
			expectedEnvs: []v1.EnvVar{
				{
					Name:  "name1",
					Value: "value2",
				},
			},
		},
		"MultiOverlap": {
			baseEnvs: []v1.EnvVar{
				{
					Name:  "name1",
					Value: "value1",
				},
				{
					Name:  "name2",
					Value: "value2",
				},
				{
					Name:  "name3",
					Value: "value3",
				},
			},
			overrideEnvs: []v1.EnvVar{
				{
					Name:  "name1",
					Value: "value3",
				},
				{
					Name:  "name3",
					Value: "value1",
				},
				{
					Name:  "name4",
					Value: "value4",
				},
			},
			expectedEnvs: []v1.EnvVar{
				{
					Name:  "name1",
					Value: "value3",
				},
				{
					Name:  "name2",
					Value: "value2",
				},
				{
					Name:  "name3",
					Value: "value1",
				},
				{
					Name:  "name4",
					Value: "value4",
				},
			},
		},
	}

	for name, scenario := range scenarios {
		envs := MergeEnvs(scenario.baseEnvs, scenario.overrideEnvs)

		if diff := cmp.Diff(scenario.expectedEnvs, envs); diff != "" {
			t.Errorf("Test %q unexpected envs (-want +got): %v", name, diff)
		}
	}
}

func TestIncludesArg(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	args := []string{
		constants.ArgumentModelName,
	}
	scenarios := map[string]struct {
		arg      string
		expected bool
	}{
		"SliceContainsArg": {
			arg:      constants.ArgumentModelName,
			expected: true,
		},
		"SliceNotContainsArg": {
			arg:      "NoArg",
			expected: false,
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res := IncludesArg(args, scenario.arg)
			g.Expect(res).To(gomega.Equal(scenario.expected))
		})
	}
}

func TestIsGpuEnabled(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		resource v1.ResourceRequirements
		expected bool
	}{
		"GpuEnabled": {
			resource: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					"cpu": resource.Quantity{
						Format: "100",
					},
					constants.NvidiaGPUResourceType: resource.MustParse("1"),
				},
				Requests: v1.ResourceList{
					"cpu": resource.Quantity{
						Format: "90",
					},
					constants.NvidiaGPUResourceType: resource.MustParse("1"),
				},
			},
			expected: true,
		},
		"GPUDisabled": {
			resource: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					"cpu": resource.Quantity{
						Format: "100",
					},
				},
				Requests: v1.ResourceList{
					"cpu": resource.Quantity{
						Format: "90",
					},
				},
			},
			expected: false,
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res := IsGPUEnabled(scenario.resource)
			g.Expect(res).To(gomega.Equal(scenario.expected))
		})
	}
}

func TestFirstNonNilError(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		errors  []error
		matcher types.GomegaMatcher
	}{
		"NoNonNilError": {
			errors: []error{
				nil,
				nil,
			},
			matcher: gomega.BeNil(),
		},
		"ContainsError": {
			errors: []error{
				nil,
				errors.New("First non nil error"),
				errors.New("Second non nil error"),
			},
			matcher: gomega.Equal(errors.New("First non nil error")),
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			err := FirstNonNilError(scenario.errors)
			g.Expect(err).Should(scenario.matcher)
		})
	}
}

func TestRemoveString(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	testStrings := []string{
		"Model Tensorflow",
		"SKLearn Model",
		"Model",
		"ModelPytorch",
	}
	expected := []string{
		"Model Tensorflow",
		"SKLearn Model",
		"ModelPytorch",
	}
	res := RemoveString(testStrings, "Model")
	g.Expect(res).Should(gomega.Equal(expected))
}

func TestIsPrefixSupported(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	prefixes := []string{
		"S3://",
		"GCS://",
		"HTTP://",
		"HTTPS://",
	}
	scenarios := map[string]struct {
		input    string
		expected bool
	}{
		"SupportedPrefix": {
			input:    "GCS://test/model",
			expected: true,
		},
		"UnSupportedPreifx": {
			input:    "PVC://test/model",
			expected: false,
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res := IsPrefixSupported(scenario.input, prefixes)
			g.Expect(res).Should(gomega.Equal(scenario.expected))
		})
	}
}

func TestGetEnvVarValue(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		envList          []v1.EnvVar
		targetEnvName    string
		expectedEnvValue string
		expectedExist    bool
	}{
		"EnvExist": {
			envList: []v1.EnvVar{
				{Name: "test-name", Value: "test-value"},
			},
			targetEnvName:    "test-name",
			expectedEnvValue: "test-value",
			expectedExist:    true,
		},
		"EnvDoesNotExist": {
			envList: []v1.EnvVar{
				{Name: "test-name", Value: "test-value"},
			},
			targetEnvName:    "wrong",
			expectedEnvValue: "",
			expectedExist:    false,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			res, exists := GetEnvVarValue(scenario.envList, scenario.targetEnvName)
			g.Expect(res).Should(gomega.Equal(scenario.expectedEnvValue))
			g.Expect(exists).Should(gomega.Equal(scenario.expectedExist))
		})
	}
}

func TestIsUnknownGpuResourceType(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		resources       v1.ResourceRequirements
		expectedUnknown bool
	}{
		"OnlyBasicResources": {
			resources: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("1"),
					v1.ResourceMemory: resource.MustParse("1Gi"),
				},
				Requests: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("1"),
					v1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			expectedUnknown: false,
		},
		"ValidGpuResource": {
			resources: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceCPU:                    resource.MustParse("1"),
					v1.ResourceMemory:                 resource.MustParse("1Gi"),
					v1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
				},
				Requests: v1.ResourceList{
					v1.ResourceCPU:                    resource.MustParse("1"),
					v1.ResourceMemory:                 resource.MustParse("1Gi"),
					v1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
				},
			},
			expectedUnknown: false,
		},
		"UnknownGpuResource": {
			resources: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceCPU:                     resource.MustParse("1"),
					v1.ResourceMemory:                  resource.MustParse("1Gi"),
					v1.ResourceName("unknown.com/gpu"): resource.MustParse("1"),
				},
				Requests: v1.ResourceList{
					v1.ResourceCPU:                     resource.MustParse("1"),
					v1.ResourceMemory:                  resource.MustParse("1Gi"),
					v1.ResourceName("unknown.com/gpu"): resource.MustParse("1"),
				},
			},
			expectedUnknown: true,
		},
		"MixedResources": {
			resources: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceCPU:                    resource.MustParse("1"),
					v1.ResourceMemory:                 resource.MustParse("1Gi"),
					v1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
				},
				Requests: v1.ResourceList{
					v1.ResourceCPU:                     resource.MustParse("1"),
					v1.ResourceMemory:                  resource.MustParse("1Gi"),
					v1.ResourceName("unknown.com/gpu"): resource.MustParse("1"),
				},
			},
			expectedUnknown: true,
		},
		"EmptyResources": {
			resources: v1.ResourceRequirements{
				Limits:   v1.ResourceList{},
				Requests: v1.ResourceList{},
			},
			expectedUnknown: false,
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			result := IsUnknownGpuResourceType(scenario.resources, "")
			g.Expect(result).Should(gomega.Equal(scenario.expectedUnknown))
		})
	}
}

func TestIsValidCustomGPUArray(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"[]", false},
		{"[\"item1\", \"item2\"]", true},
		{"[\"item1\", \"item2\", \"item3\"]", true},
		{"[\"item1\", \"item2\", \"\"]", false},
		{"[\"item1\", 42]", false},
		{"[\"item1\", \"item2\",]", false},
		{"[\"item1\", \"item2\", \"item3\"", false},
		{"[item1, item2]", false},
		{"[\"item1\", \"item2\" \"item3\"]", false},
		{"[\"item1\", null]", false},
		{"[\"item1\", true]", false},
		{"[\"item1\", false]", false},
		{"[\"item1\", \"item2\", 42]", false},
		{"[\"item1\", \"item2\", \"item3\", \"\"]", false},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := IsValidCustomGPUArray(test.input)
			if result != test.expected {
				t.Errorf("expected %v, got %v", test.expected, result)
			}
		})
	}
}

func TestAppendEnvVarIfNotExists(t *testing.T) {
	scenarios := map[string]struct {
		initialEnvs  []v1.EnvVar
		newEnvs      []v1.EnvVar
		expectedEnvs []v1.EnvVar
	}{
		"EnvVarAlreadyExists": {
			initialEnvs: []v1.EnvVar{
				{Name: "ENV1", Value: "value1"},
				{Name: "ENV2", Value: "value2"},
			},
			newEnvs: []v1.EnvVar{
				{Name: "ENV1", Value: "new_value1"},
			},
			expectedEnvs: []v1.EnvVar{
				{Name: "ENV1", Value: "value1"},
				{Name: "ENV2", Value: "value2"},
			},
		},
		"EnvVarDoesNotExist": {
			initialEnvs: []v1.EnvVar{
				{Name: "ENV1", Value: "value1"},
			},
			newEnvs: []v1.EnvVar{
				{Name: "ENV2", Value: "value2"},
			},
			expectedEnvs: []v1.EnvVar{
				{Name: "ENV1", Value: "value1"},
				{Name: "ENV2", Value: "value2"},
			},
		},
		"MultipleNewEnvVars": {
			initialEnvs: []v1.EnvVar{
				{Name: "ENV1", Value: "value1"},
			},
			newEnvs: []v1.EnvVar{
				{Name: "ENV2", Value: "value2"},
				{Name: "ENV3", Value: "value3"},
			},
			expectedEnvs: []v1.EnvVar{
				{Name: "ENV1", Value: "value1"},
				{Name: "ENV2", Value: "value2"},
				{Name: "ENV3", Value: "value3"},
			},
		},
		"NoInitialEnvVars": {
			initialEnvs: []v1.EnvVar{},
			newEnvs: []v1.EnvVar{
				{Name: "ENV1", Value: "value1"},
			},
			expectedEnvs: []v1.EnvVar{
				{Name: "ENV1", Value: "value1"},
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			result := AppendEnvVarIfNotExists(scenario.initialEnvs, scenario.newEnvs...)
			if diff := cmp.Diff(scenario.expectedEnvs, result); diff != "" {
				t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
			}
		})
	}
}

func TestAppendPortIfNotExists(t *testing.T) {
	scenarios := map[string]struct {
		initialPorts  []v1.ContainerPort
		newPorts      []v1.ContainerPort
		expectedPorts []v1.ContainerPort
	}{
		"PortAlreadyExists": {
			initialPorts: []v1.ContainerPort{
				{Name: "port1", ContainerPort: 8080},
				{Name: "port2", ContainerPort: 9090},
			},
			newPorts: []v1.ContainerPort{
				{Name: "port1", ContainerPort: 8081},
			},
			expectedPorts: []v1.ContainerPort{
				{Name: "port1", ContainerPort: 8080},
				{Name: "port2", ContainerPort: 9090},
			},
		},
		"PortDoesNotExist": {
			initialPorts: []v1.ContainerPort{
				{Name: "port1", ContainerPort: 8080},
			},
			newPorts: []v1.ContainerPort{
				{Name: "port2", ContainerPort: 9090},
			},
			expectedPorts: []v1.ContainerPort{
				{Name: "port1", ContainerPort: 8080},
				{Name: "port2", ContainerPort: 9090},
			},
		},
		"MultipleNewPorts": {
			initialPorts: []v1.ContainerPort{
				{Name: "port1", ContainerPort: 8080},
			},
			newPorts: []v1.ContainerPort{
				{Name: "port2", ContainerPort: 9090},
				{Name: "port3", ContainerPort: 10090},
			},
			expectedPorts: []v1.ContainerPort{
				{Name: "port1", ContainerPort: 8080},
				{Name: "port2", ContainerPort: 9090},
				{Name: "port3", ContainerPort: 10090},
			},
		},
		"NoInitialPorts": {
			initialPorts: []v1.ContainerPort{},
			newPorts: []v1.ContainerPort{
				{Name: "port1", ContainerPort: 8080},
			},
			expectedPorts: []v1.ContainerPort{
				{Name: "port1", ContainerPort: 8080},
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			result := AppendPortIfNotExists(scenario.initialPorts, scenario.newPorts...)
			if diff := cmp.Diff(scenario.expectedPorts, result); diff != "" {
				t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
			}
		})
	}
}

func TestSetAvailableResourcesForApi(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		groupVersion string
		resources    *metav1.APIResourceList
	}{
		"SetResourcesInEmptyCache": {
			groupVersion: "v1",
			resources: &metav1.APIResourceList{
				GroupVersion: "v1",
				APIResources: []metav1.APIResource{
					{Name: "pods", Kind: "Pod"},
				},
			},
		},
		"SetResourcesInNonEmptyCache": {
			groupVersion: "v1",
			resources: &metav1.APIResourceList{
				GroupVersion: "v1",
				APIResources: []metav1.APIResource{
					{Name: "services", Kind: "Service"},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			// Reset the cache before each test
			gvResourcesCache = nil

			SetAvailableResourcesForApi(scenario.groupVersion, scenario.resources)

			g.Expect(gvResourcesCache).ToNot(gomega.BeNil())
			g.Expect(gvResourcesCache[scenario.groupVersion]).To(gomega.BeComparableTo(scenario.resources))
		})
	}
}

func TestStringToInt32(t *testing.T) {
	scenarios := map[string]struct {
		input    string
		expected int32
		err      error
	}{
		"ValidInt32": {
			input:    "123",
			expected: 123,
			err:      nil,
		},
		"ValidNegativeInt32": {
			input:    "-123",
			expected: -123,
			err:      nil,
		},
		"ExceedsInt32Limit": {
			input:    "2147483648", // int32 max is 2147483647
			expected: 0,
			err:      &strconv.NumError{Func: "ParseInt", Num: "2147483648", Err: strconv.ErrRange},
		},
		"InvalidNumber": {
			input:    "abc",
			expected: 0,
			err:      &strconv.NumError{Func: "ParseInt", Num: "abc", Err: strconv.ErrSyntax},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			result, err := StringToInt32(scenario.input)
			if result != scenario.expected {
				t.Errorf("Test %q unexpected result: got (%d), want (%d)", name, result, scenario.expected)
			}
			if err != nil && scenario.err != nil {
				if err.Error() != scenario.err.Error() {
					t.Errorf("Test %q unexpected error: got (%v), want (%v)", name, err, scenario.err)
				}
			} else if !errors.Is(err, scenario.err) {
				t.Errorf("got %v, want %v", err, scenario.err)
			}
		})
	}
}
