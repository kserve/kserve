/*
Copyright 2020 kubeflow.org.

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
	"github.com/kubeflow/kfserving/pkg/credentials"
	"k8s.io/apimachinery/pkg/api/resource"
	"knative.dev/pkg/kmp"
	"testing"

	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	agentConfig = &AgentConfig{
		Image:         "gcr.io/kfserving/agent:latest",
		CpuRequest:    AgentDefaultCPURequest,
		CpuLimit:      AgentDefaultCPULimit,
		MemoryRequest: AgentDefaultMemoryRequest,
		MemoryLimit:   AgentDefaultMemoryLimit,
	}

	agentResourceRequirement = v1.ResourceRequirements{
		Limits: map[v1.ResourceName]resource.Quantity{
			v1.ResourceCPU:    resource.MustParse(BatcherDefaultCPULimit),
			v1.ResourceMemory: resource.MustParse(BatcherDefaultMemoryLimit),
		},
		Requests: map[v1.ResourceName]resource.Quantity{
			v1.ResourceCPU:    resource.MustParse(BatcherDefaultCPURequest),
			v1.ResourceMemory: resource.MustParse(BatcherDefaultMemoryRequest),
		},
	}
)

func TestAgentInjector(t *testing.T) {
	scenarios := map[string]struct {
		original *v1.Pod
		expected *v1.Pod
	}{
		"AddAgent": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AgentShouldInjectAnnotationKey:          "true",
						constants.AgentModelConfigVolumeNameAnnotationKey: "modelconfig-deployment-0",
						constants.AgentModelDirAnnotationKey:              "/mnt/models",
						constants.AgentModelConfigMountPathAnnotationKey:  "/mnt/configs",
					},
					Labels: map[string]string{
						"serving.kubeflow.org/inferenceservice": "sklearn",
						constants.KServiceModelLabel:            "sklearn",
						constants.KServiceEndpointLabel:         "default",
						constants.KServiceComponentLabel:        "predictor",
					},
				},
				Spec: v1.PodSpec{
					ServiceAccountName: "sa",
					Containers: []v1.Container{{
						Name: "sklearn",
					}},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
					Annotations: map[string]string{
						constants.AgentShouldInjectAnnotationKey: "true",
					},
				},
				Spec: v1.PodSpec{
					ServiceAccountName: "sa",
					Containers: []v1.Container{
						{
							Name: "sklearn",
						},
						{
							Name:      constants.AgentContainerName,
							Image:     agentConfig.Image,
							Resources: agentResourceRequirement,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      constants.ModelDirVolumeName,
									ReadOnly:  false,
									MountPath: constants.ModelDir,
								},
								{
									Name:      constants.ModelConfigVolumeName,
									ReadOnly:  false,
									MountPath: constants.ModelConfigDir,
								},
							},
							Args: []string{"-config-dir", "/mnt/configs", "-model-dir", "/mnt/models"},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "model-dir",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "model-config",
							VolumeSource: v1.VolumeSource{
								ConfigMap: &v1.ConfigMapVolumeSource{
									LocalObjectReference: v1.LocalObjectReference{
										Name: "modelconfig-deployment-0",
									},
								},
							},
						},
					},
				},
			},
		},
		"DoNotAddAgent": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					}},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name: "sklearn",
					}},
				},
			},
		},
	}

	credentialBuilder := credentials.NewCredentialBulder(c, &v1.ConfigMap{
		Data: map[string]string{},
	})

	for name, scenario := range scenarios {
		injector := &AgentInjector{
			credentialBuilder,
			agentConfig,
		}
		injector.InjectAgent(scenario.original)
		if diff, _ := kmp.SafeDiff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}
