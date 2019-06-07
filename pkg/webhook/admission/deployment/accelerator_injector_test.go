/*
Copyright 2019 kubeflow.org.

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
package deployment

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/kubeflow/kfserving/pkg/constants"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAcceleratorInjector(t *testing.T) {
	scenarios := map[string]struct {
		original *appsv1.Deployment
		expected *appsv1.Deployment
	}{
		"AddGPUSelector": {
			original: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
					Annotations: map[string]string{
						KFServingGkeAcceleratorAnnotation: "nvidia-tesla-v100",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{{
								Resources: v1.ResourceRequirements{
									Limits: v1.ResourceList{constants.NvidiaGPUResourceType: resource.MustParse("1")},
								},
							}},
						},
					},
				},
			},
			expected: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
					Annotations: map[string]string{
						KFServingGkeAcceleratorAnnotation: "nvidia-tesla-v100",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							NodeSelector: map[string]string{
								GkeAcceleratorNodeSelector: "nvidia-tesla-v100",
							},
							Containers: []v1.Container{{
								Resources: v1.ResourceRequirements{
									Limits: v1.ResourceList{constants.NvidiaGPUResourceType: resource.MustParse("1")},
								},
							}},
							Tolerations: []v1.Toleration{v1.Toleration{
								Key:      constants.NvidiaGPUResourceType,
								Value:    NvidiaGPUTaintValue,
								Operator: v1.TolerationOpEqual,
								Effect:   v1.TaintEffectPreferNoSchedule,
							}},
						},
					},
				},
			},
		},
		"DoNotAddGPUSelector": {
			original: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{{
								Resources: v1.ResourceRequirements{
									Limits: v1.ResourceList{constants.NvidiaGPUResourceType: resource.MustParse("1")},
								},
							}},
						},
					},
				},
			},
			expected: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{{
								Resources: v1.ResourceRequirements{
									Limits: v1.ResourceList{constants.NvidiaGPUResourceType: resource.MustParse("1")},
								},
							}},
							Tolerations: []v1.Toleration{v1.Toleration{
								Key:      constants.NvidiaGPUResourceType,
								Value:    NvidiaGPUTaintValue,
								Operator: v1.TolerationOpEqual,
								Effect:   v1.TaintEffectPreferNoSchedule,
							}},
						},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		Mutate(scenario.original)
		// cmd.Diff complains on ResourceList when Nvidia is key. Objects are explicitly compared
		if diff := cmp.Diff(
			scenario.expected.Spec.Template.Spec.NodeSelector,
			scenario.original.Spec.Template.Spec.NodeSelector,
		); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
		if diff := cmp.Diff(
			scenario.expected.Spec.Template.Spec.Tolerations,
			scenario.original.Spec.Template.Spec.Tolerations,
		); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}

	}
}
