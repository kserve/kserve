/*
Copyright 2024 The KServe Authors.

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
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	isvcutils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"
	"github.com/kserve/kserve/pkg/utils"
)

func TestCreateDefaultDeployment(t *testing.T) {
	type args struct {
		objectMeta       metav1.ObjectMeta
		workerObjectMeta metav1.ObjectMeta
		componentExt     *v1beta1.ComponentExtensionSpec
		podSpec          *corev1.PodSpec
		workerPodSpec    *corev1.PodSpec
	}
	testInput := map[string]args{
		"defaultDeployment": {
			objectMeta: metav1.ObjectMeta{
				Name:      "default-predictor",
				Namespace: "default-predictor-namespace",
				Annotations: map[string]string{
					"annotation": "annotation-value",
				},
				Labels: map[string]string{
					constants.DeploymentMode:  string(constants.Standard),
					constants.AutoscalerClass: string(constants.DefaultAutoscalerClass),
				},
			},
			workerObjectMeta: metav1.ObjectMeta{},
			componentExt:     &v1beta1.ComponentExtensionSpec{},
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "default-predictor-example-volume",
					},
				},
				Containers: []corev1.Container{
					{
						Name:  constants.InferenceServiceContainerName,
						Image: "default-predictor-example-image",
						Env: []corev1.EnvVar{
							{Name: "default-predictor-example-env", Value: "example-env"},
						},
					},
				},
			},
			workerPodSpec: nil,
		},
		"multiNode-deployment": {
			objectMeta: metav1.ObjectMeta{
				Name:      "default-predictor",
				Namespace: "default-predictor-namespace",
				Annotations: map[string]string{
					"annotation": "annotation-value",
				},
				Labels: map[string]string{
					constants.DeploymentMode:  string(constants.Standard),
					constants.AutoscalerClass: string(constants.AutoscalerClassNone),
				},
			},
			workerObjectMeta: metav1.ObjectMeta{
				Name:      "worker-predictor",
				Namespace: "worker-predictor-namespace",
				Annotations: map[string]string{
					"annotation": "annotation-value",
				},
				Labels: map[string]string{
					constants.DeploymentMode:  string(constants.Standard),
					constants.AutoscalerClass: string(constants.AutoscalerClassNone),
				},
			},
			componentExt: &v1beta1.ComponentExtensionSpec{},
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "default-predictor-example-volume",
					},
				},
				Containers: []corev1.Container{
					{
						Name:  constants.InferenceServiceContainerName,
						Image: "default-predictor-example-image",
						Env: []corev1.EnvVar{
							{Name: "TENSOR_PARALLEL_SIZE", Value: "1"},
							{Name: "MODEL_NAME"},
							{Name: "PIPELINE_PARALLEL_SIZE", Value: "2"},
							{Name: "RAY_NODE_COUNT", Value: "2"},
							{Name: "REQUEST_GPU_COUNT", Value: "1"},
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								constants.NvidiaGPUResourceType: resource.MustParse("1"),
							},
							Requests: corev1.ResourceList{
								constants.NvidiaGPUResourceType: resource.MustParse("1"),
							},
						},
					},
				},
			},
			workerPodSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "worker-predictor-example-volume",
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "worker-container",
						Image: "worker-predictor-example-image",
						Env: []corev1.EnvVar{
							{Name: "worker-predictor-example-env", Value: "example-env"},
							{Name: "RAY_NODE_COUNT", Value: "2"},
							{Name: "REQUEST_GPU_COUNT", Value: "1"},
							{Name: "ISVC_NAME"},
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								constants.NvidiaGPUResourceType: resource.MustParse("1"),
							},
							Requests: corev1.ResourceList{
								constants.NvidiaGPUResourceType: resource.MustParse("1"),
							},
						},
					},
				},
			},
		},
	}

	expectedDeploymentPodSpecs := map[string][]*appsv1.Deployment{
		"defaultDeployment": {
			&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-predictor",
					Namespace: "default-predictor-namespace",
					Annotations: map[string]string{
						"annotation": "annotation-value",
					},
					Labels: map[string]string{
						constants.RawDeploymentAppLabel: "isvc.default-predictor",
						constants.AutoscalerClass:       string(constants.AutoscalerClassHPA),
						constants.DeploymentMode:        string(constants.Standard),
					},
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							constants.RawDeploymentAppLabel: "isvc.default-predictor",
						},
					},
					Strategy: appsv1.DeploymentStrategy{
						Type: appsv1.RollingUpdateDeploymentStrategyType,
						RollingUpdate: &appsv1.RollingUpdateDeployment{
							MaxUnavailable: intStrPtr("25%"),
							MaxSurge:       intStrPtr("25%"),
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "default-predictor",
							Namespace: "default-predictor-namespace",
							Annotations: map[string]string{
								"annotation": "annotation-value",
							},
							Labels: map[string]string{
								constants.RawDeploymentAppLabel: "isvc.default-predictor",
								constants.AutoscalerClass:       string(constants.AutoscalerClassHPA),
								constants.DeploymentMode:        string(constants.Standard),
							},
						},
						Spec: corev1.PodSpec{
							Volumes:                      []corev1.Volume{{Name: "default-predictor-example-volume"}},
							AutomountServiceAccountToken: BoolPtr(false),
							Containers: []corev1.Container{
								{
									Name:  constants.InferenceServiceContainerName,
									Image: "default-predictor-example-image",
									Env: []corev1.EnvVar{
										{Name: "default-predictor-example-env", Value: "example-env"},
									},
									ImagePullPolicy:          "IfNotPresent",
									TerminationMessagePolicy: "File",
									TerminationMessagePath:   "/dev/termination-log",
									ReadinessProbe: &corev1.Probe{
										ProbeHandler: corev1.ProbeHandler{
											TCPSocket: &corev1.TCPSocketAction{
												Port: intstr.IntOrString{IntVal: 8080},
												Host: "",
											},
										},
										TimeoutSeconds:   1,
										PeriodSeconds:    10,
										SuccessThreshold: 1,
										FailureThreshold: 3,
									},
								},
							},
						},
					},
				},
			},
			nil,
		},
		"multiNode-deployment": {
			&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-predictor",
					Namespace: "default-predictor-namespace",
					Annotations: map[string]string{
						"annotation": "annotation-value",
					},
					Labels: map[string]string{
						"app":                               "isvc.default-predictor",
						"serving.kserve.io/autoscalerClass": "none",
						"serving.kserve.io/deploymentMode":  "Standard",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "isvc.default-predictor",
						},
					},
					Strategy: appsv1.DeploymentStrategy{
						Type: appsv1.RollingUpdateDeploymentStrategyType,
						RollingUpdate: &appsv1.RollingUpdateDeployment{
							MaxUnavailable: intStrPtr("25%"),
							MaxSurge:       intStrPtr("25%"),
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "default-predictor",
							Namespace: "default-predictor-namespace",
							Annotations: map[string]string{
								"annotation": "annotation-value",
							},
							Labels: map[string]string{
								"app":                               "isvc.default-predictor",
								"serving.kserve.io/autoscalerClass": "none",
								"serving.kserve.io/deploymentMode":  "Standard",
							},
						},
						Spec: corev1.PodSpec{
							Volumes:                      []corev1.Volume{{Name: "default-predictor-example-volume"}},
							AutomountServiceAccountToken: BoolPtr(false),
							Containers: []corev1.Container{
								{
									Name:  constants.InferenceServiceContainerName,
									Image: "default-predictor-example-image",
									Env: []corev1.EnvVar{
										{Name: "TENSOR_PARALLEL_SIZE", Value: "1"},
										{Name: "MODEL_NAME"},
										{Name: "PIPELINE_PARALLEL_SIZE", Value: "2"},
										{Name: "RAY_NODE_COUNT", Value: "2"},
										{Name: "REQUEST_GPU_COUNT", Value: "1"},
									},
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											constants.NvidiaGPUResourceType: resource.MustParse("1"),
										},
										Requests: corev1.ResourceList{
											constants.NvidiaGPUResourceType: resource.MustParse("1"),
										},
									},
									ImagePullPolicy:          "IfNotPresent",
									TerminationMessagePolicy: "File",
									TerminationMessagePath:   "/dev/termination-log",
									ReadinessProbe: &corev1.Probe{
										ProbeHandler: corev1.ProbeHandler{
											TCPSocket: &corev1.TCPSocketAction{
												Port: intstr.IntOrString{IntVal: 8080},
												Host: "",
											},
										},
										TimeoutSeconds:   1,
										PeriodSeconds:    10,
										SuccessThreshold: 1,
										FailureThreshold: 3,
									},
								},
							},
						},
					},
				},
			},
			&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "worker-predictor",
					Namespace: "worker-predictor-namespace",
					Annotations: map[string]string{
						"annotation": "annotation-value",
					},
					Labels: map[string]string{
						constants.RawDeploymentAppLabel: "isvc.default-predictor-worker",
						constants.AutoscalerClass:       string(constants.AutoscalerClassNone),
						constants.DeploymentMode:        string(constants.Standard),
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							constants.RawDeploymentAppLabel: "isvc.default-predictor-worker",
						},
					},
					Strategy: appsv1.DeploymentStrategy{
						Type: appsv1.RollingUpdateDeploymentStrategyType,
						RollingUpdate: &appsv1.RollingUpdateDeployment{
							MaxUnavailable: intStrPtr("0%"),
							MaxSurge:       intStrPtr("100%"),
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "worker-predictor",
							Namespace: "worker-predictor-namespace",
							Annotations: map[string]string{
								"annotation": "annotation-value",
							},
							Labels: map[string]string{
								constants.RawDeploymentAppLabel: "isvc.default-predictor-worker",
								constants.AutoscalerClass:       string(constants.AutoscalerClassNone),
								constants.DeploymentMode:        string(constants.Standard),
							},
						},
						Spec: corev1.PodSpec{
							Volumes:                      []corev1.Volume{{Name: "worker-predictor-example-volume"}},
							AutomountServiceAccountToken: BoolPtr(false),
							Containers: []corev1.Container{
								{
									Name:  "worker-container",
									Image: "worker-predictor-example-image",
									Env: []corev1.EnvVar{
										{Name: "worker-predictor-example-env", Value: "example-env"},
										{Name: "RAY_NODE_COUNT", Value: "2"},
										{Name: "REQUEST_GPU_COUNT", Value: "1"},
										{Name: "ISVC_NAME"},
									},
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											constants.NvidiaGPUResourceType: resource.MustParse("1"),
										},
										Requests: corev1.ResourceList{
											constants.NvidiaGPUResourceType: resource.MustParse("1"),
										},
									},
									ImagePullPolicy:          "IfNotPresent",
									TerminationMessagePolicy: "File",
									TerminationMessagePath:   "/dev/termination-log",
								},
							},
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name        string
		args        args
		expected    []*appsv1.Deployment
		expectedErr error
	}{
		{
			name: "default deployment",
			args: args{
				objectMeta:       testInput["defaultDeployment"].objectMeta,
				workerObjectMeta: testInput["defaultDeployment"].workerObjectMeta,
				componentExt:     testInput["defaultDeployment"].componentExt,
				podSpec:          testInput["defaultDeployment"].podSpec,
				workerPodSpec:    testInput["defaultDeployment"].workerPodSpec,
			},
			expected:    expectedDeploymentPodSpecs["defaultDeployment"],
			expectedErr: nil,
		},
		{
			name: "multiNode-deployment",
			args: args{
				objectMeta:       testInput["multiNode-deployment"].objectMeta,
				workerObjectMeta: testInput["multiNode-deployment"].workerObjectMeta,
				componentExt:     testInput["multiNode-deployment"].componentExt,
				podSpec:          testInput["multiNode-deployment"].podSpec,
				workerPodSpec:    testInput["multiNode-deployment"].workerPodSpec,
			},
			expected:    expectedDeploymentPodSpecs["multiNode-deployment"],
			expectedErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := createRawDeployment(tt.args.objectMeta, tt.args.workerObjectMeta, tt.args.componentExt, tt.args.podSpec, tt.args.workerPodSpec, nil)
			assert.Equal(t, tt.expectedErr, err)
			for i, deploy := range got {
				if diff := cmp.Diff(tt.expected[i], deploy, cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.SecurityContext"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.RestartPolicy"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.TerminationGracePeriodSeconds"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.DNSPolicy"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.AutomountServiceAccountToken"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.SchedulerName"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.RevisionHistoryLimit"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.ProgressDeadlineSeconds")); diff != "" {
					t.Errorf("Test %q unexpected deployment (-want +got): %v", tt.name, diff)
				}
			}
		})
	}

	// deepCopyArgs creates a deep copy of the provided args struct.
	// It ensures that nested pointers (componentExt, podSpec, workerPodSpec) are properly duplicated
	// to avoid unintended side effects when the original struct is modified.
	deepCopyArgs := func(src args) args {
		dst := args{
			objectMeta:       src.objectMeta,
			workerObjectMeta: src.workerObjectMeta,
		}
		if src.componentExt != nil {
			dst.componentExt = src.componentExt.DeepCopy()
		}
		if src.podSpec != nil {
			dst.podSpec = src.podSpec.DeepCopy()
		}
		if src.workerPodSpec != nil {
			dst.workerPodSpec = src.workerPodSpec.DeepCopy()
		}
		return dst
	}

	getDefaultArgs := func() args {
		return deepCopyArgs(testInput["multiNode-deployment"])
	}
	getDefaultExpectedDeployment := func() []*appsv1.Deployment {
		return deepCopyDeploymentList(expectedDeploymentPodSpecs["multiNode-deployment"])
	}

	// pipelineParallelSize test
	objectMeta_tests := []struct {
		name           string
		modifyArgs     func(args) args
		modifyExpected func([]*appsv1.Deployment) []*appsv1.Deployment
		expectedErr    error
	}{
		{
			name: "Set RAY_NODE_COUNT to 3 when pipelineParallelSize is 3 and tensorParallelSize is 1, with 2 worker node replicas",
			modifyArgs: func(updatedArgs args) args {
				if updatedArgs.podSpec.Containers[0].Name == constants.InferenceServiceContainerName {
					isvcutils.AddEnvVarToPodSpec(updatedArgs.podSpec, constants.InferenceServiceContainerName, constants.PipelineParallelSizeEnvName, "3")
					isvcutils.AddEnvVarToPodSpec(updatedArgs.podSpec, constants.InferenceServiceContainerName, constants.RayNodeCountEnvName, "3")
				}
				if updatedArgs.workerPodSpec.Containers[0].Name == constants.WorkerContainerName {
					isvcutils.AddEnvVarToPodSpec(updatedArgs.workerPodSpec, constants.WorkerContainerName, constants.RayNodeCountEnvName, "3")
				}
				return updatedArgs
			},
			modifyExpected: func(updatedExpected []*appsv1.Deployment) []*appsv1.Deployment {
				// updatedExpected[0] is default deployment, updatedExpected[1] is worker node deployment
				addEnvVarToDeploymentSpec(&updatedExpected[0].Spec, constants.InferenceServiceContainerName, constants.PipelineParallelSizeEnvName, "3")
				addEnvVarToDeploymentSpec(&updatedExpected[0].Spec, constants.InferenceServiceContainerName, constants.RayNodeCountEnvName, "3")
				addEnvVarToDeploymentSpec(&updatedExpected[1].Spec, constants.WorkerContainerName, constants.RayNodeCountEnvName, "3")
				updatedExpected[1].Spec.Replicas = int32Ptr(2)
				return updatedExpected
			},
		},
	}

	for _, tt := range objectMeta_tests {
		t.Run(tt.name, func(t *testing.T) {
			// retrieve args, expected
			ttArgs := getDefaultArgs()
			ttExpected := getDefaultExpectedDeployment()

			// update objectMeta using modify func
			got, err := createRawDeployment(ttArgs.objectMeta, ttArgs.workerObjectMeta, ttArgs.componentExt, tt.modifyArgs(ttArgs).podSpec, tt.modifyArgs(ttArgs).workerPodSpec, nil)
			assert.Equal(t, tt.expectedErr, err)

			// update expected value using modifyExpected func
			expected := tt.modifyExpected(ttExpected)

			for i, deploy := range got {
				if diff := cmp.Diff(expected[i], deploy, cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.SecurityContext"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.RestartPolicy"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.TerminationGracePeriodSeconds"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.DNSPolicy"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.AutomountServiceAccountToken"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.SchedulerName"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.RevisionHistoryLimit"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.ProgressDeadlineSeconds")); diff != "" {
					t.Errorf("Test %q unexpected deployment (-want +got): %v", tt.name, diff)
				}
			}
		})
	}

	// tensor-parallel-size test
	podSpec_tests := []struct {
		name                       string
		modifyPodSpecArgs          func(args) args
		modifyWorkerPodSpecArgs    func(args) args
		modifyObjectMetaArgs       func(args) args
		modifyWorkerObjectMetaArgs func(args) args
		modifyExpected             func([]*appsv1.Deployment) []*appsv1.Deployment
		expectedErr                error
	}{
		{
			name: "Use the value of GPU in resources request of container",
			modifyPodSpecArgs: func(updatedArgs args) args {
				// Overwrite the environment variable
				for j, envVar := range updatedArgs.podSpec.Containers[0].Env {
					if envVar.Name == constants.RequestGPUCountEnvName {
						updatedArgs.podSpec.Containers[0].Env[j].Value = "5"
						break
					}
				}
				return updatedArgs
			},
			modifyWorkerPodSpecArgs: func(updatedArgs args) args {
				// Overwrite the environment variable
				for j, envVar := range updatedArgs.workerPodSpec.Containers[0].Env {
					if envVar.Name == constants.RequestGPUCountEnvName {
						updatedArgs.workerPodSpec.Containers[0].Env[j].Value = "5"
						break
					}
				}
				return updatedArgs
			},
			modifyObjectMetaArgs:       func(updatedArgs args) args { return updatedArgs },
			modifyWorkerObjectMetaArgs: func(updatedArgs args) args { return updatedArgs },
			modifyExpected: func(updatedExpected []*appsv1.Deployment) []*appsv1.Deployment {
				// Overwrite the environment variable
				for j, envVar := range updatedExpected[0].Spec.Template.Spec.Containers[0].Env {
					if envVar.Name == constants.RequestGPUCountEnvName {
						updatedExpected[0].Spec.Template.Spec.Containers[0].Env[j].Value = "5"
						continue
					}
				}
				for j, envVar := range updatedExpected[1].Spec.Template.Spec.Containers[0].Env {
					if envVar.Name == constants.RequestGPUCountEnvName {
						updatedExpected[1].Spec.Template.Spec.Containers[0].Env[j].Value = "5"
						break
					}
				}

				for _, deploy := range updatedExpected {
					deploy.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							constants.NvidiaGPUResourceType: resource.MustParse("5"),
						},
						Requests: corev1.ResourceList{
							constants.NvidiaGPUResourceType: resource.MustParse("5"),
						},
					}
				}

				return updatedExpected
			},
		},
		{
			name: "Use specified gpuResourceType if it is in gpuResourceTypeList",
			modifyPodSpecArgs: func(updatedArgs args) args {
				intelGPUResourceType := corev1.ResourceName(constants.IntelGPUResourceType)
				updatedArgs.podSpec.Containers[0].Resources.Requests = corev1.ResourceList{
					intelGPUResourceType: resource.MustParse("3"),
				}
				updatedArgs.podSpec.Containers[0].Resources.Limits = corev1.ResourceList{
					intelGPUResourceType: resource.MustParse("3"),
				}

				for j, envVar := range updatedArgs.podSpec.Containers[0].Env {
					if envVar.Name == constants.RequestGPUCountEnvName {
						updatedArgs.podSpec.Containers[0].Env[j].Value = "3"
						break
					}
				}
				return updatedArgs
			},
			modifyWorkerPodSpecArgs: func(updatedArgs args) args {
				for j, envVar := range updatedArgs.workerPodSpec.Containers[0].Env {
					if envVar.Name == constants.RequestGPUCountEnvName {
						updatedArgs.workerPodSpec.Containers[0].Env[j].Value = "3"
						break
					}
				}
				return updatedArgs
			},
			modifyObjectMetaArgs:       func(updatedArgs args) args { return updatedArgs },
			modifyWorkerObjectMetaArgs: func(updatedArgs args) args { return updatedArgs },
			modifyExpected: func(updatedExpected []*appsv1.Deployment) []*appsv1.Deployment {
				// Overwrite the environment variable
				for j, envVar := range updatedExpected[0].Spec.Template.Spec.Containers[0].Env {
					if envVar.Name == constants.RequestGPUCountEnvName {
						updatedExpected[0].Spec.Template.Spec.Containers[0].Env[j].Value = "3"
						continue
					}
				}
				for j, envVar := range updatedExpected[1].Spec.Template.Spec.Containers[0].Env {
					if envVar.Name == constants.RequestGPUCountEnvName {
						updatedExpected[1].Spec.Template.Spec.Containers[0].Env[j].Value = "3"
						break
					}
				}

				updatedExpected[0].Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						constants.IntelGPUResourceType: resource.MustParse("3"),
					},
					Limits: corev1.ResourceList{
						constants.IntelGPUResourceType: resource.MustParse("3"),
					},
				}
				// worker node will use default gpuResourceType (NvidiaGPUResourceType)
				updatedExpected[1].Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						constants.NvidiaGPUResourceType: resource.MustParse("3"),
					},
					Limits: corev1.ResourceList{
						constants.NvidiaGPUResourceType: resource.MustParse("3"),
					},
				}

				return updatedExpected
			},
		},
		{
			name: "Use a custom gpuResourceType specified in annotations, even when it is not listed in the default gpuResourceTypeList",
			modifyPodSpecArgs: func(updatedArgs args) args {
				updatedArgs.podSpec.Containers[0].Resources = corev1.ResourceRequirements{}
				updatedArgs.podSpec.Containers[0].Resources.Requests = corev1.ResourceList{
					"custom.com/gpu": resource.MustParse("3"),
				}
				updatedArgs.podSpec.Containers[0].Resources.Limits = corev1.ResourceList{
					"custom.com/gpu": resource.MustParse("3"),
				}

				for j, envVar := range updatedArgs.podSpec.Containers[0].Env {
					if envVar.Name == constants.RequestGPUCountEnvName {
						updatedArgs.podSpec.Containers[0].Env[j].Value = "3"
						break
					}
				}
				return updatedArgs
			},
			modifyWorkerPodSpecArgs: func(updatedArgs args) args {
				updatedArgs.workerPodSpec.Containers[0].Resources = corev1.ResourceRequirements{}
				updatedArgs.workerPodSpec.Containers[0].Resources.Requests = corev1.ResourceList{
					"custom.com/gpu": resource.MustParse("3"),
				}
				updatedArgs.workerPodSpec.Containers[0].Resources.Limits = corev1.ResourceList{
					"custom.com/gpu": resource.MustParse("3"),
				}

				for j, envVar := range updatedArgs.workerPodSpec.Containers[0].Env {
					if envVar.Name == constants.RequestGPUCountEnvName {
						updatedArgs.workerPodSpec.Containers[0].Env[j].Value = "3"
						break
					}
				}
				return updatedArgs
			},
			modifyObjectMetaArgs: func(updatedArgs args) args {
				updatedArgs.objectMeta.Annotations[constants.CustomGPUResourceTypesAnnotationKey] = "[\"custom.com/gpu\"]"
				return updatedArgs
			},
			modifyWorkerObjectMetaArgs: func(updatedArgs args) args {
				updatedArgs.workerObjectMeta.Annotations[constants.CustomGPUResourceTypesAnnotationKey] = "[\"custom.com/gpu\"]"
				return updatedArgs
			},
			modifyExpected: func(updatedExpected []*appsv1.Deployment) []*appsv1.Deployment {
				for _, deployment := range updatedExpected {
					deployment.Annotations[constants.CustomGPUResourceTypesAnnotationKey] = "[\"custom.com/gpu\"]"
					deployment.Spec.Template.Annotations[constants.CustomGPUResourceTypesAnnotationKey] = "[\"custom.com/gpu\"]"
					deployment.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{}
				}

				for j, envVar := range updatedExpected[0].Spec.Template.Spec.Containers[0].Env {
					if envVar.Name == constants.RequestGPUCountEnvName {
						updatedExpected[0].Spec.Template.Spec.Containers[0].Env[j].Value = "3"
						continue
					}
				}
				for j, envVar := range updatedExpected[1].Spec.Template.Spec.Containers[0].Env {
					if envVar.Name == constants.RequestGPUCountEnvName {
						updatedExpected[1].Spec.Template.Spec.Containers[0].Env[j].Value = "3"
						break
					}
				}
				updatedExpected[0].Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						"custom.com/gpu": resource.MustParse("3"),
					},
					Limits: corev1.ResourceList{
						"custom.com/gpu": resource.MustParse("3"),
					},
				}
				updatedExpected[1].Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						"custom.com/gpu": resource.MustParse("3"),
					},
					Limits: corev1.ResourceList{
						"custom.com/gpu": resource.MustParse("3"),
					},
				}

				return updatedExpected
			},
		},
		{
			name: "Allow multiple custom gpuResourceTypes from annotations, even when they are not listed in the default gpuResourceTypeList",
			modifyPodSpecArgs: func(updatedArgs args) args {
				updatedArgs.podSpec.Containers[0].Resources = corev1.ResourceRequirements{}
				updatedArgs.podSpec.Containers[0].Resources.Requests = corev1.ResourceList{
					"custom.com/gpu": resource.MustParse("3"),
				}
				updatedArgs.podSpec.Containers[0].Resources.Limits = corev1.ResourceList{
					"custom.com/gpu": resource.MustParse("3"),
				}

				for j, envVar := range updatedArgs.podSpec.Containers[0].Env {
					if envVar.Name == constants.RequestGPUCountEnvName {
						updatedArgs.podSpec.Containers[0].Env[j].Value = "3"
						break
					}
				}
				return updatedArgs
			},
			modifyWorkerPodSpecArgs: func(updatedArgs args) args {
				updatedArgs.workerPodSpec.Containers[0].Resources = corev1.ResourceRequirements{}
				updatedArgs.workerPodSpec.Containers[0].Resources.Requests = corev1.ResourceList{
					"custom.com/gpu2": resource.MustParse("3"),
				}
				updatedArgs.workerPodSpec.Containers[0].Resources.Limits = corev1.ResourceList{
					"custom.com/gpu2": resource.MustParse("3"),
				}

				for j, envVar := range updatedArgs.workerPodSpec.Containers[0].Env {
					if envVar.Name == constants.RequestGPUCountEnvName {
						updatedArgs.workerPodSpec.Containers[0].Env[j].Value = "3"
						break
					}
				}
				return updatedArgs
			},
			modifyObjectMetaArgs: func(updatedArgs args) args {
				updatedArgs.objectMeta.Annotations[constants.CustomGPUResourceTypesAnnotationKey] = "[\"custom.com/gpu\", \"custom.com/gpu2\"]"
				return updatedArgs
			},
			modifyWorkerObjectMetaArgs: func(updatedArgs args) args {
				updatedArgs.workerObjectMeta.Annotations[constants.CustomGPUResourceTypesAnnotationKey] = "[\"custom.com/gpu\", \"custom.com/gpu2\"]"
				return updatedArgs
			},
			modifyExpected: func(updatedExpected []*appsv1.Deployment) []*appsv1.Deployment {
				// Overwrite the environment variable

				for _, deployment := range updatedExpected {
					// serving.kserve.io/gpu-resource-types: '["gpu-type1", "gpu-type2", "gpu-type3"]'
					deployment.Annotations[constants.CustomGPUResourceTypesAnnotationKey] = "[\"custom.com/gpu\", \"custom.com/gpu2\"]"
					deployment.Spec.Template.Annotations[constants.CustomGPUResourceTypesAnnotationKey] = "[\"custom.com/gpu\", \"custom.com/gpu2\"]"
					deployment.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{}
				}

				for j, envVar := range updatedExpected[0].Spec.Template.Spec.Containers[0].Env {
					if envVar.Name == constants.RequestGPUCountEnvName {
						updatedExpected[0].Spec.Template.Spec.Containers[0].Env[j].Value = "3"
						continue
					}
				}
				for j, envVar := range updatedExpected[1].Spec.Template.Spec.Containers[0].Env {
					if envVar.Name == constants.RequestGPUCountEnvName {
						updatedExpected[1].Spec.Template.Spec.Containers[0].Env[j].Value = "3"
						break
					}
				}

				updatedExpected[0].Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						"custom.com/gpu": resource.MustParse("3"),
					},
					Limits: corev1.ResourceList{
						"custom.com/gpu": resource.MustParse("3"),
					},
				}
				updatedExpected[1].Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						"custom.com/gpu2": resource.MustParse("3"),
					},
					Limits: corev1.ResourceList{
						"custom.com/gpu2": resource.MustParse("3"),
					},
				}

				return updatedExpected
			},
		},
	}

	for _, tt := range podSpec_tests {
		t.Run(tt.name, func(t *testing.T) {
			// retrieve args, expected
			ttArgs := getDefaultArgs()
			ttExpected := getDefaultExpectedDeployment()

			// update objectMeta using modify func
			got, err := createRawDeployment(tt.modifyObjectMetaArgs(ttArgs).objectMeta, tt.modifyWorkerObjectMetaArgs(ttArgs).workerObjectMeta, ttArgs.componentExt, tt.modifyPodSpecArgs(ttArgs).podSpec, tt.modifyWorkerPodSpecArgs(ttArgs).workerPodSpec, nil)
			assert.Equal(t, tt.expectedErr, err)
			// update expected value using modifyExpected func
			expected := tt.modifyExpected(ttExpected)

			for i, deploy := range got {
				if diff := cmp.Diff(expected[i], deploy, cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.SecurityContext"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.RestartPolicy"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.TerminationGracePeriodSeconds"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.DNSPolicy"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.AutomountServiceAccountToken"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.SchedulerName"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.RevisionHistoryLimit"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.ProgressDeadlineSeconds")); diff != "" {
					t.Errorf("Test %q unexpected deployment (-want +got): %v", tt.name, diff)
				}
			}
		})
	}
}

func TestCheckDeploymentExist(t *testing.T) {
	type fields struct {
		client kclient.Client
	}
	type args struct {
		deployment *appsv1.Deployment
		existing   *appsv1.Deployment
		getErr     error
	}
	tests := []struct {
		name         string
		args         args
		wantResult   constants.CheckResultType
		wantExisting *appsv1.Deployment
		wantErr      bool
	}{
		{
			name: "deployment not found returns CheckResultCreate",
			args: args{
				deployment: &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"},
				},
				getErr: errors.NewNotFound(appsv1.Resource("deployment"), "foo"),
			},
			wantResult:   constants.CheckResultCreate,
			wantExisting: nil,
			wantErr:      false,
		},
		{
			name: "get error returns CheckResultUnknown",
			args: args{
				deployment: &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"},
				},
				getErr: fmt.Errorf("some error"), //nolint
			},
			wantResult:   constants.CheckResultUnknown,
			wantExisting: nil,
			wantErr:      true,
		},
		{
			name: "deployment exists and is equivalent returns CheckResultExisted",
			args: args{
				deployment: &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "foo"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "foo"}},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{Name: "c", Image: "img"},
								},
							},
						},
					},
				},
				existing: &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "foo"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "foo"}},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{Name: "c", Image: "img"},
								},
							},
						},
					},
				},
				getErr: nil,
			},
			wantResult:   constants.CheckResultExisted,
			wantExisting: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"}},
			wantErr:      false,
		},
		{
			name: "deployment exists and is different returns CheckResultUpdate",
			args: args{
				deployment: &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "foo"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "foo"}},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{Name: "c", Image: "img1"},
								},
							},
						},
					},
				},
				existing: &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "foo"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "foo"}},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{Name: "c", Image: "img2"},
								},
							},
						},
					},
				},
				getErr: nil,
			},
			wantResult:   constants.CheckResultUpdate,
			wantExisting: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "bar"}},
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockClientForCheckDeploymentExist{
				getDeployment: tt.args.existing,
				getErr:        tt.args.getErr,
			}
			r := &DeploymentReconciler{
				client: mockClient,
			}
			ctx := t.Context()
			gotResult, gotExisting, err := r.checkDeploymentExist(ctx, mockClient, tt.args.deployment)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkDeploymentExist() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotResult != tt.wantResult {
				t.Errorf("checkDeploymentExist() gotResult = %v, want %v", gotResult, tt.wantResult)
			}
			// Only check name/namespace for gotExisting
			if tt.wantExisting != nil && gotExisting != nil {
				if gotExisting.Name != tt.args.deployment.Name || gotExisting.Namespace != tt.args.deployment.Namespace {
					t.Errorf("checkDeploymentExist() gotExisting = %v, want %v", gotExisting, tt.wantExisting)
				}
			}
			if tt.wantExisting == nil && gotExisting != nil {
				t.Errorf("checkDeploymentExist() gotExisting = %v, want nil", gotExisting)
			}
		})
	}
}

func TestNewDeploymentReconciler(t *testing.T) {
	type fields struct {
		client       kclient.Client
		scheme       *runtime.Scheme
		objectMeta   metav1.ObjectMeta
		workerMeta   metav1.ObjectMeta
		componentExt *v1beta1.ComponentExtensionSpec
		podSpec      *corev1.PodSpec
		workerPod    *corev1.PodSpec
	}
	tests := []struct {
		name        string
		fields      fields
		wantErr     bool
		wantWorkers int
	}{
		{
			name: "default deployment",
			fields: fields{
				client: nil,
				scheme: nil,
				objectMeta: metav1.ObjectMeta{
					Name:      "test-predictor",
					Namespace: "test-ns",
					Labels: map[string]string{
						constants.DeploymentMode:  string(constants.Standard),
						constants.AutoscalerClass: string(constants.DefaultAutoscalerClass),
					},
					Annotations: map[string]string{},
				},
				workerMeta:   metav1.ObjectMeta{},
				componentExt: &v1beta1.ComponentExtensionSpec{},
				podSpec: &corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  constants.InferenceServiceContainerName,
							Image: "test-image",
						},
					},
				},
				workerPod: nil,
			},
			wantErr:     false,
			wantWorkers: 1,
		},
		{
			name: "multi-node deployment",
			fields: fields{
				client: nil,
				scheme: nil,
				objectMeta: metav1.ObjectMeta{
					Name:      "test-predictor",
					Namespace: "test-ns",
					Labels: map[string]string{
						constants.DeploymentMode:  string(constants.Standard),
						constants.AutoscalerClass: string(constants.AutoscalerClassNone),
					},
					Annotations: map[string]string{},
				},
				workerMeta: metav1.ObjectMeta{
					Name:      "worker-predictor",
					Namespace: "test-ns",
					Labels: map[string]string{
						constants.DeploymentMode:  string(constants.Standard),
						constants.AutoscalerClass: string(constants.AutoscalerClassNone),
					},
					Annotations: map[string]string{},
				},
				componentExt: &v1beta1.ComponentExtensionSpec{},
				podSpec: &corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  constants.InferenceServiceContainerName,
							Image: "test-image",
							Env: []corev1.EnvVar{
								{Name: constants.RayNodeCountEnvName, Value: "2"},
								{Name: constants.RequestGPUCountEnvName, Value: "1"},
							},
						},
					},
				},
				workerPod: &corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  constants.WorkerContainerName,
							Image: "worker-image",
							Env: []corev1.EnvVar{
								{Name: constants.RequestGPUCountEnvName, Value: "1"},
							},
						},
					},
				},
			},
			wantErr:     false,
			wantWorkers: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewDeploymentReconciler(
				tt.fields.client,
				tt.fields.scheme,
				tt.fields.objectMeta,
				tt.fields.workerMeta,
				tt.fields.componentExt,
				tt.fields.podSpec,
				tt.fields.workerPod,
				nil, // deployConfig
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewDeploymentReconciler() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && got != nil {
				if len(got.DeploymentList) != tt.wantWorkers {
					t.Errorf("DeploymentList length = %v, want %v", len(got.DeploymentList), tt.wantWorkers)
				}
				if got.componentExt != tt.fields.componentExt {
					t.Errorf("componentExt pointer mismatch")
				}
			}
		})
	}
}

func TestSetDefaultDeploymentSpec(t *testing.T) {
	tests := []struct {
		name                   string
		inputSpec              *appsv1.DeploymentSpec
		expectedMaxSurge       *intstr.IntOrString
		expectedMaxUnavailable *intstr.IntOrString
		description            string
	}{
		{
			name:                   "Empty spec should get KServe defaults",
			inputSpec:              &appsv1.DeploymentSpec{},
			expectedMaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
			expectedMaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
			description:            "Empty spec should be populated with KServe default deployment strategy",
		},
		{
			name: "Spec with RollingUpdate type but nil RollingUpdate should get KServe defaults",
			inputSpec: &appsv1.DeploymentSpec{
				Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RollingUpdateDeploymentStrategyType,
				},
			},
			expectedMaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
			expectedMaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
			description:            "Spec with RollingUpdate type but nil RollingUpdate should get KServe defaults",
		},
		{
			name: "Spec with existing RollingUpdate should not be modified",
			inputSpec: &appsv1.DeploymentSpec{
				Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RollingUpdateDeploymentStrategyType,
					RollingUpdate: &appsv1.RollingUpdateDeployment{
						MaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
						MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "10%"},
					},
				},
			},
			expectedMaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
			expectedMaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "10%"},
			description:            "Existing RollingUpdate values should not be modified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setDefaultDeploymentSpec(tt.inputSpec)

			assert.Equal(t, appsv1.RollingUpdateDeploymentStrategyType, tt.inputSpec.Strategy.Type)
			assert.NotNil(t, tt.inputSpec.Strategy.RollingUpdate)
			assert.Equal(t, tt.expectedMaxSurge, tt.inputSpec.Strategy.RollingUpdate.MaxSurge,
				"Test: %s - %s", tt.name, tt.description)
			assert.Equal(t, tt.expectedMaxUnavailable, tt.inputSpec.Strategy.RollingUpdate.MaxUnavailable,
				"Test: %s - %s", tt.name, tt.description)
		})
	}
}

func stringPtr(s string) *string {
	return &s
}

// mockClientForCheckDeploymentExist is a minimal mock for kclient.Client for checkDeploymentExist
type mockClientForCheckDeploymentExist struct {
	kclient.Client
	getDeployment *appsv1.Deployment
	getErr        error
}

func (m *mockClientForCheckDeploymentExist) Get(ctx context.Context, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
	if m.getErr != nil {
		return m.getErr
	}
	if m.getDeployment != nil {
		d := obj.(*appsv1.Deployment)
		*d = *m.getDeployment.DeepCopy()
	}
	return nil
}

func (m *mockClientForCheckDeploymentExist) Update(ctx context.Context, obj kclient.Object, opts ...kclient.UpdateOption) error {
	// Simulate dry-run update always succeeds
	return nil
}

func intStrPtr(s string) *intstr.IntOrString {
	v := intstr.FromString(s)
	return &v
}

func int32Ptr(i int32) *int32 {
	val := i
	return &val
}

func BoolPtr(b bool) *bool {
	val := b
	return &val
}

// Function to add a new environment variable to a specific container in the DeploymentSpec
func addEnvVarToDeploymentSpec(deploymentSpec *appsv1.DeploymentSpec, containerName, envName, envValue string) {
	// Iterate over the containers in the PodTemplateSpec to find the specified container
	for i, container := range deploymentSpec.Template.Spec.Containers {
		if container.Name == containerName {
			if _, exists := utils.GetEnvVarValue(container.Env, envName); exists {
				// Overwrite the environment variable
				for j, envVar := range container.Env {
					if envVar.Name == envName {
						deploymentSpec.Template.Spec.Containers[i].Env[j].Value = envValue
						break
					}
				}
			} else {
				// Add the new environment variable to the Env field if it does not exist
				container.Env = append(container.Env, corev1.EnvVar{
					Name:  envName,
					Value: envValue,
				})
				deploymentSpec.Template.Spec.Containers[i].Env = container.Env
			}
		}
	}
}

// deepCopyDeploymentList creates a deep copy of a slice of Deployment pointers.
// This ensures that modifications to the original slice or its elements do not affect the copied slice.
func deepCopyDeploymentList(src []*appsv1.Deployment) []*appsv1.Deployment {
	if src == nil {
		return nil
	}
	copied := make([]*appsv1.Deployment, len(src))
	for i, deployment := range src {
		if deployment != nil {
			copied[i] = deployment.DeepCopy()
		}
	}
	return copied
}

func TestApplyRolloutStrategyFromConfigmap(t *testing.T) {
	tests := []struct {
		name                   string
		deployConfig           *v1beta1.DeployConfig
		expectedMaxSurge       *intstr.IntOrString
		expectedMaxUnavailable *intstr.IntOrString
	}{
		{
			name: "Apply rollout strategy with RawDeployment mode",
			deployConfig: &v1beta1.DeployConfig{
				DefaultDeploymentMode: "Standard",
				RawDeploymentRolloutStrategy: &v1beta1.RawDeploymentRolloutStrategy{
					DefaultRollout: &v1beta1.RolloutSpec{
						MaxSurge:       "1",
						MaxUnavailable: "1",
					},
				},
			},
			expectedMaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "1"},
			expectedMaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "1"},
		},
		{
			name: "No rollout strategy applied for Serverless mode",
			deployConfig: &v1beta1.DeployConfig{
				DefaultDeploymentMode: "Serverless",
				RawDeploymentRolloutStrategy: &v1beta1.RawDeploymentRolloutStrategy{
					DefaultRollout: &v1beta1.RolloutSpec{
						MaxSurge:       "50%",
						MaxUnavailable: "25%",
					},
				},
			},
			expectedMaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "25%"}, // Default value
			expectedMaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"}, // Default value
		},
		{
			name:                   "No rollout strategy configured",
			deployConfig:           nil,
			expectedMaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "25%"}, // Default value
			expectedMaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"}, // Default value
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment := &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{},
			}

			// Set default deployment spec first
			setDefaultDeploymentSpec(&deployment.Spec)

			// Apply rollout strategy from configmap
			applyRolloutStrategyFromConfigmap(&deployment.Spec, tt.deployConfig)

			// Verify the deployment strategy type is RollingUpdate
			assert.Equal(t, appsv1.RollingUpdateDeploymentStrategyType, deployment.Spec.Strategy.Type)

			// Verify maxSurge and maxUnavailable values
			assert.Equal(t, tt.expectedMaxSurge, deployment.Spec.Strategy.RollingUpdate.MaxSurge, tt.name+" - MaxSurge")
			assert.Equal(t, tt.expectedMaxUnavailable, deployment.Spec.Strategy.RollingUpdate.MaxUnavailable, tt.name+" - MaxUnavailable")
		})
	}
}

func TestCreateRawDeploymentWithPrecedence(t *testing.T) {
	tests := []struct {
		name                     string
		deploymentStrategy       *appsv1.DeploymentStrategy
		deployConfig             *v1beta1.DeployConfig
		expectedMaxSurge         *intstr.IntOrString
		expectedMaxUnavailable   *intstr.IntOrString
		expectedFromUserStrategy bool
		description              string
	}{
		{
			name: "User strategy takes precedence over configmap",
			deploymentStrategy: &appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
					MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
				},
			},
			deployConfig: &v1beta1.DeployConfig{
				DefaultDeploymentMode: "Standard",
				RawDeploymentRolloutStrategy: &v1beta1.RawDeploymentRolloutStrategy{
					DefaultRollout: &v1beta1.RolloutSpec{
						MaxSurge:       "1",
						MaxUnavailable: "1",
					},
				},
			},
			expectedMaxSurge:         &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
			expectedMaxUnavailable:   &intstr.IntOrString{Type: intstr.String, StrVal: "50%"},
			expectedFromUserStrategy: true,
			description:              "User-specified strategy should take precedence over configmap",
		},
		{
			name:               "Configmap strategy when no user strategy",
			deploymentStrategy: nil,
			deployConfig: &v1beta1.DeployConfig{
				DefaultDeploymentMode: "Standard",
				RawDeploymentRolloutStrategy: &v1beta1.RawDeploymentRolloutStrategy{
					DefaultRollout: &v1beta1.RolloutSpec{
						MaxSurge:       "2",
						MaxUnavailable: "1",
					},
				},
			},
			expectedMaxSurge:         &intstr.IntOrString{Type: intstr.String, StrVal: "2"},
			expectedMaxUnavailable:   &intstr.IntOrString{Type: intstr.String, StrVal: "1"},
			expectedFromUserStrategy: false,
			description:              "Configmap strategy should be applied when no user strategy",
		},
		{
			name:               "No strategy applied for Serverless mode",
			deploymentStrategy: nil,
			deployConfig: &v1beta1.DeployConfig{
				DefaultDeploymentMode: "Serverless",
				RawDeploymentRolloutStrategy: &v1beta1.RawDeploymentRolloutStrategy{
					DefaultRollout: &v1beta1.RolloutSpec{
						MaxSurge:       "1",
						MaxUnavailable: "1",
					},
				},
			},
			expectedMaxSurge:         &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
			expectedMaxUnavailable:   &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
			expectedFromUserStrategy: false,
			description:              "Default strategy should be used for non-RawDeployment modes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test data
			componentExt := &v1beta1.ComponentExtensionSpec{
				DeploymentStrategy: tt.deploymentStrategy,
			}

			objectMeta := metav1.ObjectMeta{
				Name:      "test-deployment",
				Namespace: "test-namespace",
				Labels:    make(map[string]string),
			}

			podSpec := &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "test-container",
						Image: "test-image",
					},
				},
			}

			// Create deployment
			deployment := createRawDefaultDeployment(objectMeta, componentExt, podSpec, tt.deployConfig)

			// Verify strategy
			assert.NotNil(t, deployment.Spec.Strategy.RollingUpdate, tt.description)
			assert.Equal(t, tt.expectedMaxSurge, deployment.Spec.Strategy.RollingUpdate.MaxSurge, tt.description+" - MaxSurge")
			assert.Equal(t, tt.expectedMaxUnavailable, deployment.Spec.Strategy.RollingUpdate.MaxUnavailable, tt.description+" - MaxUnavailable")
		})
	}
}
