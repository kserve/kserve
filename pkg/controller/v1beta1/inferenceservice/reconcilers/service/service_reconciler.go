/*
Copyright 2021 kubeflow.org.
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
	"strconv"

	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
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

//ServiceReconciler is the struct of Raw K8S Object
type ServiceReconciler struct {
	client       client.Client
	scheme       *runtime.Scheme
	Service      *corev1.Service
	componentExt *v1beta1.ComponentExtensionSpec
}

func NewServiceReconciler(client client.Client,
	scheme *runtime.Scheme,
	componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec) *ServiceReconciler {
	return &ServiceReconciler{
		client:       client,
		scheme:       scheme,
		Service:      createService(componentMeta, componentExt, podSpec),
		componentExt: componentExt,
	}
}

func createService(componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec) *corev1.Service {
	var port int
	if podSpec.Containers != nil && podSpec.Containers[0].Ports != nil {
		port = int(podSpec.Containers[0].Ports[0].ContainerPort)
	} else {
		port, _ = strconv.Atoi(constants.InferenceServiceDefaultHttpPort)
	}
	service := &corev1.Service{
		ObjectMeta: componentMeta,
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": constants.GetRawServiceLabel(componentMeta.Name),
			},
			Ports: []corev1.ServicePort{
				{
					Name: componentMeta.Name,
					Port: constants.CommonDefaultHttpPort,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: int32(port),
					},
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}
	return service
}

//checkServiceExist checks if the service exists?
func (r *ServiceReconciler) checkServiceExist(client client.Client) (constants.CheckResultType, error) {
	//get service
	exsitingService := &corev1.Service{}
	err := client.Get(context.TODO(), types.NamespacedName{
		Namespace: r.Service.Namespace,
		Name:      r.Service.Name,
	}, exsitingService)
	if err != nil {
		if apierr.IsNotFound(err) {
			return constants.CheckResultCreate, nil
		}
		return constants.CheckResultUnknown, err
	}

	//existed, check equivalent
	if semanticServiceEquals(r.Service, exsitingService) {
		return constants.CheckResultExisted, nil
	}
	return constants.CheckResultUpdate, nil
}

func semanticServiceEquals(desired, existing *corev1.Service) bool {
	return equality.Semantic.DeepEqual(desired.Spec.Ports, existing.Spec.Ports) &&
		equality.Semantic.DeepEqual(desired.Spec.Selector, existing.Spec.Selector)
}

//Reconcile ...
func (r *ServiceReconciler) Reconcile() (*corev1.Service, error) {
	//reconcile Service
	checkResult, err := r.checkServiceExist(r.client)
	log.Info("service reconcile", "checkResult", checkResult, "err", err)
	if err != nil {
		return nil, err
	}

	if checkResult == constants.CheckResultCreate {
		err = r.client.Create(context.TODO(), r.Service)
	} else if checkResult == constants.CheckResultUpdate { //CheckResultUpdate
		err = r.client.Update(context.TODO(), r.Service)
	}
	if err != nil {
		return nil, err
	}

	return r.Service, nil
}
