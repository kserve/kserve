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

	"github.com/knative/pkg/kmp"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/resources/knative"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

func (r *RouteReconciler) Reconcile(kfsvc *v1alpha1.KFService) error {
	desiredRoute := knative.NewRouteBuilder().CreateKnativeRoute(kfsvc)
	if err := controllerutil.SetControllerReference(kfsvc, &desiredRoute, r.scheme); err != nil {
		return err
	}

	// Create route if does not exist
	route := knservingv1alpha1.Route{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: desiredRoute.Name, Namespace: desiredRoute.Namespace}, &route)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating Knative Serving route", "namespace", desiredRoute.Namespace, "name", desiredRoute.Name)
			err = r.client.Create(context.TODO(), &desiredRoute)
			return err
		}
		return err
	}

	// Return if no differences to reconcile.
	if routeSemanticEquals(desiredRoute, route) {
		return nil
	}

	// Reconcile differences and update
	diff, err := kmp.SafeDiff(desiredRoute.Spec, route.Spec)
	if err != nil {
		return fmt.Errorf("failed to diff route: %v", err)
	}
	log.Info("Reconciling route diff (-desired, +observed):", "diff", diff)
	log.Info("Updating route", "namespace", route.Namespace, "name", route.Name)
	route.Spec = desiredRoute.Spec
	route.ObjectMeta.Labels = desiredRoute.ObjectMeta.Labels
	route.ObjectMeta.Annotations = desiredRoute.ObjectMeta.Annotations
	err = r.client.Update(context.TODO(), &route)
	if err != nil {
		return err
	}

	// Update parent object's status
	kfsvc.Status.PropagateRouteStatus(&route.Status)
	return nil
}

func routeSemanticEquals(desiredRoute, route knservingv1alpha1.Route) bool {
	return equality.Semantic.DeepEqual(desiredRoute.Spec, route.Spec) &&
		equality.Semantic.DeepEqual(desiredRoute.ObjectMeta.Labels, route.ObjectMeta.Labels) &&
		equality.Semantic.DeepEqual(desiredRoute.ObjectMeta.Annotations, route.ObjectMeta.Annotations)
}
