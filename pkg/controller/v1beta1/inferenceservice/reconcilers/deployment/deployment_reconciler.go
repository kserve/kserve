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
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmp"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"

	v1beta1utils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"

	"github.com/kserve/kserve/pkg/utils"
)

var log = logf.Log.WithName("DeploymentReconciler")

// DeploymentReconciler reconciles the raw kubernetes deployment resource
type DeploymentReconciler struct {
	client         kclient.Client
	scheme         *runtime.Scheme
	DeploymentList []*appsv1.Deployment
	componentExt   *v1beta1.ComponentExtensionSpec
}

const (
	tlsVolumeName = "proxy-tls"
	oauthProxy    = "oauthProxy"
)

func NewDeploymentReconciler(ctx context.Context,
	client kclient.Client,
	clientset kubernetes.Interface,
	scheme *runtime.Scheme,
	resourceType constants.ResourceType,
	componentMeta metav1.ObjectMeta,
	workerComponentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec, workerPodSpec *corev1.PodSpec,
) (*DeploymentReconciler, error) {
	deploymentList, err := createRawDeploymentODH(ctx, clientset, resourceType, componentMeta, workerComponentMeta, componentExt, podSpec, workerPodSpec)
	if err != nil {
		return nil, err
	}

	return &DeploymentReconciler{
		client:         client,
		scheme:         scheme,
		DeploymentList: deploymentList,
		componentExt:   componentExt,
	}, nil
}

func createRawDeploymentODH(ctx context.Context,
	clientset kubernetes.Interface,
	resourceType constants.ResourceType,
	componentMeta metav1.ObjectMeta,
	workerComponentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec, workerPodSpec *corev1.PodSpec,
) ([]*appsv1.Deployment, error) {
	deploymentList, err := createRawDeployment(componentMeta, workerComponentMeta, componentExt, podSpec, workerPodSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to create raw deployment: %w", err)
	}

	enableAuth := false
	// Deployment list is for multi-node, we only need to add oauth proxy and serving sercret certs to the head deployment
	headDeployment := deploymentList[0]
	if val, ok := componentMeta.Annotations[constants.ODHKserveRawAuth]; ok && strings.EqualFold(val, "true") {
		enableAuth = true

		if resourceType != constants.InferenceGraphResource { // InferenceGraphs don't use oauth-proxy
			err := addOauthContainerToDeployment(ctx, clientset, headDeployment, componentMeta, componentExt, podSpec)
			if err != nil {
				return nil, err
			}
		}
	}
	if (resourceType == constants.InferenceServiceResource && enableAuth) || resourceType == constants.InferenceGraphResource {
		mountServingSecretVolumeToDeployment(headDeployment, componentMeta, resourceType)
	}
	return deploymentList, nil
}

func createRawDeployment(componentMeta metav1.ObjectMeta, workerComponentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec, workerPodSpec *corev1.PodSpec,
) ([]*appsv1.Deployment, error) {
	var deploymentList []*appsv1.Deployment
	var workerNodeReplicas int32
	var headNodeGpuCount string
	var workerNodeGpuCount string
	multiNodeEnabled := false

	defaultDeployment := createRawDefaultDeployment(componentMeta, componentExt, podSpec)
	if workerPodSpec != nil {
		multiNodeEnabled = true

		// Use the "RAY_NODE_COUNT" environment variable to define the number of worker node replicas.
		// Set the head node GPU count using the requestGPUCount environment variable in the head container
		for _, container := range podSpec.Containers {
			if container.Name == constants.InferenceServiceContainerName {
				if value, exists := utils.GetEnvVarValue(container.Env, constants.RayNodeCountEnvName); exists {
					rayNodeCountFromEnv, err := utils.StringToInt32(value)
					if err != nil {
						log.Error(err, "Failed to convert rayNodeCount to int. Use default")
					}
					workerNodeReplicas = rayNodeCountFromEnv - 1
				}
				if value, exists := utils.GetEnvVarValue(container.Env, constants.RequestGPUCountEnvName); exists {
					headNodeGpuCount = value
				}
				break
			}
		}

		// Set the worker node GPU count using the requestGPUCount environment variable in the worker container
		for _, container := range workerPodSpec.Containers {
			if container.Name == constants.WorkerContainerName {
				if value, exists := utils.GetEnvVarValue(container.Env, constants.RequestGPUCountEnvName); exists {
					workerNodeGpuCount = value
				}
				break
			}
		}

		// Update GPU resource of default podSpec
		if err := addGPUResourceToDeployment(defaultDeployment, constants.InferenceServiceContainerName, headNodeGpuCount); err != nil {
			return nil, err
		}
	}
	deploymentList = append(deploymentList, defaultDeployment)

	// workerNode deployment
	if multiNodeEnabled {
		workerDeployment := createRawWorkerDeployment(workerComponentMeta, componentExt, workerPodSpec, componentMeta.Name, workerNodeReplicas)

		// Update GPU resource of workerPodSpec
		if err := addGPUResourceToDeployment(workerDeployment, constants.WorkerContainerName, workerNodeGpuCount); err != nil {
			return nil, err
		}
		deploymentList = append(deploymentList, workerDeployment)
	}

	return deploymentList, nil
}

func createRawDefaultDeployment(componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec,
) *appsv1.Deployment {
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
	if componentExt.MinReplicas != nil && deployment.Annotations[constants.AutoscalerClass] == string(constants.AutoscalerClassNone) {
		deployment.Spec.Replicas = ptr.To(*componentExt.MinReplicas)
	}

	return deployment
}

func mountServingSecretVolumeToDeployment(deployment *appsv1.Deployment, componentMeta metav1.ObjectMeta, resourceType constants.ResourceType) {
	updatedPodSpec := deployment.Spec.Template.Spec.DeepCopy()
	tlsSecretVolume := corev1.Volume{
		Name: tlsVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  componentMeta.Name + constants.ServingCertSecretSuffix,
				DefaultMode: func(i int32) *int32 { return &i }(420),
			},
		},
	}

	updatedPodSpec.Volumes = append(updatedPodSpec.Volumes, tlsSecretVolume)

	containerName := "kserve-container"
	if resourceType == constants.InferenceGraphResource {
		containerName = componentMeta.Name
	}
	for i, container := range updatedPodSpec.Containers {
		if container.Name == containerName {
			updatedPodSpec.Containers[i].VolumeMounts = append(updatedPodSpec.Containers[i].VolumeMounts, corev1.VolumeMount{
				Name:      tlsVolumeName,
				MountPath: "/etc/tls/private",
			})
		}
	}

	deployment.Spec.Template.Spec = *updatedPodSpec
}

func addOauthContainerToDeployment(ctx context.Context,
	clientset kubernetes.Interface,
	deployment *appsv1.Deployment,
	componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec,
) error {
	var isvcname string
	var upstreamPort string
	var upstreamTimeout string
	var sa string
	if val, ok := componentMeta.Labels[constants.InferenceServicePodLabelKey]; ok {
		isvcname = val
	} else {
		isvcname = componentMeta.Name
	}
	if val, ok := componentMeta.Annotations[constants.ODHKserveRawAuth]; ok && strings.EqualFold(val, "true") {
		switch {
		case componentExt != nil && componentExt.Batcher != nil:
			upstreamPort = constants.InferenceServiceDefaultAgentPortStr
		case componentExt != nil && componentExt.Logger != nil:
			upstreamPort = constants.InferenceServiceDefaultAgentPortStr
		default:
			upstreamPort = GetKServeContainerPort(podSpec)
			if upstreamPort == "" {
				upstreamPort = constants.InferenceServiceDefaultHttpPort
			}
		}
		if podSpec.ServiceAccountName == "" {
			sa = constants.DefaultServiceAccount
		} else {
			sa = podSpec.ServiceAccountName
		}
		if componentExt != nil && componentExt.TimeoutSeconds != nil {
			upstreamTimeout = strconv.FormatInt(*componentExt.TimeoutSeconds, 10)
		}
		oauthProxyContainer, err := generateOauthProxyContainer(ctx, clientset, isvcname, componentMeta.Namespace, upstreamPort, upstreamTimeout, sa)
		if err != nil {
			// return the deployment without the oauth proxy container if there was an error
			// This is required for the deployment_reconciler_tests
			return err
		}
		updatedPodSpec := deployment.Spec.Template.Spec.DeepCopy()
		//	updatedPodSpec := podSpec.DeepCopy()
		// ODH override. See : https://issues.redhat.com/browse/RHOAIENG-19904
		updatedPodSpec.AutomountServiceAccountToken = proto.Bool(true)
		updatedPodSpec.Containers = append(updatedPodSpec.Containers, *oauthProxyContainer)
		deployment.Spec.Template.Spec = *updatedPodSpec
	}
	return nil
}

func createRawWorkerDeployment(componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec, predictorName string, replicas int32,
) *appsv1.Deployment {
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

	// For multinode, it needs to keep original pods until new pods are ready with rollingUpdate strategy
	if deployment.Spec.Strategy.Type == appsv1.RollingUpdateDeploymentStrategyType {
		deployment.Spec.Strategy.RollingUpdate = &appsv1.RollingUpdateDeployment{
			MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "0%"},
			MaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "100%"},
		}
	}

	deployment.Spec.Replicas = &replicas
	return deployment
}

func GetKServeContainerPort(podSpec *corev1.PodSpec) string {
	var kserveContainerPort string

	for _, container := range podSpec.Containers {
		if container.Name == "transformer-container" {
			if len(container.Ports) > 0 {
				return strconv.Itoa(int(container.Ports[0].ContainerPort))
			}
		}
		if container.Name == "kserve-container" {
			if len(container.Ports) > 0 {
				kserveContainerPort = strconv.Itoa(int(container.Ports[0].ContainerPort))
			}
		}
	}

	return kserveContainerPort
}

func generateOauthProxyContainer(ctx context.Context, clientset kubernetes.Interface, isvc string, namespace string, upstreamPort string, upstreamTimeout string, sa string) (*corev1.Container, error) {
	isvcConfigMap, err := clientset.CoreV1().ConfigMaps(constants.KServeNamespace).Get(ctx, constants.InferenceServiceConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	oauthProxyJSON := strings.TrimSpace(isvcConfigMap.Data["oauthProxy"])
	oauthProxyConfig := v1beta1.OauthConfig{}
	if err := json.Unmarshal([]byte(oauthProxyJSON), &oauthProxyConfig); err != nil {
		return nil, err
	}
	if oauthProxyConfig.Image == "" || oauthProxyConfig.MemoryRequest == "" || oauthProxyConfig.MemoryLimit == "" ||
		oauthProxyConfig.CpuRequest == "" || oauthProxyConfig.CpuLimit == "" {
		return nil, errors.New("one or more required oauthProxyConfig fields are empty")
	}
	oauthImage := oauthProxyConfig.Image
	oauthMemoryRequest := oauthProxyConfig.MemoryRequest
	oauthMemoryLimit := oauthProxyConfig.MemoryLimit
	oauthCpuRequest := oauthProxyConfig.CpuRequest
	oauthCpuLimit := oauthProxyConfig.CpuLimit
	oauthUpstreamTimeout := strings.TrimSpace(oauthProxyConfig.UpstreamTimeoutSeconds)
	if upstreamTimeout != "" {
		oauthUpstreamTimeout = upstreamTimeout
	}

	cookieSecret, err := generateCookieSecret()
	if err != nil {
		return nil, err
	}

	args := []string{
		`--https-address=:` + strconv.Itoa(constants.OauthProxyPort),
		`--provider=openshift`,
		`--skip-provider-button`,
		`--openshift-service-account=` + sa,
		`--upstream=http://localhost:` + upstreamPort,
		`--tls-cert=/etc/tls/private/tls.crt`,
		`--tls-key=/etc/tls/private/tls.key`,
		`--cookie-secret=` + cookieSecret,
		`--openshift-delegate-urls={"/": {"namespace": "` + namespace + `", "resource": "inferenceservices", "group": "serving.kserve.io", "name": "` + isvc + `", "verb": "get"}}`,
		`--openshift-sar={"namespace": "` + namespace + `", "resource": "inferenceservices", "group": "serving.kserve.io", "name": "` + isvc + `", "verb": "get"}`,
	}
	if oauthUpstreamTimeout != "" {
		if _, err = strconv.ParseInt(oauthUpstreamTimeout, 10, 64); err != nil {
			return nil, fmt.Errorf("invalid oauthProxy config upstreamTimeoutSeconds value %q: %w", oauthUpstreamTimeout, err)
		}
		args = append(args, `--upstream-timeout=`+oauthUpstreamTimeout+`s`)
	}

	return &corev1.Container{
		Name:  "oauth-proxy",
		Args:  args,
		Image: oauthImage,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: constants.OauthProxyPort,
				Name:          "https",
			},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/oauth/healthz",
					Port:   intstr.FromInt(constants.OauthProxyPort),
					Scheme: corev1.URISchemeHTTPS,
				},
			},
			InitialDelaySeconds: 30,
			TimeoutSeconds:      1,
			PeriodSeconds:       5,
			SuccessThreshold:    1,
			FailureThreshold:    3,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/oauth/healthz",
					Port:   intstr.FromInt(constants.OauthProxyPort),
					Scheme: corev1.URISchemeHTTPS,
				},
			},
			InitialDelaySeconds: 5,
			TimeoutSeconds:      1,
			PeriodSeconds:       5,
			SuccessThreshold:    1,
			FailureThreshold:    3,
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(oauthCpuLimit),
				corev1.ResourceMemory: resource.MustParse(oauthMemoryLimit),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(oauthCpuRequest),
				corev1.ResourceMemory: resource.MustParse(oauthMemoryRequest),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      tlsVolumeName,
				MountPath: "/etc/tls/private",
			},
		},
	}, nil
}

func generateCookieSecret() (string, error) {
	secret := make([]byte, 32)
	_, err := rand.Read(secret)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(secret), nil
}

// checkDeploymentExist checks if the deployment exists?
func (r *DeploymentReconciler) checkDeploymentExist(ctx context.Context, client kclient.Client, deployment *appsv1.Deployment) (constants.CheckResultType, *appsv1.Deployment, error) {
	forceStopRuntime := utils.GetForceStopRuntime(deployment)

	// get deployment
	existingDeployment := &appsv1.Deployment{}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: deployment.ObjectMeta.Namespace,
		Name:      deployment.ObjectMeta.Name,
	}, existingDeployment)
	if err != nil {
		if apierr.IsNotFound(err) {
			if !forceStopRuntime {
				return constants.CheckResultCreate, nil, nil
			}
			return constants.CheckResultSkipped, nil, nil
		}
		return constants.CheckResultUnknown, nil, err
	}

	// existed, but marked for deletion
	if forceStopRuntime {
		ctrl := metav1.GetControllerOf(deployment)
		existingCtrl := metav1.GetControllerOf(existingDeployment)
		if ctrl != nil && existingCtrl != nil && ctrl.UID == existingCtrl.UID {
			return constants.CheckResultDelete, existingDeployment, nil
		}
	}

	// existed, check equivalence
	// for HPA scaling, we should ignore Replicas of Deployment
	// for none scaler, we should not ignore Replicas.
	var ignoreFields cmp.Option = nil // Initialize to nil by default

	// Set ignoreFields if the condition is met
	if existingDeployment.Annotations[constants.AutoscalerClass] != string(constants.AutoscalerClassNone) {
		ignoreFields = cmpopts.IgnoreFields(appsv1.DeploymentSpec{}, "Replicas")
	}

	// Do a dry-run update. This will populate our local deployment object with any default values
	// that are present on the remote version.
	if err := client.Update(ctx, deployment, kclient.DryRunAll); err != nil {
		log.Error(err, "Failed to perform dry-run update of deployment", "Deployment", deployment.Name)
		return constants.CheckResultUnknown, nil, err
	}

	processedExistingDep := v1beta1utils.RemoveCookieSecretArg(*existingDeployment)
	processedNewDep := v1beta1utils.RemoveCookieSecretArg(*deployment)
	if diff, err := kmp.SafeDiff(processedExistingDep.Spec, processedNewDep.Spec, ignoreFields); err != nil {
		log.Error(err, "Failed to diff deployments", "Deployment", deployment.Name)
		return constants.CheckResultUnknown, nil, err
	} else if len(diff) > 0 {
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

// addGPUResourceToDeployment assigns GPU resources to a specific container in a Deployment.
//
// Parameters:
// - deployment: Pointer to the Deployment where GPU resources should be added.
// - targetContainerName: Name of the container within the Deployment to which the GPU resources should be assigned.
// - gpuCount: String representation of the number of GPUs to allocate.
//
// Functionality:
// - Retrieves the list of GPU resource types, updating it based on annotations if available.
// - Identifies the correct GPU resource type by checking existing Limits and Requests values in the container.
// - If no GPU resource is explicitly set, it defaults to "nvidia.com/gpu".
// - Ensures that the container's Limits and Requests maps are initialized before assigning values.
// - Sets both the Limits and Requests for the selected GPU resource type using the provided gpuCount.
func addGPUResourceToDeployment(deployment *appsv1.Deployment, targetContainerName string, gpuCount string) error {
	// Default GPU type is "nvidia.com/gpu"
	gpuResourceType := corev1.ResourceName(constants.NvidiaGPUResourceType)
	updatedGPUResourceTypeList, err := utils.UpdateGPUResourceTypeListByAnnotation(deployment.Spec.Template.Annotations)
	if err != nil {
		return err
	}

	for i, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == targetContainerName {
			for _, gpuType := range updatedGPUResourceTypeList {
				resourceName := corev1.ResourceName(gpuType)
				if qty, exists := deployment.Spec.Template.Spec.Containers[i].Resources.Limits[resourceName]; exists && !qty.IsZero() {
					gpuResourceType = resourceName
					break
				}
				if qty, exists := deployment.Spec.Template.Spec.Containers[i].Resources.Requests[resourceName]; exists && !qty.IsZero() {
					gpuResourceType = resourceName
					break
				}
			}

			// Initialize Limits map if it's nil
			if container.Resources.Limits == nil {
				deployment.Spec.Template.Spec.Containers[i].Resources.Limits = make(map[corev1.ResourceName]resource.Quantity)
			}

			// Assign the gpuCount value to the GPU resource limits
			deployment.Spec.Template.Spec.Containers[i].Resources.Limits[gpuResourceType] = resource.MustParse(gpuCount)

			// Initialize Requests map if it's nil
			if container.Resources.Requests == nil {
				deployment.Spec.Template.Spec.Containers[i].Resources.Requests = make(map[corev1.ResourceName]resource.Quantity)
			}

			// Assign the gpuCount value to the GPU resource requests
			deployment.Spec.Template.Spec.Containers[i].Resources.Requests[gpuResourceType] = resource.MustParse(gpuCount)
			break
		}
	}
	return nil
}

// Reconcile ...
func (r *DeploymentReconciler) Reconcile(ctx context.Context) ([]*appsv1.Deployment, error) {
	for _, desiredDep := range r.DeploymentList {
		// Reconcile Deployment
		checkResult, existingDep, err := r.checkDeploymentExist(ctx, r.client, desiredDep)
		if err != nil {
			return nil, err
		}
		log.Info("deployment reconcile", "checkResult", checkResult, "err", err)

		var opErr error
		switch checkResult {
		case constants.CheckResultCreate:
			opErr = r.client.Create(ctx, desiredDep)
		case constants.CheckResultUpdate:
			curDeployment := existingDep.DeepCopy()
			modDeployment := desiredDep.DeepCopy()

			// To avoid the conflict between HPA and Deployment,
			// we need to remove the Replicas field from the deployment spec
			// For none autoscaler, it should not remove replicas
			if modDeployment.Annotations[constants.AutoscalerClass] != string(constants.AutoscalerClassNone) {
				modDeployment.Spec.Replicas = nil
				curDeployment.Spec.Replicas = nil
			}

			curJson, err := json.Marshal(curDeployment)
			if err != nil {
				return nil, err
			}

			modJson, err := json.Marshal(modDeployment)
			if err != nil {
				return nil, err
			}

			// Generate the strategic merge patch between the current and modified JSON
			patchByte, err := strategicpatch.CreateTwoWayMergePatch(curJson, modJson, appsv1.Deployment{})
			if err != nil {
				return nil, err
			}

			// Patch the deployment object with the strategic merge patch
			opErr = r.client.Patch(ctx, existingDep, kclient.RawPatch(types.StrategicMergePatchType, patchByte))

		case constants.CheckResultDelete:
			log.Info("Deleting deployment", "namespace", existingDep.Namespace, "name", existingDep.Name)
			if existingDep.GetDeletionTimestamp() == nil { // check if the deployment was already deleted
				opErr = r.client.Delete(ctx, existingDep)
			}
		}

		if opErr != nil {
			return nil, opErr
		}
	}
	return r.DeploymentList, nil
}
