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
	"github.com/kubeflow/kfserving/pkg/constants"

	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/controller/kfservice/resources/knative"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/kmp"
	knservingv1alpha1 "knative.dev/serving/pkg/apis/serving/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type RouteReconciler struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewRouteReconciler(client client.Client, scheme *runtime.Scheme) *RouteReconciler {
	return &RouteReconciler{
		client: client,
		scheme: scheme,
	}
}

func (r *RouteReconciler) Reconcile(kfsvc *v1alpha2.KFService) error {
	endpoint := constants.Predictor
	if kfsvc.Spec.Default.Transformer != nil {
		endpoint = constants.Transformer
	}

	desired := knative.NewRouteBuilder().CreateKnativeRoute(kfsvc, endpoint, constants.Predict)

	status, err := r.reconcileRoute(kfsvc, desired)
	if err != nil {
		return err
	}

	if kfsvc.Spec.Default.Explainer != nil {
		endpoint := constants.Explainer

		desired := knative.NewRouteBuilder().CreateKnativeRoute(kfsvc, endpoint, constants.Explain)

		//TODO - what about status returned?
		_, err := r.reconcileRoute(kfsvc, desired)
		if err != nil {
			return err
		}
	}

	// Update parent object's status
	kfsvc.Status.PropagateRouteStatus(status)
	return nil
}

func (r *RouteReconciler) reconcileRoute(kfsvc *v1alpha2.KFService, desired *knservingv1alpha1.Route) (*knservingv1alpha1.RouteStatus, error) {
	if err := controllerutil.SetControllerReference(kfsvc, desired, r.scheme); err != nil {
		return nil, err
	}

	// Create route if does not exist
	existing := &knservingv1alpha1.Route{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating Knative Serving route", "namespace", desired.Namespace, "name", desired.Name)
			return &desired.Status, r.client.Create(context.TODO(), desired)
		}
		return nil, err
	}

	// Return if no differences to reconcile.
	if routeSemanticEquals(desired, existing) {
		return &existing.Status, nil
	}

	// Reconcile differences and update
	diff, err := kmp.SafeDiff(desired.Spec, existing.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff route: %v", err)
	}
	log.Info("Reconciling route diff (-desired, +observed):", "diff", diff)
	log.Info("Updating route", "namespace", existing.Namespace, "name", existing.Name)
	existing.Spec = desired.Spec
	existing.ObjectMeta.Labels = desired.ObjectMeta.Labels
	existing.ObjectMeta.Annotations = desired.ObjectMeta.Annotations
	err = r.client.Update(context.TODO(), existing)
	if err != nil {
		return &existing.Status, err
	}

	return &existing.Status, nil
}

func routeSemanticEquals(desired, existing *knservingv1alpha1.Route) bool {
	return equality.Semantic.DeepEqual(desired.Spec, existing.Spec) &&
		equality.Semantic.DeepEqual(desired.ObjectMeta.Labels, existing.ObjectMeta.Labels) &&
		equality.Semantic.DeepEqual(desired.ObjectMeta.Annotations, existing.ObjectMeta.Annotations)
}
