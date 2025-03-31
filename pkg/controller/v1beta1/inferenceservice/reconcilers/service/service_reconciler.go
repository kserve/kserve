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
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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
	resourceType constants.ResourceType,
	componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec, multiNodeEnabled bool,
	serviceConfig *v1beta1.ServiceConfig) *ServiceReconciler {
	return &ServiceReconciler{
		client:       client,
		scheme:       scheme,
		ServiceList:  createService(resourceType, componentMeta, componentExt, podSpec, multiNodeEnabled, serviceConfig),
		componentExt: componentExt,
	}
}

func createService(resourceType constants.ResourceType, componentMeta metav1.ObjectMeta, componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec, multiNodeEnabled bool, serviceConfig *v1beta1.ServiceConfig) []*corev1.Service {
	var svcList []*corev1.Service
	var isWorkerContainer bool

	if multiNodeEnabled {
		for _, container := range podSpec.Containers {
			if container.Name == constants.WorkerContainerName {
				isWorkerContainer = true
			}
		}
	}

	if !multiNodeEnabled {
		// If multiNodeEnabled is false, only defaultSvc will be created.
		defaultSvc := createDefaultSvc(resourceType, componentMeta, componentExt, podSpec, serviceConfig)
		svcList = append(svcList, defaultSvc)
	} else if multiNodeEnabled && !isWorkerContainer {
		// If multiNodeEnabled is true, both defaultSvc and headSvc will be created.
		defaultSvc := createDefaultSvc(resourceType, componentMeta, componentExt, podSpec, serviceConfig)
		svcList = append(svcList, defaultSvc)

		headSvc := createHeadlessSvc(componentMeta)
		svcList = append(svcList, headSvc)
	}

	return svcList
}

func createDefaultSvc(resourceType constants.ResourceType, componentMeta metav1.ObjectMeta, componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec, serviceConfig *v1beta1.ServiceConfig) *corev1.Service {
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
				Protocol: container.Ports[0].Protocol,
			}
			if len(servicePort.Name) == 0 {
				servicePort.Name = "http"
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
					Protocol: port.Protocol,
				}
				servicePorts = append(servicePorts, servicePort)
			}
		} else {
			port, _ := strconv.Atoi(constants.InferenceServiceDefaultHttpPort)
			portInt32 := int32(port) // nolint  #nosec G109
			servicePorts = append(servicePorts, corev1.ServicePort{
				Name: componentMeta.Name,
				Port: constants.CommonDefaultHttpPort,
				TargetPort: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: portInt32, // #nosec G109
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

	if service.ObjectMeta.Annotations == nil {
		service.ObjectMeta.Annotations = make(map[string]string)
	}
	service.ObjectMeta.Annotations[constants.OpenshiftServingCertAnnotation] = componentMeta.Name + constants.ServingCertSecretSuffix

	if resourceType == constants.InferenceGraphResource {
		servicePorts[0].Port = int32(443)
	} else {
		if val, ok := componentMeta.Annotations[constants.ODHKserveRawAuth]; ok && strings.EqualFold(val, "true") {
			httpsPort := corev1.ServicePort{
				Name: "https",
				Port: constants.OauthProxyPort,
				TargetPort: intstr.IntOrString{
					Type:   intstr.String,
					StrVal: "https",
				},
				Protocol: corev1.ProtocolTCP,
			}
			ports := service.Spec.Ports
			replaced := false
			for i, port := range ports {
				if port.Port == constants.CommonDefaultHttpPort {
					ports[i] = httpsPort
					replaced = true
				}
			}
			if !replaced {
				ports = append(ports, httpsPort)
			}
			service.Spec.Ports = ports
		}
	}

	if serviceConfig != nil && serviceConfig.ServiceClusterIPNone {
		service.Spec.ClusterIP = corev1.ClusterIPNone
	}

	return service
}

func createHeadlessSvc(componentMeta metav1.ObjectMeta) *corev1.Service {
	workerComponentMeta := componentMeta.DeepCopy()
	predictorSvcName := workerComponentMeta.Name
	isvcGeneration := componentMeta.GetLabels()[constants.InferenceServiceGenerationPodLabelKey]
	workerComponentMeta.Name = constants.GeHeadServiceName(predictorSvcName, isvcGeneration)
	workerComponentMeta.Labels[constants.MultiNodeRoleLabelKey] = constants.MultiNodeHead

	service := &corev1.Service{
		ObjectMeta: *workerComponentMeta,
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": constants.GetRawServiceLabel(predictorSvcName),
				constants.InferenceServiceGenerationPodLabelKey: isvcGeneration,
			},
			ClusterIP:                "None", // Without this, it requires a Port but this Service does not need it.
			PublishNotReadyAddresses: true,
		},
	}
	return service
}

func (r *ServiceReconciler) cleanHeadSvc() error {
	svcList := &corev1.ServiceList{}
	if err := r.client.List(context.TODO(), svcList, client.MatchingLabels{
		constants.MultiNodeRoleLabelKey: constants.MultiNodeHead,
	}); err != nil {
		return err
	}

	sort.Slice(svcList.Items, func(i, j int) bool {
		return svcList.Items[i].CreationTimestamp.Time.After(svcList.Items[j].CreationTimestamp.Time)
	})

	// Keep the 3 newest services and delete the rest
	for i := 3; i < len(svcList.Items); i++ {
		existingService := &corev1.Service{}
		err := r.client.Get(context.TODO(), types.NamespacedName{
			Namespace: svcList.Items[i].Namespace,
			Name:      svcList.Items[i].Name,
		}, existingService)
		if err == nil {
			err := r.client.Delete(context.TODO(), existingService)
			if err != nil {
				fmt.Printf("Failed to delete service %s: %v\n", existingService.Name, err)
			} else {
				fmt.Printf("Deleted service %s in namespace %s\n", existingService.Name, existingService.Namespace)
			}
		}
	}
	return nil
}

// checkServiceExist checks if the service exists?
func (r *ServiceReconciler) checkServiceExist(client client.Client, svc *corev1.Service) (constants.CheckResultType, *corev1.Service, error) {
	// get service
	existingService := &corev1.Service{}
	err := client.Get(context.TODO(), types.NamespacedName{
		Namespace: svc.Namespace,
		Name:      svc.Name,
	}, existingService)
	if err != nil {
		if apierr.IsNotFound(err) {
			return constants.CheckResultCreate, nil, nil
		}
		return constants.CheckResultUnknown, nil, err
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
func (r *ServiceReconciler) Reconcile() ([]*corev1.Service, error) {
	for _, svc := range r.ServiceList {
		// reconcile Service
		checkResult, _, err := r.checkServiceExist(r.client, svc)
		log.Info("service reconcile", "checkResult", checkResult, "err", err)
		if err != nil {
			return nil, err
		}

		var opErr error
		switch checkResult {
		case constants.CheckResultCreate:
			opErr = r.client.Create(context.TODO(), svc)
		case constants.CheckResultUpdate:
			opErr = r.client.Update(context.TODO(), svc)
		}

		if opErr != nil {
			return nil, opErr
		}
	}
	// Clean up head svc when head sevices are more than 3.
	if len(r.ServiceList) > 1 {
		r.cleanHeadSvc()
	}
	return r.ServiceList, nil
}
