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
package pod

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAcceleratorInjector(t *testing.T) {
	scenarios := map[string]struct {
		original *v1.Pod
		expected *v1.Pod
	}{
		"AddGPUSelector": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
					Annotations: map[string]string{
						constants.InferenceServiceGKEAcceleratorAnnotationKey: "nvidia-tesla-v100",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Resources: v1.ResourceRequirements{
							Limits: v1.ResourceList{constants.NvidiaGPUResourceType: resource.MustParse("1")},
						},
					}},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
					Annotations: map[string]string{
						constants.InferenceServiceGKEAcceleratorAnnotationKey: "nvidia-tesla-v100",
					},
				},
				Spec: v1.PodSpec{
					NodeSelector: map[string]string{
						GkeAcceleratorNodeSelector: "nvidia-tesla-v100",
					},
					Containers: []v1.Container{{
						Resources: v1.ResourceRequirements{
							Limits: v1.ResourceList{constants.NvidiaGPUResourceType: resource.MustParse("1")},
						},
					}},
				},
			},
		},
		"DoNotAddGPUSelector": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Resources: v1.ResourceRequirements{
							Limits: v1.ResourceList{constants.NvidiaGPUResourceType: resource.MustParse("1")},
						},
					}},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Resources: v1.ResourceRequirements{
							Limits: v1.ResourceList{constants.NvidiaGPUResourceType: resource.MustParse("1")},
						},
					}},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		InjectGKEAcceleratorSelector(scenario.original)
		// cmd.Diff complains on ResourceList when Nvidia is key. Objects are explicitly compared
		if diff := cmp.Diff(
			scenario.expected.Spec.NodeSelector,
			scenario.original.Spec.NodeSelector,
		); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
		if diff := cmp.Diff(
			scenario.expected.Spec.Tolerations,
			scenario.original.Spec.Tolerations,
		); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}
