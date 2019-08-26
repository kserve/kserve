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

	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/kfservice/resources/knative"
	"knative.dev/pkg/kmp"
	knservingv1alpha1 "knative.dev/serving/pkg/apis/serving/v1alpha1"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
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

func (r *ServiceReconciler) Reconcile(kfsvc *v1alpha2.KFService) error {
	if err := r.reconcileDefault(kfsvc); err != nil {
		return err
	}
	if err := r.reconcileCanary(kfsvc); err != nil {
		return err
	}
	return nil
}

func (r *ServiceReconciler) reconcileDefault(kfsvc *v1alpha2.KFService) error {
	defaultService, err := r.serviceBuilder.CreateKnativeService(
		constants.DefaultServiceName(kfsvc.Name),
		kfsvc.ObjectMeta,
		&kfsvc.Spec.Default.Predictor,
	)
	if err != nil {
		return err
	}

	status, err := r.reconcileService(kfsvc, defaultService)
	if err != nil {
		return err
	}

	kfsvc.Status.PropagateDefaultPredictorStatus(status)
	return nil
}

func (r *ServiceReconciler) reconcileCanary(kfsvc *v1alpha2.KFService) error {
	if kfsvc.Spec.Canary == nil {
		if err := r.finalizeService(kfsvc); err != nil {
			return err
		}
		kfsvc.Status.PropagateCanaryPredictorStatus(nil)
		return nil
	}

	canaryService, err := r.serviceBuilder.CreateKnativeService(
		constants.CanaryServiceName(kfsvc.Name),
		kfsvc.ObjectMeta,
		&kfsvc.Spec.Canary.Predictor,
	)
	if err != nil {
		return err
	}

	status, err := r.reconcileService(kfsvc, canaryService)
	if err != nil {
		return err
	}

	kfsvc.Status.PropagateCanaryPredictorStatus(status)
	return nil
}

func (r *ServiceReconciler) finalizeService(kfsvc *v1alpha2.KFService) error {
	canaryServiceName := constants.CanaryServiceName(kfsvc.Name)
	existing := &knservingv1alpha1.Service{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: canaryServiceName, Namespace: kfsvc.Namespace}, existing); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	} else {
		log.Info("Deleting Knative Service", "namespace", kfsvc.Namespace, "name", canaryServiceName)
		if err := r.client.Delete(context.TODO(), existing, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}

func (r *ServiceReconciler) reconcileService(kfsvc *v1alpha2.KFService, desired *knservingv1alpha1.Service) (*knservingv1alpha1.ServiceStatus, error) {
	if err := controllerutil.SetControllerReference(kfsvc, desired, r.scheme); err != nil {
		return nil, err
	}
	// Create service if does not exist
	existing := &knservingv1alpha1.Service{}
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
	diff, err := kmp.SafeDiff(desired.Spec, existing.Spec)
	if err != nil {
		return &existing.Status, fmt.Errorf("failed to diff service: %v", err)
	}
	log.Info("Reconciling service diff (-desired, +observed):", "diff", diff)
	log.Info("Updating service", "namespace", desired.Namespace, "name", desired.Name)
	existing.Spec = desired.Spec
	existing.ObjectMeta.Labels = desired.ObjectMeta.Labels
	existing.ObjectMeta.Annotations = desired.ObjectMeta.Annotations
	if err := r.client.Update(context.TODO(), existing); err != nil {
		return &existing.Status, err
	}

	return &existing.Status, nil
}

func semanticEquals(desiredService, service *knservingv1alpha1.Service) bool {
	return equality.Semantic.DeepEqual(desiredService.Spec, service.Spec) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Labels, service.ObjectMeta.Labels) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Annotations, service.ObjectMeta.Annotations)
}
