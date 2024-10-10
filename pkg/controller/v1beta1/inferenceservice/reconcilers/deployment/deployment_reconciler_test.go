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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

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
					constants.DeploymentMode:  string(constants.RawDeployment),
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
					constants.DeploymentMode:  string(constants.RawDeployment),
					constants.AutoscalerClass: string(constants.AutoscalerClassExternal),
				},
			},
			workerObjectMeta: metav1.ObjectMeta{
				Name:      "worker-predictor",
				Namespace: "worker-predictor-namespace",
				Annotations: map[string]string{
					"annotation": "annotation-value",
				},
				Labels: map[string]string{
					constants.DeploymentMode:  string(constants.RawDeployment),
					constants.AutoscalerClass: string(constants.AutoscalerClassExternal),
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
							{Name: "PIPELINE_PARALLEL_SIZE", Value: "2"},
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
						constants.DeploymentMode:        string(constants.RawDeployment),
					},
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							constants.RawDeploymentAppLabel: "isvc.default-predictor",
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
								constants.DeploymentMode:        string(constants.RawDeployment),
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
						"serving.kserve.io/autoscalerClass": "external",
						"serving.kserve.io/deploymentMode":  "RawDeployment",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "isvc.default-predictor",
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
								"serving.kserve.io/autoscalerClass": "external",
								"serving.kserve.io/deploymentMode":  "RawDeployment",
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
						constants.AutoscalerClass:       string(constants.AutoscalerClassExternal),
						constants.DeploymentMode:        string(constants.RawDeployment),
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							constants.RawDeploymentAppLabel: "isvc.default-predictor-worker",
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
								constants.AutoscalerClass:       string(constants.AutoscalerClassExternal),
								constants.DeploymentMode:        string(constants.RawDeployment),
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
										{Name: "PIPELINE_PARALLEL_SIZE", Value: "2"},
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
		name     string
		args     args
		expected []*appsv1.Deployment
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
			expected: expectedDeploymentPodSpecs["defaultDeployment"],
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
			expected: expectedDeploymentPodSpecs["multiNode-deployment"],
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := createRawDeployment(tt.args.objectMeta, tt.args.workerObjectMeta, tt.args.componentExt, tt.args.podSpec, tt.args.workerPodSpec)
			for i, deploy := range got {
				if diff := cmp.Diff(tt.expected[i], deploy, cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.SecurityContext"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.RestartPolicy"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.TerminationGracePeriodSeconds"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.DNSPolicy"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.AutomountServiceAccountToken"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.SchedulerName"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Strategy.Type"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Strategy.RollingUpdate"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.RevisionHistoryLimit"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.ProgressDeadlineSeconds")); diff != "" {
					t.Errorf("Test %q unexpected deployment (-want +got): %v", tt.name, diff)
				}
			}
		})
	}

	// To test additional multi-node scenarios
	getDefaultArgs := func() args {
		return args{
			objectMeta:       testInput["multiNode-deployment"].objectMeta,
			workerObjectMeta: testInput["multiNode-deployment"].workerObjectMeta,
			componentExt:     testInput["multiNode-deployment"].componentExt,
			podSpec:          testInput["multiNode-deployment"].podSpec,
			workerPodSpec:    testInput["multiNode-deployment"].workerPodSpec,
		}
	}

	getDefaultExpectedDeployment := func() []*appsv1.Deployment {
		return expectedDeploymentPodSpecs["multiNode-deployment"]
	}

	// pipelineParallelSize test
	objectMeta_tests := []struct {
		name           string
		modifyArgs     func(args) args
		modifyExpected func([]*appsv1.Deployment) []*appsv1.Deployment
	}{
		{
			name: "When the pipelineParallelSize set to 3, PIPELINE_PARALLEL_SIZE should be set to 3, and the number of worker node replicas should be set to 2",
			modifyArgs: func(updatedArgs args) args {
				if updatedArgs.podSpec.Containers[0].Name == constants.InferenceServiceContainerName {
					isvcutils.AddEnvVarToPodSpec(updatedArgs.podSpec, constants.InferenceServiceContainerName, constants.PipelineParallelSizeEnvName, "3")
				}
				if updatedArgs.workerPodSpec.Containers[0].Name == constants.WorkerContainerName {
					isvcutils.AddEnvVarToPodSpec(updatedArgs.workerPodSpec, constants.WorkerContainerName, constants.PipelineParallelSizeEnvName, "3")
				}
				return updatedArgs
			},
			modifyExpected: func(updatedExpected []*appsv1.Deployment) []*appsv1.Deployment {
				// e[0] is default deployment, e[1] is worker node deployment
				addEnvVarToDeploymentSpec(&updatedExpected[0].Spec, constants.InferenceServiceContainerName, "PIPELINE_PARALLEL_SIZE", "3")
				addEnvVarToDeploymentSpec(&updatedExpected[1].Spec, constants.WorkerContainerName, "PIPELINE_PARALLEL_SIZE", "3")
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
			got := createRawDeployment(ttArgs.objectMeta, ttArgs.workerObjectMeta, ttArgs.componentExt, tt.modifyArgs(ttArgs).podSpec, tt.modifyArgs(ttArgs).workerPodSpec)

			// update expected value using modifyExpected func
			expected := tt.modifyExpected(ttExpected)

			for i, deploy := range got {
				if diff := cmp.Diff(expected[i], deploy, cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.SecurityContext"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.RestartPolicy"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.TerminationGracePeriodSeconds"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.DNSPolicy"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.AutomountServiceAccountToken"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.SchedulerName"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Strategy.Type"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Strategy.RollingUpdate"),
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
	}{
		{
			name: "Use the value of TENSOR_PARALLEL_SIZE from the environment variables of pod for GPU resources when it is set",
			modifyPodSpecArgs: func(updatedArgs args) args {
				if _, exists := utils.GetEnvVarValue(updatedArgs.podSpec.Containers[0].Env, constants.TensorParallelSizeEnvName); exists {
					// Overwrite the environment variable
					for j, envVar := range updatedArgs.podSpec.Containers[0].Env {
						if envVar.Name == constants.TensorParallelSizeEnvName {
							updatedArgs.podSpec.Containers[0].Env[j].Value = "5"
							break
						}
					}
				}
				return updatedArgs
			},
			modifyWorkerPodSpecArgs:    func(updatedArgs args) args { return updatedArgs },
			modifyObjectMetaArgs:       func(updatedArgs args) args { return updatedArgs },
			modifyWorkerObjectMetaArgs: func(updatedArgs args) args { return updatedArgs },
			modifyExpected: func(updatedExpected []*appsv1.Deployment) []*appsv1.Deployment {
				// Overwrite the environment variable
				for j, envVar := range updatedExpected[0].Spec.Template.Spec.Containers[0].Env {
					if envVar.Name == constants.TensorParallelSizeEnvName {
						updatedExpected[0].Spec.Template.Spec.Containers[0].Env[j].Value = "5"
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

				if _, exists := utils.GetEnvVarValue(updatedArgs.podSpec.Containers[0].Env, constants.TensorParallelSizeEnvName); exists {
					// Overwrite the environment variable
					for j, envVar := range updatedArgs.podSpec.Containers[0].Env {
						if envVar.Name == constants.TensorParallelSizeEnvName {
							updatedArgs.podSpec.Containers[0].Env[j].Value = "3"
							break
						}
					}
				}
				return updatedArgs
			},
			modifyWorkerPodSpecArgs:    func(updatedArgs args) args { return updatedArgs },
			modifyObjectMetaArgs:       func(updatedArgs args) args { return updatedArgs },
			modifyWorkerObjectMetaArgs: func(updatedArgs args) args { return updatedArgs },
			modifyExpected: func(updatedExpected []*appsv1.Deployment) []*appsv1.Deployment {
				// Overwrite the environment variable
				for j, envVar := range updatedExpected[0].Spec.Template.Spec.Containers[0].Env {
					if envVar.Name == constants.TensorParallelSizeEnvName {
						updatedExpected[0].Spec.Template.Spec.Containers[0].Env[j].Value = "3"
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
			name: "Use one custom gpuResourceTypes when it is set in annotations even though it is not in gpuResourceTypeList",
			modifyPodSpecArgs: func(updatedArgs args) args {
				updatedArgs.podSpec.Containers[0].Resources = corev1.ResourceRequirements{}
				updatedArgs.podSpec.Containers[0].Resources.Requests = corev1.ResourceList{
					"custom.com/gpu": resource.MustParse("3"),
				}
				updatedArgs.podSpec.Containers[0].Resources.Limits = corev1.ResourceList{
					"custom.com/gpu": resource.MustParse("3"),
				}

				if _, exists := utils.GetEnvVarValue(updatedArgs.podSpec.Containers[0].Env, constants.TensorParallelSizeEnvName); exists {
					// Overwrite the environment variable
					for j, envVar := range updatedArgs.podSpec.Containers[0].Env {
						if envVar.Name == constants.TensorParallelSizeEnvName {
							updatedArgs.podSpec.Containers[0].Env[j].Value = "3"
							break
						}
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

				if _, exists := utils.GetEnvVarValue(updatedArgs.podSpec.Containers[0].Env, constants.TensorParallelSizeEnvName); exists {
					// Overwrite the environment variable
					for j, envVar := range updatedArgs.podSpec.Containers[0].Env {
						if envVar.Name == constants.TensorParallelSizeEnvName {
							updatedArgs.podSpec.Containers[0].Env[j].Value = "3"
							break
						}
					}
				}
				return updatedArgs
			},
			modifyObjectMetaArgs: func(updatedArgs args) args {
				updatedArgs.objectMeta.Annotations[constants.CustomGPUResourceTypesAnnotationKey] = "custom.com/gpu"
				return updatedArgs
			},
			modifyWorkerObjectMetaArgs: func(updatedArgs args) args {
				updatedArgs.workerObjectMeta.Annotations[constants.CustomGPUResourceTypesAnnotationKey] = "custom.com/gpu"
				return updatedArgs
			},
			modifyExpected: func(updatedExpected []*appsv1.Deployment) []*appsv1.Deployment {
				// Overwrite the environment variable

				for _, deployment := range updatedExpected {
					deployment.Annotations[constants.CustomGPUResourceTypesAnnotationKey] = "custom.com/gpu"
					deployment.Spec.Template.Annotations[constants.CustomGPUResourceTypesAnnotationKey] = "custom.com/gpu"
					deployment.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{}
				}

				for j, envVar := range updatedExpected[0].Spec.Template.Spec.Containers[0].Env {
					if envVar.Name == constants.TensorParallelSizeEnvName {
						updatedExpected[0].Spec.Template.Spec.Containers[0].Env[j].Value = "3"
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
			name: "Use multiple custom gpuResourceTypes when it is set in annotations even though it is not in gpuResourceTypeList",
			modifyPodSpecArgs: func(updatedArgs args) args {
				updatedArgs.podSpec.Containers[0].Resources = corev1.ResourceRequirements{}
				updatedArgs.podSpec.Containers[0].Resources.Requests = corev1.ResourceList{
					"custom.com/gpu2": resource.MustParse("3"),
				}
				updatedArgs.podSpec.Containers[0].Resources.Limits = corev1.ResourceList{
					"custom.com/gpu2": resource.MustParse("3"),
				}

				if _, exists := utils.GetEnvVarValue(updatedArgs.podSpec.Containers[0].Env, constants.TensorParallelSizeEnvName); exists {
					// Overwrite the environment variable
					for j, envVar := range updatedArgs.podSpec.Containers[0].Env {
						if envVar.Name == constants.TensorParallelSizeEnvName {
							updatedArgs.podSpec.Containers[0].Env[j].Value = "3"
							break
						}
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

				if _, exists := utils.GetEnvVarValue(updatedArgs.podSpec.Containers[0].Env, constants.TensorParallelSizeEnvName); exists {
					// Overwrite the environment variable
					for j, envVar := range updatedArgs.podSpec.Containers[0].Env {
						if envVar.Name == constants.TensorParallelSizeEnvName {
							updatedArgs.podSpec.Containers[0].Env[j].Value = "3"
							break
						}
					}
				}
				return updatedArgs
			},
			modifyObjectMetaArgs: func(updatedArgs args) args {
				updatedArgs.objectMeta.Annotations[constants.CustomGPUResourceTypesAnnotationKey] = strings.Join([]string{"custom.com/gpu", "custom.com/gpu2"}, ",")
				return updatedArgs
			},
			modifyWorkerObjectMetaArgs: func(updatedArgs args) args {
				updatedArgs.workerObjectMeta.Annotations[constants.CustomGPUResourceTypesAnnotationKey] = strings.Join([]string{"custom.com/gpu", "custom.com/gpu2"}, ",")
				return updatedArgs
			},
			modifyExpected: func(updatedExpected []*appsv1.Deployment) []*appsv1.Deployment {
				// Overwrite the environment variable

				for _, deployment := range updatedExpected {
					// serving.kserve.io/gpu-resource-types: '["gpu-type1", "gpu-type2", "gpu-type3"]'
					deployment.Annotations[constants.CustomGPUResourceTypesAnnotationKey] = strings.Join([]string{"custom.com/gpu", "custom.com/gpu2"}, ",")
					deployment.Spec.Template.Annotations[constants.CustomGPUResourceTypesAnnotationKey] = strings.Join([]string{"custom.com/gpu", "custom.com/gpu2"}, ",")
					deployment.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{}
				}

				for j, envVar := range updatedExpected[0].Spec.Template.Spec.Containers[0].Env {
					if envVar.Name == constants.TensorParallelSizeEnvName {
						updatedExpected[0].Spec.Template.Spec.Containers[0].Env[j].Value = "3"
						break
					}
				}
				updatedExpected[0].Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						"custom.com/gpu2": resource.MustParse("3"),
					},
					Limits: corev1.ResourceList{
						"custom.com/gpu2": resource.MustParse("3"),
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
			got := createRawDeployment(tt.modifyObjectMetaArgs(ttArgs).objectMeta, tt.modifyWorkerObjectMetaArgs(ttArgs).workerObjectMeta, ttArgs.componentExt, tt.modifyPodSpecArgs(ttArgs).podSpec, tt.modifyWorkerPodSpecArgs(ttArgs).workerPodSpec)

			// update expected value using modifyExpected func
			expected := tt.modifyExpected(ttExpected)

			for i, deploy := range got {
				if diff := cmp.Diff(expected[i], deploy, cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.SecurityContext"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.RestartPolicy"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.TerminationGracePeriodSeconds"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.DNSPolicy"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.AutomountServiceAccountToken"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.SchedulerName"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Strategy.Type"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Strategy.RollingUpdate"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.RevisionHistoryLimit"),
					cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.ProgressDeadlineSeconds")); diff != "" {
					t.Errorf("Test %q unexpected deployment (-want +got): %v", tt.name, diff)
				}
			}
		})
	}
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
