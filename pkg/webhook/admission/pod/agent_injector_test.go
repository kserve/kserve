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
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
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
		Image:      "gcr.io/kfserving/agent:latest",
		DefaultUrl: "http://httpbin.org/",
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
								ProbeHandler: v1.ProbeHandler{
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
								ProbeHandler: v1.ProbeHandler{
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
								ProbeHandler: v1.ProbeHandler{
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
								ProbeHandler: v1.ProbeHandler{
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
								ProbeHandler: v1.ProbeHandler{
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
								ProbeHandler: v1.ProbeHandler{
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
		"AddBatcher": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.BatcherInternalAnnotationKey:             "true",
						constants.BatcherMaxLatencyInternalAnnotationKey:   "100",
						constants.BatcherMaxBatchSizeInternalAnnotationKey: "30",
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
								ProbeHandler: v1.ProbeHandler{
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
						constants.BatcherInternalAnnotationKey:             "true",
						constants.BatcherMaxLatencyInternalAnnotationKey:   "100",
						constants.BatcherMaxBatchSizeInternalAnnotationKey: "30",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "sklearn",
							ReadinessProbe: &v1.Probe{
								ProbeHandler: v1.ProbeHandler{
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
								BatcherEnableFlag,
								BatcherArgumentMaxBatchSize,
								"30",
								BatcherArgumentMaxLatency,
								"100",
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
								ProbeHandler: v1.ProbeHandler{
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
		"DoNotAddBatcher": {
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
		"AgentAlreadyInjected": {
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
								ProbeHandler: v1.ProbeHandler{
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
								ProbeHandler: v1.ProbeHandler{
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
								ProbeHandler: v1.ProbeHandler{
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
							Env: []v1.EnvVar{
								{Name: "SERVING_READINESS_PROBE", Value: "{\"tcpSocket\":{\"port\":8080},\"timeoutSeconds\":1,\"periodSeconds\":10,\"successThreshold\":1,\"failureThreshold\":3}"}},
							ReadinessProbe: &v1.Probe{
								ProbeHandler: v1.ProbeHandler{
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
		"DefaultLoggerConfig": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.LoggerInternalAnnotationKey: "true",
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
							Name: "kserve-container",
							ReadinessProbe: &v1.Probe{
								ProbeHandler: v1.ProbeHandler{
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
							Name: "kserve-container",
							ReadinessProbe: &v1.Probe{
								ProbeHandler: v1.ProbeHandler{
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
								"--component-port",
								constants.InferenceServiceDefaultHttpPort,
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
								ProbeHandler: v1.ProbeHandler{
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
		"QueueProxyUserPortProvided": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deployment",
					Namespace: "default",
					Annotations: map[string]string{
						constants.LoggerInternalAnnotationKey: "true",
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
							Name: "kserve-container",
							ReadinessProbe: &v1.Probe{
								ProbeHandler: v1.ProbeHandler{
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
							Env: []v1.EnvVar{
								{Name: "SERVING_READINESS_PROBE", Value: "{\"tcpSocket\":{\"port\":8080},\"timeoutSeconds\":1,\"periodSeconds\":10,\"successThreshold\":1,\"failureThreshold\":3}"},
								{Name: "USER_PORT", Value: "8080"},
							},
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
							Name: "kserve-container",
							ReadinessProbe: &v1.Probe{
								ProbeHandler: v1.ProbeHandler{
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
							Env: []v1.EnvVar{
								{Name: "SERVING_READINESS_PROBE", Value: "{\"tcpSocket\":{\"port\":8080},\"timeoutSeconds\":1,\"periodSeconds\":10,\"successThreshold\":1,\"failureThreshold\":3}"},
								{Name: "USER_PORT", Value: constants.InferenceServiceDefaultAgentPortStr},
							},
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
								"--component-port",
								constants.InferenceServiceDefaultHttpPort,
							},
							Ports: []v1.ContainerPort{
								{
									Name:          "agent-port",
									ContainerPort: constants.InferenceServiceDefaultAgentPort,
									Protocol:      "TCP",
								},
							},
							Env: []v1.EnvVar{
								{Name: "SERVING_READINESS_PROBE", Value: "{\"tcpSocket\":{\"port\":8080},\"timeoutSeconds\":1,\"periodSeconds\":10,\"successThreshold\":1,\"failureThreshold\":3}"},
								{Name: "USER_PORT", Value: "8080"},
							},
							Resources: agentResourceRequirement,
							ReadinessProbe: &v1.Probe{
								ProbeHandler: v1.ProbeHandler{
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
		"KserveContainer has port": {
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
							Name: "kserve-container",
							ReadinessProbe: &v1.Probe{
								ProbeHandler: v1.ProbeHandler{
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
							Ports: []v1.ContainerPort{
								{
									Name:          "serving-port",
									ContainerPort: 80,
								},
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
							Name: "kserve-container",
							ReadinessProbe: &v1.Probe{
								ProbeHandler: v1.ProbeHandler{
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
							Ports: []v1.ContainerPort{
								{
									Name:          "serving-port",
									ContainerPort: 80,
								},
							},
							VolumeMounts: []v1.VolumeMount{
								{Name: "model-dir", MountPath: "/mnt/models"},
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
							Args: []string{"--enable-puller", "--config-dir", "/mnt/configs", "--model-dir", "/mnt/models", "--component-port", "80"},
							Ports: []v1.ContainerPort{
								{
									Name:          "agent-port",
									ContainerPort: constants.InferenceServiceDefaultAgentPort,
									Protocol:      "TCP",
								},
							},
							Env: []v1.EnvVar{{Name: "SERVING_READINESS_PROBE", Value: "{\"tcpSocket\":{\"port\":8080},\"timeoutSeconds\":1,\"periodSeconds\":10,\"successThreshold\":1,\"failureThreshold\":3}"}},
							ReadinessProbe: &v1.Probe{
								ProbeHandler: v1.ProbeHandler{
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
	}

	credentialBuilder := credentials.NewCredentialBuilder(c, &v1.ConfigMap{
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

func TestGetLoggerConfigs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	cases := []struct {
		name      string
		configMap *v1.ConfigMap
		matchers  []types.GomegaMatcher
	}{
		{
			name: "Valid Logger Config",
			configMap: &v1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Data: map[string]string{
					LoggerConfigMapKeyName: `{
						"Image":         "gcr.io/kfserving/logger:latest",
						"CpuRequest":    "100m",
						"CpuLimit":      "1",
						"MemoryRequest": "200Mi",
						"MemoryLimit":   "1Gi"
					}`,
				},
				BinaryData: map[string][]byte{},
			},
			matchers: []types.GomegaMatcher{
				gomega.Equal(&LoggerConfig{
					Image:         "gcr.io/kfserving/logger:latest",
					CpuRequest:    "100m",
					CpuLimit:      "1",
					MemoryRequest: "200Mi",
					MemoryLimit:   "1Gi",
				}),
				gomega.BeNil(),
			},
		},
		{
			name: "Invalid Resource Value",
			configMap: &v1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Data: map[string]string{
					LoggerConfigMapKeyName: `{
						"Image":         "gcr.io/kfserving/logger:latest",
						"CpuRequest":    "100m",
						"CpuLimit":      "1",
						"MemoryRequest": "200mc",
						"MemoryLimit":   "1Gi"
					}`,
				},
				BinaryData: map[string][]byte{},
			},
			matchers: []types.GomegaMatcher{
				gomega.Equal(&LoggerConfig{
					Image:         "gcr.io/kfserving/logger:latest",
					CpuRequest:    "100m",
					CpuLimit:      "1",
					MemoryRequest: "200mc",
					MemoryLimit:   "1Gi",
				}),
				gomega.HaveOccurred(),
			},
		},
	}

	for _, tc := range cases {
		loggerConfigs, err := getLoggerConfigs(tc.configMap)
		g.Expect(err).Should(tc.matchers[1])
		g.Expect(loggerConfigs).Should(tc.matchers[0])
	}
}

func TestGetAgentConfigs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	cases := []struct {
		name      string
		configMap *v1.ConfigMap
		matchers  []types.GomegaMatcher
	}{
		{
			name: "Valid Agent Config",
			configMap: &v1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Data: map[string]string{
					constants.AgentConfigMapKeyName: `{
						"Image":         "gcr.io/kfserving/agent:latest",
						"CpuRequest":    "100m",
						"CpuLimit":      "1",
						"MemoryRequest": "200Mi",
						"MemoryLimit":   "1Gi"
					}`,
				},
				BinaryData: map[string][]byte{},
			},
			matchers: []types.GomegaMatcher{
				gomega.Equal(&AgentConfig{
					Image:         "gcr.io/kfserving/agent:latest",
					CpuRequest:    "100m",
					CpuLimit:      "1",
					MemoryRequest: "200Mi",
					MemoryLimit:   "1Gi",
				}),
				gomega.BeNil(),
			},
		},
		{
			name: "Invalid Resource Value",
			configMap: &v1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Data: map[string]string{
					constants.AgentConfigMapKeyName: `{
						"Image":         "gcr.io/kfserving/agent:latest",
						"CpuRequest":    "100m",
						"CpuLimit":      "1",
						"MemoryRequest": "200mc",
						"MemoryLimit":   "1Gi"
					}`,
				},
				BinaryData: map[string][]byte{},
			},
			matchers: []types.GomegaMatcher{
				gomega.Equal(&AgentConfig{
					Image:         "gcr.io/kfserving/agent:latest",
					CpuRequest:    "100m",
					CpuLimit:      "1",
					MemoryRequest: "200mc",
					MemoryLimit:   "1Gi",
				}),
				gomega.HaveOccurred(),
			},
		},
	}

	for _, tc := range cases {
		loggerConfigs, err := getAgentConfigs(tc.configMap)
		g.Expect(err).Should(tc.matchers[1])
		g.Expect(loggerConfigs).Should(tc.matchers[0])
	}
}
