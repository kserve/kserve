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

package service

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

var log = logf.Log.WithName("ServiceReconciler")

// ServiceReconciler is the struct of Raw K8S Object
type ServiceReconciler struct {
	client       client.Client
	scheme       *runtime.Scheme
	ServiceList  []*corev1.Service
	componentExt *v1beta1.ComponentExtensionSpec
}

func NewServiceReconciler(client client.Client,
	scheme *runtime.Scheme,
	componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec, multiNodeEnabled bool,
	serviceConfig *v1beta1.ServiceConfig,
	podTemplateHash string,
) *ServiceReconciler {
	return &ServiceReconciler{
		client:       client,
		scheme:       scheme,
		ServiceList:  createService(componentMeta, componentExt, podSpec, multiNodeEnabled, serviceConfig, podTemplateHash),
		componentExt: componentExt,
	}
}

// isGrpcPort checks if the port is a grpc port or not by port name
func isGrpcPort(port corev1.ContainerPort) bool {
	if strings.Contains(port.Name, "grpc") || strings.Contains(port.Name, "h2c") {
		return true
	}
	return false
}

func getAppProtocol(port corev1.ContainerPort) *string {
	if isGrpcPort(port) {
		return ptr.To("kubernetes.io/h2c")
	}
	return nil
}

func createService(componentMeta metav1.ObjectMeta, componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec, multiNodeEnabled bool, serviceConfig *v1beta1.ServiceConfig,
	podTemplateHash string,
) []*corev1.Service {
	var svcList []*corev1.Service

	if !multiNodeEnabled {
		// If multiNodeEnabled is false, only defaultSvc will be created.
		defaultSvc := createDefaultSvc(componentMeta, componentExt, podSpec, serviceConfig)
		svcList = append(svcList, defaultSvc)
	} else {
		// If multiNodeEnabled is true, create defaultSvc, headSvc and workerSvc.
		defaultSvc := createDefaultSvc(componentMeta, componentExt, podSpec, serviceConfig)
		svcList = append(svcList, defaultSvc)

		headSvc := createHeadlessSvc(componentMeta, podTemplateHash)
		svcList = append(svcList, headSvc)

		workerSvc := createWorkerHeadlessSvc(componentMeta, podTemplateHash)
		svcList = append(svcList, workerSvc)
	}

	return svcList
}

func createDefaultSvc(componentMeta metav1.ObjectMeta, componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec, serviceConfig *v1beta1.ServiceConfig,
) *corev1.Service {
	var servicePorts []corev1.ServicePort

	if len(podSpec.Containers) != 0 {
		container := podSpec.Containers[0]
		for _, c := range podSpec.Containers {
			if c.Name == constants.TransformerContainerName {
				container = c
				break
			}
		}
		if len(container.Ports) > 0 {
			var servicePort corev1.ServicePort
			servicePort = corev1.ServicePort{
				Name: container.Ports[0].Name,
				Port: constants.CommonDefaultHttpPort,
				TargetPort: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: container.Ports[0].ContainerPort,
				},
				Protocol:    container.Ports[0].Protocol,
				AppProtocol: getAppProtocol(container.Ports[0]),
			}
			servicePorts = append(servicePorts, servicePort)

			for i := 1; i < len(container.Ports); i++ {
				port := container.Ports[i]
				if port.Protocol == "" {
					port.Protocol = corev1.ProtocolTCP
				}
				servicePort = corev1.ServicePort{
					Name: port.Name,
					Port: port.ContainerPort,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: port.ContainerPort,
					},
					Protocol:    port.Protocol,
					AppProtocol: getAppProtocol(port),
				}
				servicePorts = append(servicePorts, servicePort)
			}
		} else {
			port, _ := utils.StringToInt32(constants.InferenceServiceDefaultHttpPort)
			servicePorts = append(servicePorts, corev1.ServicePort{
				Name: componentMeta.Name,
				Port: constants.CommonDefaultHttpPort,
				TargetPort: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: port,
				},
				Protocol: corev1.ProtocolTCP,
			})
		}
	}
	if componentExt != nil && componentExt.Batcher != nil {
		servicePorts[0].TargetPort = intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: constants.InferenceServiceDefaultAgentPort,
		}
	}
	if componentExt != nil && componentExt.Logger != nil {
		servicePorts[0].TargetPort = intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: constants.InferenceServiceDefaultAgentPort,
		}
	}

	service := &corev1.Service{
		ObjectMeta: componentMeta,
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": constants.GetRawServiceLabel(componentMeta.Name),
			},
			Ports: servicePorts,
		},
	}

	if serviceConfig != nil && serviceConfig.ServiceClusterIPNone {
		service.Spec.ClusterIP = corev1.ClusterIPNone
	}

	return service
}

func createWorkerHeadlessSvc(componentMeta metav1.ObjectMeta, podTemplateHash string) *corev1.Service {
	workerComponentMeta := componentMeta.DeepCopy()
	predictorSvcName := workerComponentMeta.Name
	workerComponentMeta.Name = constants.GetWorkerServiceName(predictorSvcName, podTemplateHash)
	workerComponentMeta.Labels[constants.MultiNodeRoleLabelKey] = constants.MultiNodeWorker

	service := &corev1.Service{
		ObjectMeta: *workerComponentMeta,
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":                             constants.GetRawWorkerServiceLabel(predictorSvcName),
				constants.PodTemplateHashLabelKey: podTemplateHash,
			},
			ClusterIP:                "None",
			PublishNotReadyAddresses: true,
		},
	}
	return service
}

func createHeadlessSvc(componentMeta metav1.ObjectMeta, podTemplateHash string) *corev1.Service {
	workerComponentMeta := componentMeta.DeepCopy()
	predictorSvcName := workerComponentMeta.Name
	workerComponentMeta.Name = constants.GetHeadServiceName(predictorSvcName, podTemplateHash)
	workerComponentMeta.Labels[constants.MultiNodeRoleLabelKey] = constants.MultiNodeHead

	service := &corev1.Service{
		ObjectMeta: *workerComponentMeta,
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":                             constants.GetRawServiceLabel(predictorSvcName),
				constants.PodTemplateHashLabelKey: podTemplateHash,
			},
			ClusterIP:                "None", // Without this, it requires a Port but this Service does not need it.
			PublishNotReadyAddresses: true,
		},
	}
	return service
}

// checkServiceExist checks if the service exists?
func (r *ServiceReconciler) checkServiceExist(ctx context.Context, client client.Client, svc *corev1.Service) (constants.CheckResultType, *corev1.Service, error) {
	forceStopRuntime := utils.GetForceStopRuntime(svc)

	// get service
	existingService := &corev1.Service{}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: svc.Namespace,
		Name:      svc.Name,
	}, existingService)
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
		ctrl := metav1.GetControllerOf(svc)
		existingCtrl := metav1.GetControllerOf(existingService)
		if ctrl != nil && existingCtrl != nil && ctrl.UID == existingCtrl.UID {
			return constants.CheckResultDelete, existingService, nil
		}
	}

	// existed, check equivalent
	if semanticServiceEquals(svc, existingService) {
		return constants.CheckResultExisted, existingService, nil
	}
	return constants.CheckResultUpdate, existingService, nil
}

func semanticServiceEquals(desired, existing *corev1.Service) bool {
	return equality.Semantic.DeepEqual(desired.Spec.Ports, existing.Spec.Ports) &&
		equality.Semantic.DeepEqual(desired.Spec.Selector, existing.Spec.Selector)
}

// Reconcile ...
func (r *ServiceReconciler) Reconcile(ctx context.Context) ([]*corev1.Service, error) {
	for _, svc := range r.ServiceList {
		// reconcile Service
		checkResult, _, err := r.checkServiceExist(ctx, r.client, svc)
		log.Info("service reconcile", "checkResult", checkResult, "err", err)
		if err != nil {
			return nil, err
		}

		var opErr error
		switch checkResult {
		case constants.CheckResultCreate:
			opErr = r.client.Create(ctx, svc)
		case constants.CheckResultUpdate:
			opErr = r.client.Update(ctx, svc)
		case constants.CheckResultDelete:
			if svc.GetDeletionTimestamp() == nil { // check if the service was already deleted
				log.Info("Deleting service", "namespace", svc.Namespace, "name", svc.Name)
				opErr = r.client.Delete(ctx, svc)
			}
		}

		if opErr != nil {
			return nil, opErr
		}
	}

	// Clean up old headless services from previous pod template hashes
	if err := r.cleanupOldHeadlessServices(ctx); err != nil {
		return nil, err
	}

	return r.ServiceList, nil
}

// cleanupOldHeadlessServices removes headless services that belong to the same
// InferenceService but have a stale pod-template-hash (from a previous rollout).
func (r *ServiceReconciler) cleanupOldHeadlessServices(ctx context.Context) error {
	// Collect current head and worker service names
	currentSvcNames := map[string]bool{}
	var namespace, isvcName string
	for _, svc := range r.ServiceList {
		if svc.Labels != nil {
			role := svc.Labels[constants.MultiNodeRoleLabelKey]
			if role == constants.MultiNodeHead || role == constants.MultiNodeWorker {
				currentSvcNames[svc.Name] = true
				namespace = svc.Namespace
				if isvcName == "" {
					isvcName = svc.Labels[constants.InferenceServicePodLabelKey]
				}
			}
		}
	}
	if len(currentSvcNames) == 0 || isvcName == "" {
		return nil
	}

	// List all head and worker headless services for this ISVC and delete stale ones
	for _, role := range []string{constants.MultiNodeHead, constants.MultiNodeWorker} {
		svcList := &corev1.ServiceList{}
		if err := r.client.List(ctx, svcList,
			client.InNamespace(namespace),
			client.MatchingLabels{
				constants.MultiNodeRoleLabelKey:       role,
				constants.InferenceServicePodLabelKey: isvcName,
			},
		); err != nil {
			return err
		}

		for i := range svcList.Items {
			if currentSvcNames[svcList.Items[i].Name] {
				continue
			}
			// Skip deletion while pods still use this service as their master address.
			// PublishNotReadyAddresses is set, so all pods appear in the slice regardless
			// of readiness; any entry means at least one pod still needs this service.
			epSliceList := &discoveryv1.EndpointSliceList{}
			if err := r.client.List(ctx, epSliceList,
				client.InNamespace(namespace),
				client.MatchingLabels{discoveryv1.LabelServiceName: svcList.Items[i].Name},
			); err != nil {
				log.Error(err, "Failed to list EndpointSlices for stale service, skipping deletion",
					"name", svcList.Items[i].Name)
				continue
			}
			hasEndpoints := false
			for _, eps := range epSliceList.Items {
				if len(eps.Endpoints) > 0 {
					hasEndpoints = true
					break
				}
			}
			if hasEndpoints {
				log.Info("Skipping cleanup of old headless service — endpoints still active",
					"name", svcList.Items[i].Name)
				continue
			}
			log.Info("Cleaning up old headless service", "name", svcList.Items[i].Name)
			if err := r.client.Delete(ctx, &svcList.Items[i]); err != nil && !apierr.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

// GetServiceList returns all managed services
func (r *ServiceReconciler) GetServiceList() []*corev1.Service {
	return r.ServiceList
}

// SetControllerReferences sets owner references on all services
func (r *ServiceReconciler) SetControllerReferences(owner metav1.Object, scheme *runtime.Scheme) error {
	for _, svc := range r.ServiceList {
		if err := controllerutil.SetControllerReference(owner, svc, scheme); err != nil {
			return err
		}
	}
	return nil
}
