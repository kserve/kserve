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

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/credentials"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"

	"knative.dev/pkg/kmp"

	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	AgentDefaultCPURequest    = "100m"
	AgentDefaultCPULimit      = "1"
	AgentDefaultMemoryRequest = "200Mi"
	AgentDefaultMemoryLimit   = "1Gi"
)

var (
	agentConfig = &AgentConfig{
		Image:         "gcr.io/kfserving/agent:latest",
		CpuRequest:    AgentDefaultCPURequest,
		CpuLimit:      AgentDefaultCPULimit,
		MemoryRequest: AgentDefaultMemoryRequest,
		MemoryLimit:   AgentDefaultMemoryLimit,
	}

	loggerConfig = &LoggerConfig{
		Image: "gcr.io/kfserving/agent:latest",
	}
	batcherTestConfig = &BatcherConfig{
		Image: "gcr.io/kfserving/batcher:latest",
	}
	agentResourceRequirement = v1.ResourceRequirements{
		Limits: map[v1.ResourceName]resource.Quantity{
			v1.ResourceCPU:    resource.MustParse(AgentDefaultCPULimit),
			v1.ResourceMemory: resource.MustParse(AgentDefaultMemoryLimit),
		},
		Requests: map[v1.ResourceName]resource.Quantity{
			v1.ResourceCPU:    resource.MustParse(AgentDefaultCPURequest),
			v1.ResourceMemory: resource.MustParse(AgentDefaultMemoryRequest),
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
						"serving.kserve.io/inferenceservice": "sklearn",
						constants.KServiceModelLabel:         "sklearn",
						constants.KServiceEndpointLabel:      "default",
						constants.KServiceComponentLabel:     "predictor",
					},
				},
				Spec: v1.PodSpec{
					ServiceAccountName: "sa",
					Containers: []v1.Container{
						{
							Name: "sklearn",
							ReadinessProbe: &v1.Probe{
								Handler: v1.Handler{
									TCPSocket: &v1.TCPSocketAction{
										Port: intstr.IntOrString{
											IntVal: 8080,
										},
									},
								},
								InitialDelaySeconds: 0,
								TimeoutSeconds:      1,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
						},
					},
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
							ReadinessProbe: &v1.Probe{
								Handler: v1.Handler{
									TCPSocket: &v1.TCPSocketAction{
										Port: intstr.IntOrString{
											IntVal: 8080,
										},
									},
								},
								InitialDelaySeconds: 0,
								TimeoutSeconds:      1,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
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
							Args: []string{"--enable-puller", "--config-dir", "/mnt/configs", "--model-dir", "/mnt/models"},
							Ports: []v1.ContainerPort{
								{
									Name:          "agent-port",
									ContainerPort: constants.InferenceServiceDefaultAgentPort,
									Protocol:      "TCP",
								},
							},
							Env: []v1.EnvVar{{Name: "SERVING_READINESS_PROBE", Value: "{\"tcpSocket\":{\"port\":8080},\"timeoutSeconds\":1,\"periodSeconds\":10,\"successThreshold\":1,\"failureThreshold\":3}"}},
							ReadinessProbe: &v1.Probe{
								Handler: v1.Handler{
									HTTPGet: &v1.HTTPGetAction{
										HTTPHeaders: []v1.HTTPHeader{
											{
												Name:  "K-Network-Probe",
												Value: "queue",
											},
										},
										Port:   intstr.FromInt(9081),
										Path:   "/",
										Scheme: "HTTP",
									},
								},
							},
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
		"AddLogger": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.LoggerInternalAnnotationKey:        "true",
						constants.LoggerSinkUrlInternalAnnotationKey: "http://httpbin.org/",
						constants.LoggerModeInternalAnnotationKey:    string(v1beta1.LogAll),
					},
					Labels: map[string]string{
						"serving.kserve.io/inferenceservice": "sklearn",
						constants.KServiceModelLabel:         "sklearn",
						constants.KServiceEndpointLabel:      "default",
						constants.KServiceComponentLabel:     "predictor",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "sklearn",
							ReadinessProbe: &v1.Probe{
								Handler: v1.Handler{
									TCPSocket: &v1.TCPSocketAction{
										Port: intstr.IntOrString{
											IntVal: 8080,
										},
									},
								},
								InitialDelaySeconds: 0,
								TimeoutSeconds:      1,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
						},
						{
							Name: "queue-proxy",
							Env:  []v1.EnvVar{{Name: "SERVING_READINESS_PROBE", Value: "{\"tcpSocket\":{\"port\":8080},\"timeoutSeconds\":1,\"periodSeconds\":10,\"successThreshold\":1,\"failureThreshold\":3}"}},
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment",
					Annotations: map[string]string{
						constants.LoggerInternalAnnotationKey:        "true",
						constants.LoggerSinkUrlInternalAnnotationKey: "http://httpbin.org/",
						constants.LoggerModeInternalAnnotationKey:    string(v1beta1.LogAll),
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "sklearn",
							ReadinessProbe: &v1.Probe{
								Handler: v1.Handler{
									TCPSocket: &v1.TCPSocketAction{
										Port: intstr.IntOrString{
											IntVal: 8080,
										},
									},
								},
								InitialDelaySeconds: 0,
								TimeoutSeconds:      1,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
						},
						{
							Name: "queue-proxy",
							Env:  []v1.EnvVar{{Name: "SERVING_READINESS_PROBE", Value: "{\"tcpSocket\":{\"port\":8080},\"timeoutSeconds\":1,\"periodSeconds\":10,\"successThreshold\":1,\"failureThreshold\":3}"}},
						},
						{
							Name:  constants.AgentContainerName,
							Image: loggerConfig.Image,
							Args: []string{
								LoggerArgumentLogUrl,
								"http://httpbin.org/",
								LoggerArgumentSourceUri,
								"deployment",
								LoggerArgumentMode,
								"all",
								LoggerArgumentInferenceService,
								"sklearn",
								LoggerArgumentNamespace,
								"default",
								LoggerArgumentEndpoint,
								"default",
								LoggerArgumentComponent,
								"predictor",
							},
							Ports: []v1.ContainerPort{
								{
									Name:          "agent-port",
									ContainerPort: constants.InferenceServiceDefaultAgentPort,
									Protocol:      "TCP",
								},
							},
							Env:       []v1.EnvVar{{Name: "SERVING_READINESS_PROBE", Value: "{\"tcpSocket\":{\"port\":8080},\"timeoutSeconds\":1,\"periodSeconds\":10,\"successThreshold\":1,\"failureThreshold\":3}"}},
							Resources: agentResourceRequirement,
							ReadinessProbe: &v1.Probe{
								Handler: v1.Handler{
									HTTPGet: &v1.HTTPGetAction{
										HTTPHeaders: []v1.HTTPHeader{
											{
												Name:  "K-Network-Probe",
												Value: "queue",
											},
										},
										Port:   intstr.FromInt(9081),
										Path:   "/",
										Scheme: "HTTP",
									},
								},
							},
						},
					},
				},
			},
		},
		"DoNotAddLogger": {
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
			loggerConfig,
			batcherTestConfig,
		}
		injector.InjectAgent(scenario.original)
		if diff, _ := kmp.SafeDiff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}
