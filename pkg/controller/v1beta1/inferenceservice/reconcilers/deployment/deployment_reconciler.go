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

package deployment

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"knative.dev/pkg/kmp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("DeploymentReconciler")

// DeploymentReconciler reconciles the raw kubernetes deployment resource
type DeploymentReconciler struct {
	client         kclient.Client
	scheme         *runtime.Scheme
	DeploymentList []*appsv1.Deployment
	componentExt   *v1beta1.ComponentExtensionSpec
}

func NewDeploymentReconciler(client kclient.Client,
	scheme *runtime.Scheme,
	componentMeta metav1.ObjectMeta,
	workerComponentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec, workerPodSpec *corev1.PodSpec) *DeploymentReconciler {
	return &DeploymentReconciler{
		client:         client,
		scheme:         scheme,
		DeploymentList: createRawDeployment(componentMeta, workerComponentMeta, componentExt, podSpec, workerPodSpec),
		componentExt:   componentExt,
	}
}
func createRawDeployment(componentMeta metav1.ObjectMeta, workerComponentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec, //nolint:unparam
	podSpec *corev1.PodSpec, workerPodSpec *corev1.PodSpec) []*appsv1.Deployment {
	var deploymentList []*appsv1.Deployment
	var workerNodeSize string
	var pipelineParallelSize string

	isvcName := componentMeta.GetLabels()[constants.InferenceServiceLabel]
	multiNodeEnabled := false

	if workerComponentMeta.Name != "" {
		multiNodeEnabled = true
		workerNodeSize = componentMeta.GetAnnotations()[constants.WorkerNodeSize]
		if parsedValue, err := strconv.Atoi(workerNodeSize); err == nil {
			// Set pipelineParallelSize to workerNodeSize + 1 (head)
			pipelineParallelSize = strconv.Itoa(parsedValue + 1)
		} else {
			log.Error(err, "Failed to convert pipelineParallelSize to int, using the WorkerSpec.Size)")
		}
	}

	// Use defaut value(1) if tensor-parallel-size is not set (gpu count)
	tensorParallelSize := constants.DefaultTensorParallelSize

	for _, container := range podSpec.Containers {
		if container.Name == constants.InferenceServiceContainerName {
			if value, exists := utils.GetEnvVarValue(container.Env, constants.TensorParallelSizeEnvName); exists {
				if intValue, err := strconv.Atoi(value); err == nil {
					if intValue > 0 {
						// Use the environment variable value if it's greater than 0
						tensorParallelSize = value
					} else {
						log.Info("Using the default value for tensor-parallel-size because the provided value is less than 0")
					}
				} else {
					// Log the error if the conversion fails, and use default
					log.Error(err, "Failed to convert tensorParallelSize to int, using default")
				}

				break
			}
		}
	}

	defaultDeployment := createRawDefaultDeployment(componentMeta, componentExt, podSpec)
	if multiNodeEnabled {
		// Update GPU resource of default podSpec
		addGPUResourceToDeployment(defaultDeployment, constants.InferenceServiceContainerName, tensorParallelSize)

		// Set the environment variable for "isvc name" to the MODEL_NAME when multiNodeEnabled is true.
		addEnvVarToDeploymentSpec(&defaultDeployment.Spec, constants.InferenceServiceContainerName, "MODEL_NAME", isvcName)

		deploymentAnnotations := componentMeta.GetAnnotations()[constants.StorageInitializerSourceUriInternalAnnotationKey]
		storageProtocol := strings.Split(deploymentAnnotations, "://")[0]
		if storageProtocol == "pvc" {
			// Set the environment variable for "/mnt/models" to the MODEL_DIR when multiNodeEnabled is true.
			addEnvVarToDeploymentSpec(&defaultDeployment.Spec, constants.InferenceServiceContainerName, "MODEL_DIR", constants.DefaultModelLocalMountPath)
		}
		// Set the environment variable PIPELINE_PARALLEL_SIZE when multiNodeEnabled is true.
		addEnvVarToDeploymentSpec(&defaultDeployment.Spec, constants.InferenceServiceContainerName, constants.PipelineParallelSizeEnvName, pipelineParallelSize)
	}
	deploymentList = append(deploymentList, defaultDeployment)

	// Adds workerNode deployment
	if multiNodeEnabled {
		workerDeployment := createRawWorkerDeployment(workerComponentMeta, componentExt, workerPodSpec, componentMeta.Name, pipelineParallelSize, isvcName)

		// Update GPU resource of workerPodSpec based on tensor-parallelSize
		addGPUResourceToDeployment(workerDeployment, constants.WorkerContainerName, tensorParallelSize)
		deploymentList = append(deploymentList, workerDeployment)
	}

	return deploymentList
}

func createRawDefaultDeployment(componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec, //nolint:unparam
	podSpec *corev1.PodSpec) *appsv1.Deployment {
	podMetadata := componentMeta
	podMetadata.Labels["app"] = constants.GetRawServiceLabel(componentMeta.Name)
	setDefaultPodSpec(podSpec)
	deployment := &appsv1.Deployment{
		ObjectMeta: componentMeta,
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": constants.GetRawServiceLabel(componentMeta.Name),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: podMetadata,
				Spec:       *podSpec,
			},
		},
	}
	if componentExt.DeploymentStrategy != nil {
		deployment.Spec.Strategy = *componentExt.DeploymentStrategy
	}
	setDefaultDeploymentSpec(&deployment.Spec)
	return deployment
}
func createRawWorkerDeployment(componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec, //nolint:unparam
	podSpec *corev1.PodSpec, predictorName string, pipelineParallelSize string, isvcName string) *appsv1.Deployment {
	podMetadata := componentMeta
	workerPredictorName := constants.GetRawWorkerServiceLabel(predictorName)
	podMetadata.Labels["app"] = workerPredictorName
	setDefaultPodSpec(podSpec)
	deployment := &appsv1.Deployment{
		ObjectMeta: componentMeta,
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": workerPredictorName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: podMetadata,
				Spec:       *podSpec,
			},
		},
	}
	if componentExt.DeploymentStrategy != nil {
		deployment.Spec.Strategy = *componentExt.DeploymentStrategy
	}
	setDefaultDeploymentSpec(&deployment.Spec)

	// default workerNode deployment replicas
	replicas, err := utils.ConvertStringToInt32(constants.DefaultWorkerMinSize, 1)
	if err != nil {
		log.Error(err, "Failed to convert replicas int to int32, using the default value(1) for replicas.")
	}
	if parsedValue, err := strconv.Atoi(pipelineParallelSize); err == nil {
		replicas, err = utils.ConvertStringToInt32(parsedValue-1, 1)
		if err != nil {
			log.Error(err, "Failed to convert replicas int to int32, using the default value(1) for replicas.")
		}
	} else {
		log.Error(err, "Failed to convert pipelineParallelSize string to int, using the default value(1) for replicas.")
	}

	deployment.Spec.Replicas = &replicas
	addEnvVarToDeploymentSpec(&deployment.Spec, constants.WorkerContainerName, "ISVC_NAME", isvcName)
	addEnvVarToDeploymentSpec(&deployment.Spec, constants.WorkerContainerName, constants.PipelineParallelSizeEnvName, pipelineParallelSize)
	return deployment
}

// checkDeploymentExist checks if the deployment exists?
func (r *DeploymentReconciler) checkDeploymentExist(client kclient.Client, deployment *appsv1.Deployment) (constants.CheckResultType, *appsv1.Deployment, error) {
	// get deployment
	existingDeployment := &appsv1.Deployment{}
	err := client.Get(context.TODO(), types.NamespacedName{
		Namespace: deployment.ObjectMeta.Namespace,
		Name:      deployment.ObjectMeta.Name,
	}, existingDeployment)
	if err != nil {
		if apierr.IsNotFound(err) {
			return constants.CheckResultCreate, nil, nil
		}
		return constants.CheckResultUnknown, nil, err
	}
	// existed, check equivalence
	// for HPA scaling, we should ignore Replicas of Deployment
	ignoreFields := cmpopts.IgnoreFields(appsv1.DeploymentSpec{}, "Replicas")
	// Do a dry-run update. This will populate our local deployment object with any default values
	// that are present on the remote version.
	if err := client.Update(context.TODO(), deployment, kclient.DryRunAll); err != nil {
		log.Error(err, "Failed to perform dry-run update of deployment", "Deployment", deployment.Name)
		return constants.CheckResultUnknown, nil, err
	}
	if diff, err := kmp.SafeDiff(deployment.Spec, existingDeployment.Spec, ignoreFields); err != nil {
		return constants.CheckResultUnknown, nil, err
	} else if diff != "" {
		log.Info("Deployment Updated", "Diff", diff)
		return constants.CheckResultUpdate, existingDeployment, nil
	}
	return constants.CheckResultExisted, existingDeployment, nil
}

func setDefaultPodSpec(podSpec *corev1.PodSpec) {
	if podSpec.DNSPolicy == "" {
		podSpec.DNSPolicy = corev1.DNSClusterFirst
	}
	if podSpec.RestartPolicy == "" {
		podSpec.RestartPolicy = corev1.RestartPolicyAlways
	}
	if podSpec.TerminationGracePeriodSeconds == nil {
		TerminationGracePeriodSeconds := int64(corev1.DefaultTerminationGracePeriodSeconds)
		podSpec.TerminationGracePeriodSeconds = &TerminationGracePeriodSeconds
	}
	if podSpec.SecurityContext == nil {
		podSpec.SecurityContext = &corev1.PodSecurityContext{}
	}
	if podSpec.SchedulerName == "" {
		podSpec.SchedulerName = corev1.DefaultSchedulerName
	}
	for i := range podSpec.Containers {
		container := &podSpec.Containers[i]
		if container.TerminationMessagePath == "" {
			container.TerminationMessagePath = "/dev/termination-log"
		}
		if container.TerminationMessagePolicy == "" {
			container.TerminationMessagePolicy = corev1.TerminationMessageReadFile
		}
		if container.ImagePullPolicy == "" {
			container.ImagePullPolicy = corev1.PullIfNotPresent
		}
		// generate default readiness probe for model server container and for transformer container in case of collocation
		if container.Name == constants.InferenceServiceContainerName || container.Name == constants.TransformerContainerName {
			if container.ReadinessProbe == nil {
				if len(container.Ports) == 0 {
					container.ReadinessProbe = &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							TCPSocket: &corev1.TCPSocketAction{
								Port: intstr.IntOrString{
									IntVal: 8080,
								},
							},
						},
						TimeoutSeconds:   1,
						PeriodSeconds:    10,
						SuccessThreshold: 1,
						FailureThreshold: 3,
					}
				} else {
					container.ReadinessProbe = &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							TCPSocket: &corev1.TCPSocketAction{
								Port: intstr.IntOrString{
									IntVal: container.Ports[0].ContainerPort,
								},
							},
						},
						TimeoutSeconds:   1,
						PeriodSeconds:    10,
						SuccessThreshold: 1,
						FailureThreshold: 3,
					}
				}
			}
		}
	}
}

func setDefaultDeploymentSpec(spec *appsv1.DeploymentSpec) {
	if spec.Strategy.Type == "" {
		spec.Strategy.Type = appsv1.RollingUpdateDeploymentStrategyType
	}
	if spec.Strategy.Type == appsv1.RollingUpdateDeploymentStrategyType && spec.Strategy.RollingUpdate == nil {
		spec.Strategy.RollingUpdate = &appsv1.RollingUpdateDeployment{
			MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
			MaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
		}
	}
	if spec.RevisionHistoryLimit == nil {
		revisionHistoryLimit := int32(10)
		spec.RevisionHistoryLimit = &revisionHistoryLimit
	}
	if spec.ProgressDeadlineSeconds == nil {
		progressDeadlineSeconds := int32(600)
		spec.ProgressDeadlineSeconds = &progressDeadlineSeconds
	}
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
				// Add the new environment variable to the Env field if it ooes not exist
				container.Env = append(container.Env, corev1.EnvVar{
					Name:  envName,
					Value: envValue,
				})
				deploymentSpec.Template.Spec.Containers[i].Env = container.Env
			}
			log.Info("Added env variable to container",
				"envName", envName,
				"envValue", envValue,
				"containerName", containerName)
			return
		}
	}
	log.Info("Container not found in DeploymentSpec", "containerName", containerName)
}

func addGPUResourceToDeployment(deployment *appsv1.Deployment, targetContainerName string, tensorParallelSize string) {
	for i, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == targetContainerName {
			// Initialize Limits map if it's nil
			if container.Resources.Limits == nil {
				deployment.Spec.Template.Spec.Containers[i].Resources.Limits = make(map[corev1.ResourceName]resource.Quantity)
			}

			// Assign the tensorParallelSize value to the GPU "nvidia.com/gpu" resource limits
			deployment.Spec.Template.Spec.Containers[i].Resources.Limits[constants.NvidiaGPUResourceType] = resource.MustParse(tensorParallelSize)

			// Initialize Requests map if it's nil
			if container.Resources.Requests == nil {
				deployment.Spec.Template.Spec.Containers[i].Resources.Requests = make(map[corev1.ResourceName]resource.Quantity)
			}

			// Assign the tensorParallelSize value to the GPU "nvidia.com/gpu" resource requests
			deployment.Spec.Template.Spec.Containers[i].Resources.Requests[constants.NvidiaGPUResourceType] = resource.MustParse(tensorParallelSize)
			break
		}
	}
}

// Reconcile ...
func (r *DeploymentReconciler) Reconcile() ([]*appsv1.Deployment, error) {
	for _, deployment := range r.DeploymentList {
		// Reconcile Deployment
		checkResult, _, err := r.checkDeploymentExist(r.client, deployment)
		if err != nil {
			return nil, err
		}
		log.Info("deployment reconcile", "checkResult", checkResult, "err", err)

		var opErr error
		switch checkResult {
		case constants.CheckResultCreate:
			opErr = r.client.Create(context.TODO(), deployment)
		case constants.CheckResultUpdate:
			curJson, err := json.Marshal(deployment)
			if err != nil {
				return nil, err
			}

			// To avoid the conflict between HPA and Deployment,
			// we need to remove the Replicas field from the deployment spec
			modDeployment := deployment.DeepCopy()
			modDeployment.Spec.Replicas = nil

			modJson, err := json.Marshal(modDeployment)
			if err != nil {
				return nil, err
			}
			// Generate the strategic merge patch between the current and modified JSON
			patchByte, err := strategicpatch.StrategicMergePatch(curJson, modJson, appsv1.Deployment{})
			if err != nil {
				return nil, err
			}

			// Patch the deployment object with the strategic merge patch
			opErr = r.client.Patch(context.TODO(), deployment, client.RawPatch(types.StrategicMergePatchType, patchByte))
		}

		if opErr != nil {
			return nil, opErr
		}
	}
	return r.DeploymentList, nil
}
