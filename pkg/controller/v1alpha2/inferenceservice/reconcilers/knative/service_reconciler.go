/*
Copyright 2019 kubeflow.org.

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

package knative

import (
	"context"
	"fmt"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/inferenceservice/resources/knative"
	"knative.dev/pkg/kmp"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("Reconciler")

type ServiceReconciler struct {
	client         client.Client
	scheme         *runtime.Scheme
	serviceBuilder *knative.ServiceBuilder
}

func NewServiceReconciler(client client.Client, scheme *runtime.Scheme, config *v1.ConfigMap) *ServiceReconciler {
	return &ServiceReconciler{
		client:         client,
		scheme:         scheme,
		serviceBuilder: knative.NewServiceBuilder(client, config),
	}
}

func (r *ServiceReconciler) Reconcile(isvc *v1alpha2.InferenceService) error {
	for _, component := range []constants.InferenceServiceComponent{constants.Predictor, constants.Transformer, constants.Explainer} {
		if err := r.reconcileComponent(isvc, component, false); err != nil {
			return err
		}
		if err := r.reconcileComponent(isvc, component, true); err != nil {
			return err
		}
	}
	return nil
}

func (r *ServiceReconciler) reconcileComponent(isvc *v1alpha2.InferenceService, component constants.InferenceServiceComponent, isCanary bool) error {
	endpointSpec := &isvc.Spec.Default
	serviceName := constants.DefaultServiceName(isvc.Name, component)
	propagateStatusFn := isvc.Status.PropagateDefaultStatus
	if isCanary {
		endpointSpec = isvc.Spec.Canary
		serviceName = constants.CanaryServiceName(isvc.Name, component)
		propagateStatusFn = isvc.Status.PropagateCanaryStatus
	}
	var service *knservingv1.Service
	var err error
	if endpointSpec != nil {
		service, err = r.serviceBuilder.CreateInferenceServiceComponent(isvc, component, isCanary)
		if err != nil {
			return err
		}
	}
	if service == nil {
		if err = r.finalizeService(serviceName, isvc.Namespace); err != nil {
			return err
		}
		propagateStatusFn(component, nil)
		return nil
	} else {
		if status, err := r.reconcileService(isvc, service); err != nil {
			return err
		} else {
			propagateStatusFn(component, status)
			return nil
		}
	}
}

func (r *ServiceReconciler) finalizeService(serviceName, namespace string) error {
	existing := &knservingv1.Service{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: serviceName, Namespace: namespace}, existing); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	} else {
		log.Info("Deleting Knative Service", "namespace", namespace, "name", serviceName)
		if err := r.client.Delete(context.TODO(), existing, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}

func (r *ServiceReconciler) reconcileService(isvc *v1alpha2.InferenceService, desired *knservingv1.Service) (*knservingv1.ServiceStatus, error) {
	if err := controllerutil.SetControllerReference(isvc, desired, r.scheme); err != nil {
		return nil, err
	}
	// Create service if does not exist
	existing := &knservingv1.Service{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating Knative Service", "namespace", desired.Namespace, "name", desired.Name)
			return &desired.Status, r.client.Create(context.TODO(), desired)
		}
		return nil, err
	}

	// Return if no differences to reconcile.
	if semanticEquals(desired, existing) {
		return &existing.Status, nil
	}

	// Reconcile differences and update
	diff, err := kmp.SafeDiff(desired.Spec.ConfigurationSpec, existing.Spec.ConfigurationSpec)
	if err != nil {
		return &existing.Status, fmt.Errorf("failed to diff Knative Service: %v", err)
	}
	log.Info("Reconciling Knative Service diff (-desired, +observed):", "diff", diff)
	log.Info("Updating Knative Service", "namespace", desired.Namespace, "name", desired.Name)
	existing.Spec.ConfigurationSpec = desired.Spec.ConfigurationSpec
	existing.ObjectMeta.Labels = desired.ObjectMeta.Labels
	if err := r.client.Update(context.TODO(), existing); err != nil {
		return &existing.Status, err
	}

	return &existing.Status, nil
}

func semanticEquals(desiredService, service *knservingv1.Service) bool {
	return equality.Semantic.DeepEqual(desiredService.Spec.ConfigurationSpec, service.Spec.ConfigurationSpec) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Labels, service.ObjectMeta.Labels)
}
